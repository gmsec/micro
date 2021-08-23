package registry

import "github.com/gmsec/micro/naming"

// RegNaming grpc自带服务发现
type RegNaming interface {
	Init(...Option) error
	// Options returns the current options
	Options() Options

	Resolve(target string) (naming.Watcher, error)
	//SetNodeID(nodeID string)                             // 本实例节点唯一id
	Register(address string, Metadata interface{}) error // 注册，新加
	Deregister() error                                   // 注销服务
	String() string
}

// Registry some of registry model
type Registry struct {
	RegNaming RegNaming
	// outher model
}

// type RegisterOption func(*RegisterOptions)
// type WatchOption func(*WatchOptions)
