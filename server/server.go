package server

import (
	"net"
	"time"

	"github.com/gmsec/micro/registry"

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
}

type Option func(*Options)

var (
	DefaultAddress                 = ":0"
	DefaultName                    = "go.micro.server"
	DefaultVersion                 = "latest"
	DefaultId                      = uuid.New().String()
	DefaultNamingServer     Server = newNamingServer()
	DefaultRegisterInterval        = time.Second * 30
	DefaultRegisterTTL             = time.Minute

	// NewServer creates a new server
	NewServer func(...Option) Server = newNamingServer
)

func newNamingServer(opts ...Option) Server {
	options := newOptions(opts...)

	if options.Registry == nil {
		options.Registry = &registry.Registry{
			RegNaming: registry.NewDNSNamingRegistry(),
		}
	}

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
