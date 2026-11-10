package server

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/tracer"

	"github.com/xxjwxc/public/dev"
	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/grpc_opentracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// var (
// 	DefaultnamingResolver registry.RegistryNaming =
// )
/*
	github.com/gmsec/micro/naming
	grpc 默认支持项,多语言同时接入时推荐使用
*/

type namingResolver struct {
	opts Options
	sync.RWMutex
	// marks the serve as started
	started bool
	// health 健康检查服务,用于停机时先把状态置为 NOT_SERVING
	health *health.Server
	// serveErrCh 回传 gs.Serve 的错误(启动期错误由 Start 直接返回)
	serveErrCh chan error
}

// NewNamingResolver new one
func NewNamingResolver(opts ...Option) *namingResolver {
	resp := &namingResolver{}

	resp.Init(opts...)

	return resp
}

func (s *namingResolver) Options() Options {
	s.RLock()
	opts := s.opts
	s.RUnlock()
	return opts
}

func (s *namingResolver) Init(opts ...Option) error {
	s.Lock()
	defer s.Unlock()

	for _, opt := range opts {
		opt(&s.opts)
	}

	return nil
}

// Handle(Handler) error
// NewHandler(interface{}, ...HandlerOption) Handler
// NewSubscriber(string, interface{}, ...SubscriberOption) Subscriber
// Subscribe(Subscriber) error
// readyListener 在 gRPC Server 首次调用 Accept 时关闭 ready,
// 让 Start() 能等到"监听真正就绪"再返回。
type readyListener struct {
	net.Listener
	once  sync.Once
	ready chan struct{}
}

func (l *readyListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.ready) })
	return l.Listener.Accept()
}

func (s *namingResolver) Start() error {
	gs := s.GetServer()
	s.Lock()
	defer s.Unlock()

	lis, err := s.opts.getListener()
	if err != nil {
		return err
	}

	if s.started {
		return nil
	}

	addr := s.opts.Address

	if s.opts.Registry == nil { // default naming register
		s.opts.Registry = &registry.Registry{
			RegNaming: registry.NewDNSNamingRegistry(),
		}
	}

	// init registry parms
	s.opts.Registry.RegNaming.Init(registry.WithAddrs(s.opts.Address),
		registry.WithNodeID(s.opts.ID),
		registry.WithServiceName(s.opts.Name),
		registry.WithTimeout(s.opts.RegisterTTL),
	)

	// 注册失败不能"打条日志继续启动":那样进程活着但没注册,
	// 调用方永远发现不了本服务,而 Start() 却恒返回 nil。
	reg := s.opts.Registry.RegNaming
	if reg != nil {
		if err := reg.Register(s.opts.Address, nil); err != nil {
			return fmt.Errorf("grpc server register: %w", err)
		}
	}

	// 健康检查
	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, hsrv)
	hsrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.health = hsrv

	// Serve 的错误回传到 errCh;历史实现把错误吞在 goroutine 里,
	// Start() 恒返回 nil,出现"进程活着但没在监听"。
	errCh := make(chan error, 1)
	s.serveErrCh = errCh

	rl := &readyListener{Listener: lis, ready: make(chan struct{})}
	go func() {
		mylog.Info(fmt.Sprintf("grpc server in: %v", addr))
		if err := gs.Serve(rl); err != nil {
			select {
			case errCh <- err:
			default:
			}
			mylog.ErrorString("gRPC Server serve error:")
			mylog.Error(err)
		}
	}()

	// 等待首次 Accept(监听真正就绪)或 Serve 立即失败,再返回。
	select {
	case <-rl.ready:
	case err := <-errCh:
		return err
	}

	s.started = true

	return nil
}
func (s *namingResolver) Stop() error {
	s.RLock()
	if !s.started {
		s.RUnlock()
		return nil
	}
	hsrv := s.health
	delay := s.opts.DeregisterDelay
	stopTimeout := s.opts.StopTimeout
	serveErrCh := s.serveErrCh
	var reg registry.RegNaming
	if s.opts.Registry != nil {
		reg = s.opts.Registry.RegNaming
	}
	s.RUnlock()

	// 1. 先置为不健康,让调用方(负载均衡/健康检查)尽快把流量摘除
	if hsrv != nil {
		hsrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}

	// 2. 注销服务
	if reg != nil {
		if err := reg.Deregister(); err != nil {
			mylog.Error(err)
		}
	}

	// tracer 是进程级共享资源,不再在单个服务 Stop 时关闭:
	// 改为由 micro.Service 在最后一个 Service 停止时统一关闭。

	// 3. 留出摘流时间后再停止 gRPC 监听(时长可配置,默认 1s)
	if delay > 0 {
		time.Sleep(delay)
	}
	stopGrpc(s.GetServer(), stopTimeout)

	s.Lock()
	s.started = false
	s.health = nil
	s.serveErrCh = nil
	s.Unlock()

	// 把 Serve 运行期错误透传给调用方(micro.Run/Stop 能返回的 error)。
	if serveErrCh != nil {
		select {
		case err := <-serveErrCh:
			return err
		default:
		}
	}

	return nil
}

// stopGrpc 先尝试优雅关闭,超过 timeout 后强制停止。
// 历史上的直接 Stop() 会立即断开在途请求,导致客户端收到 Unavailable。
func stopGrpc(gs *grpc.Server, timeout time.Duration) {
	if gs == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		gs.GracefulStop()
	}()

	if timeout <= 0 {
		gs.Stop()
		<-done
		return
	}

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C: // 超时,强制关闭
		gs.Stop()
		<-done
	}
}

// String name string
func (s *namingResolver) String() string {
	return "naming_resolver"
}

// GetServer get server
func (s *namingResolver) GetServer() *grpc.Server {
	s.Lock()
	defer s.Unlock()

	if s.opts.Server == nil {
		trace := tracer.GetTracer()
		if trace != nil {
			s.opts.Server = grpc.NewServer(grpc.UnaryInterceptor(
				grpc_opentracing.UnaryServerInterceptor(grpc_opentracing.WithTracer(trace),
					grpc_opentracing.WithDev(dev.IsDev()))))
		} else {
			s.opts.Server = grpc.NewServer()
		}
	}

	return s.opts.Server
}

// GetListener listener
func (s *namingResolver) GetListener() net.Listener {
	s.Lock()
	defer s.Unlock()
	lis, err := s.opts.getListener()
	if err != nil {
		// 保持 Server 接口的 net.Listener 返回签名,不再用 Fatal 结束进程;
		// 需要错误信息的调用方请用 GetListenerWithError。
		mylog.Error(err)
		return nil
	}
	return lis
}

// GetListenerWithError 与 GetListener 相同,但把监听失败的错误返回给调用方。
func (s *namingResolver) GetListenerWithError() (net.Listener, error) {
	s.Lock()
	defer s.Unlock()
	return s.opts.getListener()
}

// SetListener 设置listener
func (s *namingResolver) SetListener(l net.Listener) bool {
	s.Lock()
	defer s.Unlock()
	return s.opts.setListener(l)
}

func (s *namingResolver) SetAddress(add string) {
	s.Lock()
	defer s.Unlock()
	s.opts.Address = add
}

func (s *namingResolver) GetAddress() string {
	s.RLock()
	defer s.RUnlock()
	return s.opts.Address
}
