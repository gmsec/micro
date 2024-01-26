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
	//exit    chan chan error
}

// NewNamingResolver new one
func NewNamingResolver(opts ...Option) *namingResolver {
	resp := &namingResolver{}

	resp.Init(opts...)

	// if resp.opts.Registry == nil {
	// 	resp.opts.Registry = &registry.Registry{
	// 		RegNaming: registry.NewDNSNamingRegistry(),
	// 	}
	// }

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
	if err := reg.Register(s.opts.Address, nil); err != nil {
		mylog.ErrorString("gRPC Server register error:")
		mylog.Error(err)
	}

	// 健康检查
	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, hsrv)
	hsrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

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
	s.RUnlock()

	gs := s.GetServer()
	reg := s.opts.Registry.RegNaming
	reg.Deregister()
	tracer.CloseTracer() // 关闭链路追踪

	// paus one second
	select {
	case <-time.After(time.Second):
		gs.Stop()
	}

	s.Lock()
	s.started = false
	s.Unlock()

	return nil
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
	s.opts.Address = add
}

func (s *namingResolver) GetAddress() string {
	return s.opts.Address
}
