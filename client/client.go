package client

import (
	"context"
	"errors"
	"time"
)

// ErrClientNotRegistered client 未登记时返回的错误
var ErrClientNotRegistered = errors.New("micro: client not registered")

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

// ContextClient 是 Client 的可选能力:允许调用方把 ctx 透传到建连过程。
// 刻意不把它加进 Client 接口,以免下游(goplugins、gmsec 等)自定义实现
// 因为接口新增方法而编译失败;支持该能力的实现可通过类型断言使用。
type ContextClient interface {
	NextWithContext(ctx context.Context) (*poolConn, error)
}

// NextWithContext 获取连接并透传 ctx。若 c 实现了 ContextClient 则调用其
// NextWithContext,否则退回旧方法 Next()(保持对自定义 Client 实现的兼容)。
func NextWithContext(c Client, ctx context.Context) (*poolConn, error) {
	if cc, ok := c.(ContextClient); ok {
		return cc.NextWithContext(ctx)
	}
	return c.Next()
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

	rc := &namingResolver{
		opts: options,
	}
	rc.pool = newPool(options.PoolSize, options.PoolTTL, rc.poolMaxIdle(options), rc.poolMaxStreams(options), options.TimeOut)

	return rc
}

func newIPAddrClient(opts ...Option) Client {
	options := newOptions(opts...)
	rc := &namingResolver{
		// 历史实现漏了 opts,导致 rc.opts 为零值 Options:
		// Init 会把 pool.size/ttl 写成 0,直连模式每次 RPC 都重新 Dial。
		opts: options,
	}
	rc.pool = newPool(options.PoolSize, options.PoolTTL, rc.poolMaxIdle(options), rc.poolMaxStreams(options), options.TimeOut)

	return rc
}
