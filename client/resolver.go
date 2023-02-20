package client

import (
	"github.com/gmsec/micro/naming"
	"github.com/gmsec/micro/registry"
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
	watcher, err := rb.regNaming.Resolve(target.Endpoint())
	if err != nil {
		return nil, err
	}

	r := &myResolver{
		target:     target,
		cc:         cc,
		watcher:    watcher,
		addrsStore: make(map[string]*naming.Update),
	}

	r.start()
	return r, nil
}
func (rb *resolverBuilder) Scheme() string { return rb.scheme }

type myResolver struct {
	target     resolver.Target
	cc         resolver.ClientConn
	watcher    naming.Watcher
	addrsStore map[string]*naming.Update
	isClose    bool
}

func (r *myResolver) start() {
	go func() {
		for {
			updates, err := r.watcher.Next()
			if err != nil {
				grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
				mylog.Error(err)
				break
			}

			isUpdate := false
			for _, update := range updates {
				switch update.Op {
				case naming.Add: // 添加
					{
						if _, ok := r.addrsStore[update.Addr]; !ok { // 有新加才添加
							r.addrsStore[update.Addr] = update
							isUpdate = true
						}
					}
				case naming.Delete: // 删除
					{
						delete(r.addrsStore, update.Addr) // map 删除
						// todo:r.cc.delete
					}
				}
				mylog.Debugf("watcher(%v):%v", r.target.Endpoint, tools.JSONDecode(update))
			}

			if isUpdate {
				var addrs []resolver.Address
				for _, v := range r.addrsStore {
					addrs = append(addrs, resolver.Address{Addr: v.Addr})
				}
				r.cc.UpdateState(resolver.State{Addresses: addrs})
			}

			if r.isClose {
				r.watcher.Close()
				break
			}
		}
	}()

}

func (*myResolver) ResolveNow(o resolver.ResolveNowOptions) {
	//fmt.Println(o)
}

func (r *myResolver) Close() {
	r.isClose = true
	mylog.Debugf("Close:%v", r.target.Endpoint)
}
