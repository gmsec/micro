package micro

import (
	"fmt"
	"sync"

	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/client"
	"github.com/gmsec/micro/registry"
)

var mut sync.RWMutex
var _mp map[string]Service
var _IpAddrMp map[string]client.Client
var _RegNaming registry.RegNaming

func init() {
	_mp = make(map[string]Service)
	_IpAddrMp = make(map[string]client.Client)
}

// GetService get service
func GetService(name string) Service {
	mut.RLock()
	defer mut.RUnlock()

	if len(name) > 0 {
		s, ok := _mp[name]
		if ok {
			return s
		}
	}

	mylog.Info(fmt.Sprintf("[%v]:not fond ,use traverse mode", name))
	for k, v := range _mp {
		mylog.Info(k)
		return v
	}
	mylog.ErrorString("please init first.")
	return nil
}

// GetClient get client from cliet name
func GetClient(clientName string) client.Client {
	// if  _IpAddrMp
	c, ok := _IpAddrMp[clientName]
	if ok {
		return c
	}

	s := GetService("")
	if s != nil {
		return s.Client()
	}
	return nil
}

// SetClientServiceName set service name with client name
func SetClientServiceName(clientName, serviceName string) {
	if !IsExist(clientName) {
		mut.RLock()
		defer mut.RUnlock()
		var opts []client.Option
		if _RegNaming != nil {
			opts = append(opts, client.WithRegistryNaming(_RegNaming))
		}
		tmp := client.NewClient(opts...)
		tmp.Init(client.WithServiceName(serviceName))
		_IpAddrMp[clientName] = tmp
	}
}

// IsExist existed
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
		_RegNaming = s.Options().Registry.RegNaming
	}
}

// SetClientServiceAddr set service address with client name
func SetClientServiceAddr(clientName string, ips ...string) {
	if !IsExist(clientName) {
		mut.RLock()
		defer mut.RUnlock()
		tmp := client.NewIPAddrClient()
		tmp.Init(client.WithServiceIps(ips))
		_IpAddrMp[clientName] = tmp
	}
}
