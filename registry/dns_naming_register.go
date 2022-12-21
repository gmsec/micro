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
)

// DNSNamingRegister dns default register
type DNSNamingRegister struct {
	opts Options
	// the mdns domain
	domain string
	sync.Mutex
	node *mdns.Server

	// listener
	// listener chan *mdns.ServiceEntry
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
	for _, o := range opts {
		o(&r.opts)
	}

	return nil
}

func (r *DNSNamingRegister) String() string {
	return r.opts.ServiceName
}

// Register register & add new node
func (r *DNSNamingRegister) Register(address string, Metadata interface{}) error {
	r.Lock()
	defer r.Unlock()
	// r.opts.Timeout = time.Millisecond * 100

	r.opts.Addrs = []string{address}
	host, pt, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	port, _ := strconv.Atoi(pt)

	// to add someone service
	txt := naming.Update{
		Op:       naming.Add,
		Addr:     address,
		Metadata: port,
	}

	if r.node != nil {
		r.node.Shutdown()
	}

	s, err := mdns.NewMDNSService(
		r.opts.NodeID,
		r.opts.ServiceName,
		r.domain+".",
		"",
		port,
		[]net.IP{net.ParseIP(host)},
		encode(&txt),
	)
	if err != nil {
		return err
	}
	r.node, err = mdns.NewServer(&mdns.Config{Zone: &mdns.DNSSDService{MDNSService: s}})
	if err != nil {
		return err
	}

	return nil
}

// Deregister 注销
func (r *DNSNamingRegister) Deregister() error {
	r.Lock()
	defer r.Unlock()
	if r.node != nil {
		r.node.Shutdown()
		r.node = nil
	}

	return nil
}

// Resolve resolve begin
func (r *DNSNamingRegister) Resolve(target string) (naming.Watcher, error) {
	r.Lock()
	defer r.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	w := &dnsNamingWatcher{
		node:        r.node,
		timeout:     r.opts.Timeout,
		serviceName: target,
		domain:      r.domain,
		ctx:         ctx,
		cancel:      cancel,
	}
	return w, nil
}

type dnsNamingWatcher struct {
	sync.Mutex
	node    *mdns.Server
	timeout time.Duration

	serviceName string
	domain      string

	// watch mabey
	cancel context.CancelFunc
	ctx    context.Context
	isInit bool
	// c      *etcd.Client
	// target string
	// wch    etcd.WatchChan
	// err    error
}

// Next gets the next set of updates from the etcd resolver.
// Calls to Next should be serialized; concurrent calls are not safe since
// there is no way to reconcile the update ordering.
func (r *dnsNamingWatcher) Next() ([]*naming.Update, error) {
	r.Lock()
	defer r.Unlock()

	if !r.isInit { // first
		r.isInit = true
		timeout := r.timeout
		defer func() {
			if timeout > time.Second*3 {
				timeout = time.Second * 3
			}
			r.timeout = timeout
		}()
		r.timeout = time.Millisecond * 100
		return r.getService()
	}

	return r.getService()
}

func (r *dnsNamingWatcher) getService() ([]*naming.Update, error) {
	var serviceList []*naming.Update
	entries := make(chan *mdns.ServiceEntry, 10)
	done := make(chan bool)

	p := mdns.DefaultParams(r.serviceName)
	// set context with timeout
	var cancel context.CancelFunc
	p.Context, cancel = context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	// set entries channel
	p.Entries = entries
	// set the domain
	p.Domain = r.domain

	go func() {
		for {
			select {
			case e := <-entries:
				// list record so skip
				if p.Domain != r.domain {
					continue
				}
				if e.TTL == 0 {
					continue
				}

				var upInfo naming.Update
				if len(e.InfoFields) > 0 {
					tmp, err := decode(e.InfoFields)
					if err != nil {
						mylog.ErrorString(err.Error())
					} else {
						upInfo = *tmp
					}
				}
				if len(upInfo.Addr) == 0 {
					continue
				}

				serviceList = append(serviceList, &upInfo)
			case <-p.Context.Done():
				close(done)
				return
			}
		}
	}()

	// execute the query
	if err := mdns.Query(p); err != nil {
		return nil, err
	}

	// wait for completion
	<-done

	return serviceList, nil
}

// Close close watcher
func (r *dnsNamingWatcher) Close() { r.cancel() }

// Options get opts list
func (r *DNSNamingRegister) Options() Options {
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
