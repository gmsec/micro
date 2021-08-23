package client

import (
	"fmt"
	"sync"

	"github.com/gmsec/micro/naming"
	"github.com/gmsec/micro/registry"
	"github.com/xxjwxc/public/dev"
	"github.com/xxjwxc/public/mylog"
	"github.com/xxjwxc/public/tools"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
	scheme    string
	regNaming registry.RegNaming
}

func (rb *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	watcher, err := rb.regNaming.Resolve(target.Endpoint)
	if err != nil {
		return nil, err
	}

	r := &myResolver{
		target:     target,
		cc:         cc,
		watcher:    watcher,
		addrsStore: make(map[string]map[string]*naming.Update),
	}

	r.start()
	return r, nil
}
func (rb *resolverBuilder) Scheme() string { return rb.scheme }

type myResolver struct {
	target     resolver.Target
	cc         resolver.ClientConn
	watcher    naming.Watcher
	addrsStore map[string]map[string]*naming.Update
	wg         sync.WaitGroup
	isClose    bool
}

func (r *myResolver) start() {
	go func() {
		r.wg.Add(1)
		for {
			updates, err := r.watcher.Next()
			if err != nil {
				grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
				mylog.Error(err)
			} else {
				var addrs []resolver.Address
				for _, update := range updates {
					switch update.Op {
					case naming.Add: // 添加
						{
							if _, ok := r.addrsStore[r.target.Endpoint][update.Addr]; !ok {
								r.addrsStore[r.target.Endpoint] = make(map[string]*naming.Update)
							}
							if _, ok := r.addrsStore[r.target.Endpoint][update.Addr]; !ok { // 有新加才添加
								r.addrsStore[r.target.Endpoint][update.Addr] = update
								addrs = append(addrs, resolver.Address{Addr: update.Addr})
							}
						}
					case naming.Delete: // 删除
						{
							if _, ok := r.addrsStore[r.target.Endpoint][update.Addr]; !ok {
								r.addrsStore[r.target.Endpoint] = make(map[string]*naming.Update)
							}
							if _, ok := r.addrsStore[r.target.Endpoint][update.Addr]; !ok { // 有新加才添加
								delete(r.addrsStore[r.target.Endpoint], update.Addr) // map 删除
							}
						}
					}
					if dev.IsDev() {
						mylog.Debugf("watcher:%v", tools.JSONDecode(update))
					}
				}

				if len(addrs) > 0 {
					r.cc.UpdateState(resolver.State{Addresses: addrs})
				}
			}

			if r.isClose {
				break
			}
		}
		r.wg.Done()
	}()

}

func (*myResolver) ResolveNow(o resolver.ResolveNowOptions) {
	//fmt.Println(o)
}
func (r *myResolver) Close() {
	r.isClose = true
	r.wg.Wait()
	fmt.Println("Close--------------")
}
