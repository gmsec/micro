package server

import (
	"net"
	"time"

	"github.com/gmsec/micro/registry"

	"github.com/xxjwxc/public/mylog"

	"google.golang.org/grpc"
)

// Options is a simple micro server abstraction
type Options struct {
	Name    string
	Address string
	// Advertise string
	ID      string
	Version string

	// registry
	// The register expiry time
	RegisterTTL time.Duration
	// The interval on which to register
	RegisterInterval time.Duration

	Registry *registry.Registry

	Server   *grpc.Server
	Listener net.Listener
}

// Name Server name
func Name(n string) Option {
	return func(o *Options) {
		o.Name = n
	}
}

// RegisterTTL Register the service with a TTL
func RegisterTTL(t time.Duration) Option {
	return func(o *Options) {
		o.RegisterTTL = t
	}
}

// RegisterInterval Register the service with at interval
func RegisterInterval(t time.Duration) Option {
	return func(o *Options) {
		o.RegisterInterval = t
	}
}

func newOptions(opt ...Option) Options {
	opts := Options{
		// Codecs:           make(map[string]codec.NewCodec),
		// Metadata:         map[string]string{},
		RegisterInterval: DefaultRegisterInterval,
		RegisterTTL:      DefaultRegisterTTL,
	}

	for _, o := range opt {
		o(&opts)
	}

	if len(opts.Address) == 0 {
		opts.Address = DefaultAddress
	}

	if len(opts.Name) == 0 {
		opts.Name = DefaultName
	}

	if len(opts.ID) == 0 {
		opts.ID = DefaultID
	}

	if len(opts.Version) == 0 {
		opts.Version = DefaultVersion
	}

	return opts
}

func (obj *Options) getListener() net.Listener {
	if obj.Listener == nil {
		//起服务
		lis, err := net.Listen("tcp", obj.Address)
		if err != nil {
			mylog.Fatal("failed to listen: ", err)
		}
		obj.Listener = lis
		obj.Address = lis.Addr().String()
	}

	return obj.Listener
}

func (obj *Options) setListener(lis net.Listener) bool {
	if obj.Listener == nil {
		//起服务
		obj.Listener = lis
		obj.Address = lis.Addr().String()
		return true
	}

	return false
}

// WithRegistryNaming 注册naming 服务发现
func WithRegistryNaming(reg registry.RegNaming) Option {
	return func(o *Options) {
		o.Registry = &registry.Registry{
			RegNaming: reg,
		}
	}
}
