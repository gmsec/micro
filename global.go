package micro

import (
	"fmt"
	"sync"

	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/client"
)

var mut sync.RWMutex
var _mp map[string]Service
var _IpAddrMp map[string]client.Client

var _cliMut sync.RWMutex
var _cliGroup map[string]string

func init() {
	_mp = make(map[string]Service)
	_IpAddrMp = make(map[string]client.Client)
	_cliGroup = make(map[string]string)
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
	var serverName = ""
	_cliMut.RLock()
	defer _cliMut.RUnlock()
	if _, ok := _cliGroup[clientName]; ok {
		serverName = _cliGroup[clientName]
	}

	// if  _IpAddrMp
	c, ok := _IpAddrMp[clientName]
	if ok {
		return c
	}

	s := GetService(serverName)
	if s != nil {
		return s.Client()
	}
	return nil
}

// SetClientServiceName set service name with client name
func SetClientServiceName(clientName, serviceName string) {
	_cliMut.Lock()
	defer _cliMut.Unlock()

	_cliGroup[clientName] = serviceName
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
}

// SetClientServiceAddr set service address with client name
func SetClientServiceAddr(clientName, serviceName string) {
	SetClientServiceName(clientName, serviceName)
	if !IsExist(clientName) {
		mut.RLock()
		defer mut.RUnlock()
		tmp := client.DefaultIPAddrClient
		tmp.Init(client.WithServiceName(serviceName))
		_IpAddrMp[clientName] = tmp
	}
}
