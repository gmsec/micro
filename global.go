package micro

import (
	"fmt"
	"sync"

	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/client"
	"github.com/gmsec/micro/registry"
)

var (
	// mut 保护下面两张全局表:_mp(service) 与 _IpAddrMp(client)
	mut        sync.RWMutex
	_mp        = make(map[string]Service)
	_IpAddrMp  = make(map[string]client.Client)
	_RegNaming registry.RegNaming
)

// GetService get service by name.
// 按服务名精确获取已初始化的 Service,找不到返回 nil。
// 只有在 name 为空且进程内恰好只有一个 Service 时才回退返回该 Service,
// 避免多服务进程里"随机挑一个服务"造成难以排查的错调。
func GetService(name string) Service {
	mut.RLock()
	defer mut.RUnlock()

	if len(name) > 0 {
		if s, ok := _mp[name]; ok {
			return s
		}
		mylog.ErrorString(fmt.Sprintf("service [%v] not found, please init it first.", name))
		return nil
	}

	if len(_mp) == 1 {
		for _, v := range _mp {
			return v
		}
	}

	mylog.ErrorString(fmt.Sprintf("service name is empty but there are [%v] services, please specify one.", len(_mp)))
	return nil
}

// GetClient get client by client name.
// client 名必须先通过 SetClientServiceName / SetClientServiceAddr 登记目标服务。
// 未登记时返回一个固定报错的 Client,让调用方拿到明确的错误信息,
// 而不是拿到当前服务的 client 打到自己身上(历史行为)或直接 nil panic。
func GetClient(clientName string) client.Client {
	mut.RLock()
	c, ok := _IpAddrMp[clientName]
	mut.RUnlock()

	if ok && c != nil {
		return c
	}

	err := fmt.Errorf("micro: client [%v] not registered, call micro.SetClientServiceName(%q, <target service name>) during init",
		clientName, clientName)
	mylog.Error(err)

	return client.NewErrorClient(err)
}

// SetClientServiceName set service name with client name.
// clientName 建议使用 proto 全名(如 example.example),serviceName 为注册中心里的服务名(如 gmsec.srv.example)。
func SetClientServiceName(clientName, serviceName string) {
	if len(clientName) == 0 || len(serviceName) == 0 {
		mylog.ErrorString("SetClientServiceName: clientName and serviceName must not be empty")
		return
	}

	mut.Lock()
	defer mut.Unlock()

	if _, ok := _IpAddrMp[clientName]; ok { // 已登记,保留先前的配置
		return
	}

	var opts []client.Option
	if _RegNaming != nil {
		opts = append(opts, client.WithRegistryNaming(_RegNaming))
	}
	tmp := client.NewClient(opts...)
	tmp.Init(client.WithServiceName(serviceName))
	_IpAddrMp[clientName] = tmp
}

// IsExist service existed
func IsExist(name string) bool {
	mut.RLock()
	defer mut.RUnlock()
	_, ok := _mp[name]
	return ok
}

func initService(name string, s *service) {
	mut.Lock()
	defer mut.Unlock()

	_mp[name] = s
	if _RegNaming == nil {
		if reg := s.Options().Registry; reg != nil {
			_RegNaming = reg.RegNaming
		}
	}
}

// SetClientServiceAddr set service address with client name.
// 直连模式:跳过服务发现,直接对给定地址做负载均衡。
func SetClientServiceAddr(clientName string, ips ...string) {
	if len(clientName) == 0 || len(ips) == 0 {
		mylog.ErrorString("SetClientServiceAddr: clientName and ips must not be empty")
		return
	}

	mut.Lock()
	defer mut.Unlock()

	if _, ok := _IpAddrMp[clientName]; ok { // 已登记,保留先前的配置
		return
	}

	tmp := client.NewIPAddrClient()
	tmp.Init(client.WithServiceIps(ips))
	_IpAddrMp[clientName] = tmp
}
