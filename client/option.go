package client

import (
	"context"
	"time"

	"github.com/gmsec/micro/registry"
)

// Options client of options
type Options struct {
	// Connection Pool
	PoolSize int
	PoolTTL  time.Duration
	TimeOut  time.Duration

	Registry *registry.Registry
	// registry
	// The register expiry time
	RegisterTTL time.Duration
	// The interval on which to register
	RegisterInterval time.Duration
	Scheme           string // client gmsec (注册名)

	// Client *grpc.ClientConn

	name        string
	serviceName string
	serviceIps  []string

	// Other options for implementations of the interface
	// can be stored in a context
	Context context.Context
}

func newOptions(options ...Option) Options {
	opts := Options{
		Context:     context.Background(),
		PoolSize:    DefaultPoolSize,
		PoolTTL:     DefaultPoolTTL,
		RegisterTTL: time.Millisecond * 100,
		TimeOut:     DefaultPoolTimeout,
		Scheme:      "gmsec",
	}

	for _, o := range options {
		o(&opts)
	}

	return opts
}

// WithServiceName 设置服务名字
func WithServiceName(name string) Option {
	return func(o *Options) {
		o.serviceName = name
	}
}

// WithServiceIps 设置服务ip列表
func WithServiceIps(ips []string) Option {
	return func(o *Options) {
		o.serviceIps = ips
	}
}

// WithName 设置客户端名字
func WithName(name string) Option {
	return func(o *Options) {
		o.name = name
	}
}

// WithScheme 设置客户端服务名
func WithScheme(name string) Option {
	return func(o *Options) {
		o.Scheme = name
	}
}

// WithPoolSize sets the connection pool size
func WithPoolSize(d int) Option {
	return func(o *Options) {
		o.PoolSize = d
	}
}

// WithPoolTTL sets the connection pool ttl
func WithPoolTTL(d time.Duration) Option {
	return func(o *Options) {
		o.PoolTTL = d
	}
}

// WithRegistryNaming 注册naming 服务发现
func WithRegistryNaming(reg registry.RegNaming) Option {
	return func(o *Options) {
		o.Registry = &registry.Registry{
			RegNaming: reg,
		}
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
