package server

import (
	"net"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

// Server is a simple micro server abstraction
type Server interface {
	Options() Options
	Init(...Option) error
	Start() error
	Stop() error
	String() string
	GetServer() *grpc.Server
	GetListener() net.Listener
	SetListener(net.Listener) bool
	SetAddress(add string)
	GetAddress() string
}

// Option option list
type Option func(*Options)

var (
	// DefaultAddress default addr
	DefaultAddress = ":0"
	// DefaultName default name
	DefaultName = "go.micro.server"
	// DefaultVersion version
	DefaultVersion = "latest"
	// DefaultID node of id
	DefaultID = uuid.New().String()
	// DefaultNamingServer ...
	// DefaultNamingServer Server = newNamingServer()
	// DefaultRegisterInterval ...
	DefaultRegisterInterval = time.Second * 30
	// DefaultRegisterTTL ...
	DefaultRegisterTTL = time.Minute

	// NewServer creates a new server
	NewServer func(...Option) Server = newNamingServer
)

func newNamingServer(opts ...Option) Server {
	options := newOptions(opts...)

	// if options.Registry == nil {
	// 	options.Registry = &registry.Registry{
	// 		RegNaming: registry.NewDNSNamingRegistry(),
	// 	}
	// }

	return &namingResolver{
		opts: options,
		// register:registry.NewDNSNamingRegistry()
		// router:      router,
		// handlers:    make(map[string]Handler),
		// subscribers: make(map[Subscriber][]broker.Subscriber),
		// exit:        make(chan chan error),
		// wg:          wait(options.Context),
	}
}
