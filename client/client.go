package client

import (
	"time"
)

// Client is the interface used to make requests to services.
// It supports Request/Response via Transport and Publishing via the Broker.
// It also supports bidirectional streaming of requests.
type Client interface {
	Init(...Option) error
	Options() Options
	String() string
	Next() (*poolConn, error) // next connon
	//IsIpAddr() bool
	// Copy() *Client
}

// Option used by the Client
type Option func(*Options)
type poolMaxStreams struct{}
type poolMaxIdle struct{}

var (
	// DefaultPoolMaxStreams maximum streams on a connectioin
	// (20)
	DefaultPoolMaxStreams = 20

	// DefaultPoolMaxIdle maximum idle conns of a pool
	// (50)
	DefaultPoolMaxIdle = 50
	// // DefaultNamingClient is a default client to use out of the box
	//  DefaultNamingClient Client = newNamingClient()
	// // DefaultIPAddrClient is a default client to use addr to connection
	// DefaultIPAddrClient Client = newIPAddrClient()
	// DefaultRetries is the default number of times a request is tried
	DefaultRetries = 1
	// DefaultRequestTimeout is the default request timeout
	DefaultRequestTimeout = time.Second * 5
	// DefaultPoolSize sets the connection pool size
	DefaultPoolSize = 100
	// DefaultPoolTTL sets the connection pool ttl
	DefaultPoolTTL = time.Minute

	// DefaultPoolTimeout sets the connection pool ttl
	DefaultPoolTimeout = 5 * time.Second

	// NewClient returns a new client
	NewClient func(...Option) Client = newNamingClient
	// NewIPAddrClient returns a new client
	NewIPAddrClient func(...Option) Client = newIPAddrClient
)

func newNamingClient(opts ...Option) Client {
	options := newOptions(opts...)

	// if options.Registry == nil {
	// 	options.Registry = &registry.Registry{
	// 		RegNaming: registry.NewDNSNamingRegistry(),
	// 	}
	// }

	rc := &namingResolver{
		opts: options,
		// register:registry.NewDNSNamingRegistry()
		// router:      router,
		// handlers:    make(map[string]Handler),
		// subscribers: make(map[Subscriber][]broker.Subscriber),
		// exit:        make(chan chan error),
		// wg:          wait(options.Context),
	}
	// rc.once.Store(false)
	rc.pool = newPool(options.PoolSize, options.PoolTTL, rc.poolMaxIdle(), rc.poolMaxStreams(), false, options.TimeOut)

	return rc
}

func newIPAddrClient(opts ...Option) Client {
	options := newOptions(opts...)
	rc := &namingResolver{
		//opts: options,
		// register:registry.NewDNSNamingRegistry()
		// router:      router,
		// handlers:    make(map[string]Handler),
		// subscribers: make(map[Subscriber][]broker.Subscriber),
		// exit:        make(chan chan error),
		// wg:          wait(options.Context),
	}
	// rc.once.Store(false)
	rc.pool = newPool(options.PoolSize, options.PoolTTL, rc.poolMaxIdle(), rc.poolMaxStreams(), true, options.TimeOut)

	return rc
}
