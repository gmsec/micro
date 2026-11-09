package registry

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gmsec/micro/naming"
	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/mdns"
	"github.com/google/uuid"
	"github.com/xxjwxc/public/tools"
)

var (
	// use a .micro domain rather than .local
	mdnsDomain = "gmsec"

	// mdnsGraceWindow 是"本轮查不到就判删"的容忍窗口。
	// mDNS 走组播,一次查询漏掉应答很常见,单次查不到就删会误伤健康节点;
	// 连续一个窗口都没出现,才认为对端已经下线。
	mdnsGraceWindow = 5 * time.Second

	// mdnsPollInterval 是两次查询之间的最小间隔。
	// mdns.Query 是"发查询等应答",返回得很快,不节流的话上层会在一个紧凑循环里
	// 反复扫描组播网段,白吃一个核。
	mdnsPollInterval = time.Second
)

// DNSNamingRegister dns default register
type DNSNamingRegister struct {
	opts Options
	// the mdns domain
	domain string

	// Lock 只保护 node / opts.Addrs 两个字段,不覆盖任何网络操作
	sync.Mutex
	node *mdns.Server

	// reg 串行化 Register / Deregister:
	// 两者都会发 goodbye 组播,并发交叉执行会让对端收到互相矛盾的记录。
	reg sync.Mutex
}

// NewDNSNamingRegistry returns a new default dns naming registry which is mdns
func NewDNSNamingRegistry(opts ...Option) RegNaming {
	return newDNSNamingRegistry(opts...)
}

func newDNSNamingRegistry(opts ...Option) RegNaming {
	options := Options{
		Context:          context.Background(),
		Timeout:          time.Millisecond * 100,
		KeepHeartTimeout: time.Second * 15,
		NodeID:           uuid.New().String(),
		ServiceName:      "gmsec.service",
	}
	for _, o := range opts {
		o(&options)
	}

	// set the domain
	domain := mdnsDomain

	d, ok := options.Context.Value("mdns.domain").(string)
	if ok {
		domain = d
	}

	return &DNSNamingRegister{
		opts:   options,
		domain: domain,
	}
}

// Init init option
func (r *DNSNamingRegister) Init(opts ...Option) error {
	r.Lock()
	defer r.Unlock()

	for _, o := range opts {
		o(&r.opts)
	}

	return nil
}

func (r *DNSNamingRegister) String() string {
	r.Lock()
	defer r.Unlock()
	return r.opts.ServiceName
}

// Register register & add new node
func (r *DNSNamingRegister) Register(address string, Metadata interface{}) error {
	r.reg.Lock()
	defer r.reg.Unlock()

	host, pt, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(pt)
	if err != nil {
		return err
	}

	// to add someone service
	txt := naming.Update{
		Op:       naming.Add,
		Addr:     address,
		Metadata: port,
	}

	r.Lock()
	r.opts.Addrs = []string{address}
	nodeID := r.opts.NodeID
	serviceName := r.opts.ServiceName
	old := r.node
	r.node = nil
	r.Unlock()

	// Shutdown 会发 goodbye 组播并做收尾,放在锁里会把所有 Resolve 一起堵住。
	// 原来这段在锁内执行,重注册期间任何服务发现都会被卡住。
	if old != nil {
		old.Shutdown()
	}

	s, err := mdns.NewMDNSService(
		nodeID,
		serviceName,
		r.domain+".",
		"",
		port,
		[]net.IP{net.ParseIP(host)},
		encode(&txt),
	)
	if err != nil {
		return err
	}

	node, err := mdns.NewServer(&mdns.Config{Zone: &mdns.DNSSDService{MDNSService: s}})
	if err != nil {
		return err
	}

	r.Lock()
	r.node = node
	r.Unlock()

	return nil
}

// Deregister 注销
func (r *DNSNamingRegister) Deregister() error {
	r.reg.Lock()
	defer r.reg.Unlock()

	r.Lock()
	node := r.node
	r.node = nil
	r.Unlock()

	if node != nil {
		node.Shutdown()
	}

	return nil
}

// Resolve resolve begin
func (r *DNSNamingRegister) Resolve(target string) (naming.Watcher, error) {
	r.Lock()
	timeout := r.opts.Timeout
	r.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	w := &dnsNamingWatcher{
		timeout:     timeout,
		serviceName: target,
		domain:      r.domain,
		ctx:         ctx,
		cancel:      cancel,
		reported:    make(map[string]bool),
		lastSeen:    make(map[string]time.Time),
	}
	return w, nil
}

type dnsNamingWatcher struct {
	sync.Mutex
	timeout time.Duration

	serviceName string
	domain      string

	// watch mabey
	cancel context.CancelFunc
	ctx    context.Context
	isInit bool

	// reported 记录已经上报过 Add 的地址,避免每轮查询都重复推送同一批实例。
	// lastSeen 记录最近一次出现的时刻,节点静默消失时靠它判定下线。
	reported  map[string]bool
	lastSeen  map[string]time.Time
	lastQuery time.Time
}

