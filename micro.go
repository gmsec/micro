package micro

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/xxjwxc/public/mylog"

	"github.com/gmsec/micro/client"
	"github.com/gmsec/micro/profile"
	"github.com/gmsec/micro/profile/pprof"
	"github.com/gmsec/micro/registry"
	"github.com/gmsec/micro/server"
)

// Service is an interface that wraps the lower level libraries
// within go-micro. Its a convenience method for building
// and initialising services.
type Service interface {
	// The service name
	Name() string
	// Init initialises options
	Init(...Option)
	// Options returns the current options
	Options() Options
	// Client is used to call services
	Client() client.Client
	// Server is for handling requests and events
	Server() server.Server
	// Run the service
	Run() error
	// The service implementation
	String() string
	// stop
	Stop() error

	// stop signal
	NotifyStop()
}

type service struct {
	opts Options
	once sync.Once

	// Before and After funcs
	BeforeStart []func() error
	BeforeStop  []func() error
	AfterStart  []func() error
	AfterStop   []func() error

	cc chan os.Signal
}

// Option  ...
type Option func(*Options)

// NewService newservice
func NewService(opts ...Option) Service {
	return newService(opts...)
}

func newService(opts ...Option) Service {
	options := newOptions(opts...)

	// options.Client = &clientWrapper{
	// 	options.Client,
	// 	metadata.Metadata{
	// 		HeaderPrefix + "From-Service": options.Server.Options().Name,
	// 	},
	// }

	s := &service{
		opts: options,
		cc:   make(chan os.Signal, 1),
	}
	s.Init()

	if !IsExist(s.Name()) {
		initService(s.Name(), s)
	} else {
		mylog.Info(fmt.Sprintf("service [%v] existed.", s.Name()))
	}
	return s
}

func (s *service) Client() client.Client {
	return s.opts.Client
}

func (s *service) Server() server.Server {
	return s.opts.Server
}

func (s *service) Name() string {
	return s.opts.Server.Options().Name
}

// Init initialises options. Additionally it calls cmd.Init
// which parses command line flags. cmd.Init is only called
// on first Init.
func (s *service) Init(opts ...Option) {
	// process options
	for _, o := range opts {
		o(&s.opts)
	}

	if s.opts.Registry == nil {
		s.opts.Server = server.NewServer()
		s.opts.Registry = &registry.Registry{
			RegNaming: registry.NewDNSNamingRegistry(),
		}

		s.opts.Client = client.NewClient(client.WithRegistryNaming(s.opts.Registry.RegNaming))

		s.opts.Server.Init(server.WithRegistryNaming(s.opts.Registry.RegNaming))
		s.opts.Client.Init(client.WithRegistryNaming(s.opts.Registry.RegNaming))
	}

	// init registry parms
	s.opts.Registry.RegNaming.Init(registry.WithAddrs(s.opts.Server.Options().Address),
		registry.WithNodeID(s.opts.Server.Options().ID),
		registry.WithServiceName(s.opts.Server.Options().Name),
		registry.WithTimeout(s.opts.Server.Options().RegisterTTL),
	)

	s.once.Do(func() {
		// setup the plugins
		// TODO:once done
	})
}

// Options all of plugs is options
func (s *service) Options() Options {
	return s.opts
}

func (s *service) Run() error {
	// start the profiler
	// TODO: set as an option to the service, don't just use pprof
	if prof := os.Getenv("MICRO_DEBUG_PROFILE"); len(prof) > 0 {
		service := s.opts.Server.Options().Name
		version := s.opts.Server.Options().Version
		id := s.opts.Server.Options().ID
		profiler := pprof.NewProfile(
			profile.Name(service + "." + version + "." + id),
		)
		if err := profiler.Start(); err != nil {
			return err
		}
		defer profiler.Stop()
	}

	if err := s.Start(); err != nil {
		return err
	}

	// s.cc = make(chan os.Signal, 1)
	signal.Notify(s.cc, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	select {
	// wait on kill signal
	case <-s.cc:
	// wait on context cancel
	case <-s.opts.Context.Done():
	}
	return s.Stop()
}

// String returns name of Server implementation
func (s *service) String() string {
	return "gmsec"
}

// Start starts the default server
func (s *service) Start() error {
	// for _, fn := range s.BeforeStart {
	// 	if err := fn(); err != nil {
	// 		return err
	// 	}
	// }

	///start func
	s.opts.Server.Start()

	// register service

	/// -------

	// for _, fn := range s.AfterStart {
	// 	if err := fn(); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

// NotifyStop 发送停止信号
func (s *service) NotifyStop() {
	s.cc <- syscall.SIGINT
}

// Stop stops the default server
func (s *service) Stop() error {

	var gerr error

	// for _, fn := range s.BeforeStop {
	// 	if err := fn(); err != nil {
	// 		gerr = err
	// 	}
	// }

	s.opts.Server.Stop()

	// for _, fn := range s.AfterStop {
	// 	if err := fn(); err != nil {
	// 		gerr = err
	// 	}
	// }

	return gerr
}
