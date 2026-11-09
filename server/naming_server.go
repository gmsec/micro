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
func (s *namingResolver) Start() error {
	gs := s.GetServer()
	s.Lock()
	defer s.Unlock()
	lis := s.opts.getListener()

	if s.started {
		return nil
	}

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

	// 开始注册
	reg := s.opts.Registry.RegNaming
	if reg != nil {
		if err := reg.Register(s.opts.Address, nil); err != nil {
			mylog.ErrorString("gRPC Server register error:")
			mylog.Error(err)
		}
	}

	// 健康检查
	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, hsrv)
	hsrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.health = hsrv

	// micro: go ts.Accept(s.accept)
	go func() {
		mylog.Info(fmt.Sprintf("grpc server in: %v", s.opts.Address))
		if err := gs.Serve(lis); err != nil {
			mylog.ErrorString("gRPC Server start error:")
			mylog.Error(err)
		}
	}()

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

	tracer.CloseTracer() // 关闭链路追踪

	// 3. 留出摘流时间后再停止 gRPC 监听(时长可配置,默认 1s)
	if delay > 0 {
		time.Sleep(delay)
	}
	stopGrpc(s.GetServer(), stopTimeout)

	s.Lock()
	s.started = false
	s.health = nil
	s.Unlock()

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
