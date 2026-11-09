package client

import (
	"errors"
	"sort"
	"sync"

	"github.com/gmsec/micro/naming"
	"github.com/gmsec/micro/registry"
	"github.com/xxjwxc/public/mylog"
	"github.com/xxjwxc/public/tools"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
)

// ErrResolveNoNaming 没有注册服务发现实现时使用
var ErrResolveNoNaming = errors.New("micro: no naming registry configured, cannot resolve target")

type resolverBuilder struct {
	scheme    string
	regNaming registry.RegNaming
}

func (rb *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	if rb.regNaming == nil {
		return nil, ErrResolveNoNaming
	}

	ep := target.Endpoint()
	watcher, err := rb.regNaming.Resolve(ep)
	if err != nil {
		return nil, err
	}

	r := &myResolver{
		target:     target,
		cc:         cc,
		watcher:    watcher,
		addrsStore: make(map[string]*naming.Update),
		done:       make(chan struct{}),
	}

	r.start()
	return r, nil
}

func (rb *resolverBuilder) Scheme() string { return rb.scheme }

type myResolver struct {
	target  resolver.Target
	cc      resolver.ClientConn
	watcher naming.Watcher

	// mu 保护 addrsStore;同时保证 UpdateState 的调用顺序与 map 内容一致
	mu         sync.Mutex
	addrsStore map[string]*naming.Update

	// done 关闭表示 resolver 已被 grpc 释放,后台 goroutine 需要退出
	done     chan struct{}
	closeOne sync.Once
}

func (r *myResolver) start() {
	go func() {
		for {
			// 已经被关闭,直接退出,不再发起新的查询
			select {
			case <-r.done:
				return
			default:
			}

			updates, err := r.watcher.Next()
			if err != nil {
				select {
				case <-r.done: // Close() 引发的正常退出
				default:
					grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
					mylog.Error(err)
				}
				return
			}

			if r.apply(updates) {
				r.push()
			}
		}
	}()
}

// apply 把一批更新写入 addrsStore,返回是否有变化
func (r *myResolver) apply(updates []*naming.Update) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	isUpdate := false
	for _, update := range updates {
		if update == nil || len(update.Addr) == 0 {
			continue
		}
		switch update.Op {
		case naming.Add:
			if _, ok := r.addrsStore[update.Addr]; !ok { // 有新加才添加
				r.addrsStore[update.Addr] = update
				isUpdate = true
			}
		case naming.Delete:
			if _, ok := r.addrsStore[update.Addr]; ok {
				delete(r.addrsStore, update.Addr) // map 删除
				isUpdate = true
			}
		}
		mylog.Debugf("watcher(%v):%v", r.target.Endpoint(), tools.JSONDecode(update))
	}

	return isUpdate
}

// push 把当前地址列表推给 grpc。
// 排序后推送,避免 map 遍历顺序随机导致同一份地址被反复上报。
func (r *myResolver) push() {
	r.mu.Lock()
	addrs := make([]resolver.Address, 0, len(r.addrsStore))
	for _, v := range r.addrsStore {
		addrs = append(addrs, resolver.Address{Addr: v.Addr})
	}
	r.mu.Unlock()

	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Addr < addrs[j].Addr })

	if err := r.cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		mylog.Error(err)
	}
}

func (*myResolver) ResolveNow(o resolver.ResolveNowOptions) {
	// 本实现由 watcher 持续推送更新,无需响应主动解析请求
}

func (r *myResolver) Close() {
	r.closeOne.Do(func() {
		close(r.done)
		// 关闭 watcher 让阻塞中的 Next() 立刻返回,否则后台 goroutine 永久泄漏
		r.watcher.Close()
	})
	mylog.Debugf("Close:%v", r.target.Endpoint())
}
