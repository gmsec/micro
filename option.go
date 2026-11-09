package micro

import (
	"context"
	"time"

	"github.com/gmsec/micro/client"
	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/server"
	"github.com/gmsec/micro/tracer"
)

// Options ...
type Options struct {
	Client   client.Client
	Server   server.Server
	Registry *registry.Registry

	// Other options for implementations of the interface
	// can be stored in a context
	Context context.Context
}

// newOptions default of option define
func newOptions(opts ...Option) Options {
	opt := Options{
		Client:  client.NewClient(),
		Server:  server.NewServer(),
		Context: context.Background(),
	}

	for _, o := range opts {
		o(&opt)
	}

	if opt.Registry == nil { // 默认注册 服务发现
		opt.Registry = &registry.Registry{
			RegNaming: registry.NewDNSNamingRegistry(),
		}
		opt.Client.Init(client.WithRegistryNaming(opt.Registry.RegNaming))
		opt.Server.Init(server.WithRegistryNaming(opt.Registry.RegNaming))
	}
	return opt
}

// WithName of the service . Specify service name (Group)
func WithName(n string) Option {
	return func(o *Options) {
		// 副作用放在 Option 被应用时执行:只构造 Option 而不传给 NewService 不应该改变全局状态
		tracer.SetServiceName(n)
		o.Server.Init(server.Name(n))
		o.Client.Init(client.WithServiceName(n))
	}
}

// WithRegisterTTL the service with a TTL
func WithRegisterTTL(t time.Duration) Option {
	return func(o *Options) {
		o.Server.Init(server.RegisterTTL(t))
		o.Client.Init(client.RegisterTTL(t))
	}
}

// WithRegisterInterval the service with at interval.
func WithRegisterInterval(t time.Duration) Option {
	return func(o *Options) {
		o.Server.Init(server.RegisterInterval(t))
	}
}

// WithRegistryNaming Register for naming service discovery
func WithRegistryNaming(reg registry.RegNaming) Option {
	return func(o *Options) {
		// o.Server = server.NewServer()
		// o.Client = client.NewClient()
		o.Registry = &registry.Registry{
			RegNaming: reg,
		}
		o.Server.Init(server.WithRegistryNaming(o.Registry.RegNaming))
		o.Client.Init(client.WithRegistryNaming(o.Registry.RegNaming))
	}
}
