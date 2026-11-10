package registry

import (
	"context"
	"crypto/tls"
	"time"
)

// Options opts define
type Options struct {
	Addrs            []string
	Timeout          time.Duration
	KeepHeartTimeout time.Duration

	// Secure 表示与对端通信启用 TLS(由 WithTLS / WithSecure 设置)。
	Secure bool
	// TLSConfig 是 TLS 配置,由 WithTLS 设置;nil 时保持明文以兼容现有部署。
	TLSConfig *tls.Config

	// Advertise 显式指定对外公告的可达地址(IP:Port)。
	// 监听地址常常是 ":0" / "[::]:port" 这类通配地址,直接写进发现记录
	// 会让客户端 dial 到不可达地址;未指定时由注册实现推导可达 IP。
	Advertise string

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
// 	Service string
// 	// If blank, the watch is for all services
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

// WithAdvertise 显式指定对外公告的可达地址(IP:Port)。
// 监听地址是通配地址时,注册端会改用这里指定的地址写入发现记录。
func WithAdvertise(addr string) Option {
	return func(o *Options) {
		o.Advertise = addr
	}
}

// WithSecure communication with the registry
func WithSecure(b bool) Option {
	return func(o *Options) {
		o.Secure = b
	}
}

// WithTLS 设置 TLS 配置作为框架的 TLS 入口,并自动把 Secure 置位。
// 传入 nil 表示回到明文。默认不调用即保持明文,兼容现有部署。
func WithTLS(t *tls.Config) Option {
	return func(o *Options) {
		o.TLSConfig = t
		o.Secure = t != nil
	}
}

// WithTLSConfig 是 WithTLS 的别名,保持与历史注释里的命名一致。
func WithTLSConfig(t *tls.Config) Option {
	return WithTLS(t)
}

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
