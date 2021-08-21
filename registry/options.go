package registry

import (
	"context"
	"time"
)

// Options opts define
type Options struct {
	Addrs            []string
	Timeout          time.Duration
	KeepHeartTimeout time.Duration
	// Secure      bool
	// TLSConfig   *tls.Config
	NodeID      string
	ServiceName string
	// Other options for implementations of the interface
	// can be stored in a context
	Context context.Context
}

// Option opts list func
type Option func(*Options)

// type RegisterOptions struct {
// 	TTL time.Duration
// 	// Other options for implementations of the interface
// 	// can be stored in a context
// 	Context context.Context
// }

// type WatchOptions struct {
// 	// Specify a service to watch
// 	// If blank, the watch is for all services
// 	Service string
// 	// Other options for implementations of the interface
// 	// can be stored in a context
// 	Context context.Context
// }

// WithAddrs is the registry addresses to use
func WithAddrs(addrs ...string) Option {
	return func(o *Options) {
		o.Addrs = addrs
	}
}

// WithTimeout set timeout
func WithTimeout(t time.Duration) Option {
	return func(o *Options) {
		o.Timeout = t
	}
}

// WithKeepHeartTimeout set heart timeout
func WithKeepHeartTimeout(t time.Duration) Option {
	return func(o *Options) {
		o.KeepHeartTimeout = t
	}
}

// WithSecure communication with the registry
// func WithSecure(b bool) Option {
// 	return func(o *Options) {
// 		o.Secure = b
// 	}
// }

// // WithTLSConfig TLS Config
// func WithTLSConfig(t *tls.Config) Option {
// 	return func(o *Options) {
// 		o.TLSConfig = t
// 	}
// }

// func RegisterTTL(t time.Duration) RegisterOption {
// 	return func(o *RegisterOptions) {
// 		o.TTL = t
// 	}
// }

// Watch a service
// func WatchService(name string) WatchOption {
// 	return func(o *WatchOptions) {
// 		o.Service = name
// 	}
// }

// WithServiceName 设置服务名字
func WithServiceName(name string) Option {
	return func(o *Options) {
		o.ServiceName = name
	}
}

// WithNodeID 设置节点id
func WithNodeID(nodeID string) Option {
	return func(o *Options) {
		o.NodeID = nodeID
	}
}
