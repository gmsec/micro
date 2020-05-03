package micro

import "sync"

var mut sync.RWMutex
var _s *service

// GetService get service
func GetService() Service {
	mut.RLock()
	defer mut.RUnlock()
	return _s
}

func initService(s *service) {
	mut.Lock()
	defer mut.Unlock()
	_s = s
}