// Next gets the next set of updates from the etcd resolver.
// Calls to Next should be serialized; concurrent calls are not safe since
// there is no way to reconcile the update ordering.
func (r *dnsNamingWatcher) Next() ([]*naming.Update, error) {
	r.Lock()
	defer r.Unlock()

	// watcher 已关闭:立即返回错误让调用方(grpc resolver)退出,
	// 否则调用方的后台 goroutine 会永久阻塞在这里。
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	// 两次查询之间留出最小间隔;等待期间仍然能被 Close() 打断。
	if !r.lastQuery.IsZero() {
		if wait := mdnsPollInterval - time.Since(r.lastQuery); wait > 0 {
			select {
			case <-r.ctx.Done():
				return nil, r.ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	r.lastQuery = time.Now()

	// 首次用配置的超时(容忍冷启动时网卡还没准备好),之后固定 100ms
	// 快速轮询,靠 lastSeen 的容忍窗口兜住漏应答。
	if r.isInit {
		r.timeout = time.Millisecond * 100
	}
	r.isInit = true

	return r.getService()
}

func (r *dnsNamingWatcher) getService() ([]*naming.Update, error) {
	now := time.Now()
	entries := make(chan *mdns.ServiceEntry, 10)
	done := make(chan struct{})

	p := mdns.DefaultParams(r.serviceName)
	// 继承 watcher 的 ctx:Close() 能立即打断正在进行的查询
	var cancel context.CancelFunc
	p.Context, cancel = context.WithTimeout(r.ctx, r.timeout)
	defer cancel()
	// set entries channel
	p.Entries = entries
	// set the domain
	p.Domain = r.domain

	// live 是本轮查询到的存活记录,gone 是收到 goodbye(TTL=0)的记录
	live := make(map[string]*naming.Update)
	gone := make(map[string]bool)

	go func() {
		defer close(done)
		for {
			select {
			case e := <-entries:
				u := decodeEntry(e)
				if e.TTL == 0 {
					// 对端优雅下线时会发 TTL=0 的记录,直接判定为删除。
					// 注意 mdns.Query 只收主动查询的应答,收不到主动组播的 goodbye,
					// 所以这条只是"能收到就立刻删"的快路径,兜底仍然靠下面的 lastSeen。
					if u != nil {
						gone[u.Addr] = true
					}
					continue
				}
				if u == nil {
					continue
				}
				live[u.Addr] = u
			case <-p.Context.Done():
				return
			}
		}
	}()

	// execute the query
	err := mdns.Query(p)

	// 无论查询成功与否都等待收集 goroutine 退出,避免泄漏；
	// Query 返回时 ctx 已经结束,goroutine 会立即退出。
	cancel()
	<-done

	if err != nil {
		return nil, err
	}

	var out []*naming.Update

	// 1) 本轮出现的实例:更新 lastSeen,首次出现才推 Add
	for addr, u := range live {
		r.lastSeen[addr] = now
		if !r.reported[addr] {
			r.reported[addr] = true
			out = append(out, u)
		}
	}

	// 2) 收到 goodbye 的实例:立即删
	for addr := range gone {
		if r.reported[addr] {
			delete(r.reported, addr)
			delete(r.lastSeen, addr)
			out = append(out, &naming.Update{Op: naming.Delete, Addr: addr})
		}
	}

	// 3) 本轮没出现、且静默超过容忍窗口的实例:判定下线
	for addr := range r.reported {
		if _, ok := live[addr]; ok {
			continue
		}
		if now.Sub(r.lastSeen[addr]) < mdnsGraceWindow {
			continue
		}
		delete(r.reported, addr)
		delete(r.lastSeen, addr)
		out = append(out, &naming.Update{Op: naming.Delete, Addr: addr})
	}

	return out, nil
}

// decodeEntry 把一条 mDNS 记录还原成命名更新,信息不全时返回 nil
func decodeEntry(e *mdns.ServiceEntry) *naming.Update {
	if len(e.InfoFields) == 0 {
		return nil
	}
	u, err := decode(e.InfoFields)
	if err != nil {
		mylog.ErrorString(err.Error())
		return nil
	}
	if len(u.Addr) == 0 {
		return nil
	}
	return u
}

// Close close watcher
func (r *dnsNamingWatcher) Close() { r.cancel() }

// Options get opts list
func (r *DNSNamingRegister) Options() Options {
	r.Lock()
	defer r.Unlock()
	return r.opts
}

func encode(txt *naming.Update) []string {
	b, _ := json.Marshal(txt)

	// var buf bytes.Buffer
	// defer buf.Reset()

	// w := zlib.NewWriter(&buf)

	// w.Write(b)
	// w.Close()
	encoded := tools.ByteToHex(b) //  hex.EncodeToString(b)
	// split encoded string
	var record []string

	for len(encoded) > 255 {
		record = append(record, encoded[:255])
		encoded = encoded[255:]
	}

	record = append(record, encoded)
	return record
}

func decode(record []string) (*naming.Update, error) {
	hr := tools.HexToBye(strings.Join(record, "")) // hex.DecodeString(encoded)

	var txt *naming.Update
	if err := json.Unmarshal(hr, &txt); err != nil {
		return nil, err
	}

	return txt, nil
}
