package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/tracer"

	"github.com/xxjwxc/public/mylog"
	"github.com/xxjwxc/public/tools"

	// go-grpc-middleware v1.3.0 (2021) 的 opentracing 拦截器与 grpc v1.79.1 混用
	// 属已知兼容风险:本次不升级依赖主版本,仅在此标注,详见 CI 说明。
	grpc_opentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
	c.Lock()
	defer c.Unlock()

	// 在锁内读取旧值:历史实现在加锁前裸读 c.opts,与并发 Init/Options 有数据竞争。
	size := c.opts.PoolSize
	ttl := c.opts.PoolTTL
	timeOut := c.opts.TimeOut

	for _, opt := range opts {
		opt(&c.opts)
	}

	// 与 newOptions 一致的默认值兜底。历史实现里 newIPAddrClient 的 opts 是零值,
	// Init 会把 pool.size/ttl/timeOut 写成 0;ttl 截断成 0 秒后 getConn 里
	// "now-created > ttl" 恒成立,直连模式每次 RPC 都会重新 Dial。
	if c.opts.PoolSize <= 0 {
		c.opts.PoolSize = DefaultPoolSize
	}
	if c.opts.PoolTTL <= 0 {
		c.opts.PoolTTL = DefaultPoolTTL
	}
	if c.opts.TimeOut <= 0 {
		c.opts.TimeOut = DefaultPoolTimeout
	}

	if size != c.opts.PoolSize || ttl != c.opts.PoolTTL || timeOut != c.opts.TimeOut {
		c.pool.Lock()
		c.pool.size = c.opts.PoolSize
		sec := int64(c.opts.PoolTTL.Seconds())
		if sec < 1 { // ttl 以秒存储,写入前保证至少 1s
			sec = 1
		}
		c.pool.ttl = sec
		c.pool.timeOut = c.opts.TimeOut
		c.pool.Unlock()
	}

	if c.opts.Registry != nil && c.opts.Registry.RegNaming != nil {
		// 这里**不能**传 registry.WithServiceName。RegNaming 是进程内共享的单例,
		// 它的 ServiceName 决定注册端往哪个服务名下写、Deregister 时删哪个 key。
		// 客户端要发现的服务只体现在 Resolve(target) 的入参上。
		c.opts.Registry.RegNaming.Init(registry.WithTimeout(c.opts.RegisterTTL))
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
	c.RLock()
	name := c.opts.name
	c.RUnlock()
	return name
}

// initResolver 注册平衡器。使用局部 opts 快照,避免与 Init 并发写 c.opts 竞争。
func (c *namingResolver) initResolver(opts Options) {
	if opts.Registry != nil && opts.Registry.RegNaming != nil {
		resolver.Register(&resolverBuilder{scheme: opts.Scheme, regNaming: opts.Registry.RegNaming})
	}
}

// Next 获取一个连接。旧签名保持不变,以 Options.Context 为父 ctx 包装 NextWithContext。
func (c *namingResolver) Next() (*poolConn, error) {
	ctx := c.Options().Context
	if ctx == nil {
		ctx = context.Background()
	}
	return c.NextWithContext(ctx)
}

// NextWithContext 获取一个连接,并把调用方的 ctx 透传到建连过程(dial)。
// 所有对 c.opts 的读取都在 RLock 下拷贝成局部值后再使用,避免与 Init() 并发写竞争。
func (c *namingResolver) NextWithContext(ctx context.Context) (*poolConn, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// RLock 拷贝一份 opts(与 Options() 一致),后续统一使用局部值。
	c.RLock()
	opts := c.opts
	c.RUnlock()
	// serviceIps 是 slice,结构体值拷贝只复制了 slice header;深拷贝一份,
	// 避免与 Init() 并发替换底层数组时读到被改写的数据或越界。
	if len(opts.serviceIps) > 0 {
		opts.serviceIps = append([]string(nil), opts.serviceIps...)
	}

	c.once.Do(func() { c.initResolver(opts) })

	dialOpts := []grpc.DialOption{
		// 保留 WithBlock:连接池 getConn 依赖 DialContext 返回时连接已就绪
		// (对 Connecting 状态的连接是跳过而非复用)。去掉会让首次并发建连期间
		// 重复 Dial;该改造需同时调整连接池状态机,本次不做。
		grpc.WithBlock(),
	}
	var regNaming registry.RegNaming
	if opts.Registry != nil {
		regNaming = opts.Registry.RegNaming
	}
	if regNaming != nil {
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
		// TLS 入口来自 registry.Options.WithTLS;默认 nil 时保持明文以兼容现有部署。
		if tlsCfg := regNaming.Options().TLSConfig; tlsCfg != nil {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
		} else {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	trace := tracer.GetTracer()
	if trace != nil {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(grpc_opentracing.UnaryClientInterceptor(
			grpc_opentracing.WithTracer(trace),
		)))
	}

	var addr string
	if len(opts.serviceName) > 0 {
		addr = fmt.Sprintf("%v:///%v", opts.Scheme, opts.serviceName)
	} else if n := len(opts.serviceIps); n > 0 {
		idx := tools.GetRandInt(0, n)
		if idx < 0 {
			idx = 0
		}
		if idx >= n { // 防御 GetRandInt 返回上界导致 slice 越界
			idx = n - 1
		}
		addr = opts.serviceIps[idx]
	}

	cc, err := c.pool.getConnContext(ctx, addr, dialOpts...)
	if err != nil {
		mylog.Error(err)
		return nil, err
	}
	return cc, nil
}

func (c *namingResolver) poolMaxStreams(opts Options) int {
	if opts.Context == nil {
		return DefaultPoolMaxStreams
	}
	v := opts.Context.Value(poolMaxStreams{})
	if v == nil {
		return DefaultPoolMaxStreams
	}
	return v.(int)
}

func (c *namingResolver) poolMaxIdle(opts Options) int {
	if opts.Context == nil {
		return DefaultPoolMaxIdle
	}
	v := opts.Context.Value(poolMaxIdle{})
	if v == nil {
		return DefaultPoolMaxIdle
	}
	return v.(int)
}
