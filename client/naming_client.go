package client

import (
	"fmt"
	"sync"

	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/tracer"

	"github.com/xxjwxc/public/mylog"
	"github.com/xxjwxc/public/tools"

	grpc_opentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
)

type namingResolver struct {
	opts Options
	sync.RWMutex
	pool *pool
	once sync.Once
	// marks the serve as started
}

func (c *namingResolver) Init(opts ...Option) error {
	size := c.opts.PoolSize
	ttl := c.opts.PoolTTL

	c.Lock()
	defer c.Unlock()

	for _, opt := range opts {
		opt(&c.opts)
	}

	// update pool configuration if the options changed
	if size != c.opts.PoolSize || ttl != c.opts.PoolTTL {
		c.pool.Lock()
		c.pool.size = c.opts.PoolSize
		c.pool.ttl = int64(c.opts.PoolTTL.Seconds())
		c.pool.Unlock()
	}

	if len(c.opts.serviceName) > 0 && c.opts.Registry != nil {
		// init registry parms
		c.opts.Registry.RegNaming.Init(registry.WithServiceName(c.opts.serviceName),
			registry.WithTimeout(c.opts.RegisterTTL),
		)
	}

	return nil
}
func (c *namingResolver) Options() Options {
	c.RLock()
	opts := c.opts
	c.RUnlock()
	return opts
}

func (c *namingResolver) String() string {
	return c.opts.name
}

// initResolver 注册平衡器
func (c *namingResolver) initResolver() {
	if c.opts.Registry != nil {
		resolver.Register(&resolverBuilder{scheme: c.opts.Scheme, regNaming: c.opts.Registry.RegNaming})
	}
}

// Next connon
func (c *namingResolver) Next() (*poolConn, error) {
	c.once.Do(c.initResolver)

	opt := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithBlock(),
	}
	// 开始注册
	if c.opts.Registry != nil {
		opt = append(opt, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`)) //grpc.WithBalancer(grpc.RoundRobin(c.opts.Registry.RegNaming))
	}
	trace := tracer.GetTracer()
	if trace != nil {
		opt = append(opt, grpc.WithUnaryInterceptor(grpc_opentracing.UnaryClientInterceptor(
			grpc_opentracing.WithTracer(trace),
		)))
	}
	var addr string
	if len(c.opts.serviceName) > 0 {
		addr = fmt.Sprintf("%v:///%v", c.opts.Scheme, c.opts.serviceName)
	} else if len(c.opts.serviceIps) > 0 {
		addr = c.opts.serviceIps[tools.GetRandInt(0, len(c.opts.serviceIps))] // 随机
	}

	cc, err := c.pool.getConn(addr, opt...)
	if err != nil {
		mylog.Error(err)
		return nil, err
	}

	// //建立连接
	// conn, err := grpc.Dial(c.opts.serviceName, grpc.WithInsecure(), grpc.WithBalancer(b), grpc.WithBlock())
	// if err != nil {
	// 	return conn, err
	// }

	// cli := grpc_health_v1.NewHealthClient(conn)
	// go func() {
	// 	for {
	// 		resp, err := cli.Check(context.TODO(), &grpc_health_v1.HealthCheckRequest{})
	// 		if err != nil {
	// 			fmt.Printf("健康检查报错: %+v\n", err)
	// 			// os.Exit(1)
	// 		}
	// 		fmt.Printf("服务健康状态: %+v\n", resp)
	// 		time.Sleep(time.Second * 5)
	// 	}
	// }()

	return cc, err
}

// func Copy() *Client {

// }

func (c *namingResolver) poolMaxStreams() int {
	if c.opts.Context == nil {
		return DefaultPoolMaxStreams
	}
	v := c.opts.Context.Value(poolMaxStreams{})
	if v == nil {
		return DefaultPoolMaxStreams
	}
	return v.(int)
}

func (c *namingResolver) poolMaxIdle() int {
	if c.opts.Context == nil {
		return DefaultPoolMaxIdle
	}
	v := c.opts.Context.Value(poolMaxIdle{})
	if v == nil {
		return DefaultPoolMaxIdle
	}
	return v.(int)
}
