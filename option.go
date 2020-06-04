package micro

import (
	"context"
	"time"

	"github.com/gmsec/micro/client"
	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/server"
)

// Options ...
type Options struct {
	// Broker    broker.Broker
	// Cmd       cmd.Cmd
	Client   client.Client
	Server   server.Server
	Registry *registry.Registry

	// Registry  registry.Registry
	// Transport transport.Transport

	// // Before and After funcs
	// BeforeStart []func() error
	// BeforeStop  []func() error
	// AfterStart  []func() error
	// AfterStop   []func() error

	// // Other options for implementations of the interface
	// // can be stored in a context
	Context context.Context
}

// newOptions default of option define
func newOptions(opts ...Option) Options {
	// reg := registry.NewDNSNamingRegistry()
	opt := Options{
		Client: client.DefaultNamingClient,
		Server: server.DefaultNamingServer,
		// Registry: &registry.Registry{
		// 	RegNaming: reg,
		// },
		Context: context.Background(),
	}

	for _, o := range opts {
		o(&opt)
	}

	return opt
}

// WithName of the service . Specify service name (Group)
func WithName(n string) Option {
	return func(o *Options) {
		o.Server.Init(server.Name(n))
		o.Client.Init(client.WithServiceName(n))
		//o.Server.Name = n
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
		o.Server = server.DefaultNamingServer
		o.Client = client.DefaultNamingClient
		o.Registry = &registry.Registry{
			RegNaming: reg,
		}
		o.Server.Init(server.WithRegistryNaming(o.Registry.RegNaming))
		o.Client.Init(client.WithRegistryNaming(o.Registry.RegNaming))
	}
}
