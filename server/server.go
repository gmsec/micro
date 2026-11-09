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
	// DefaultRegisterInterval ...
	DefaultRegisterInterval = time.Second * 30
	// DefaultRegisterTTL ...
	DefaultRegisterTTL = time.Minute
	// DefaultDeregisterDelay 注销服务后等待流量摘除的时长
	DefaultDeregisterDelay = time.Second
	// DefaultStopTimeout 优雅关闭 gRPC server 的最长等待时间
	DefaultStopTimeout = 5 * time.Second

	// NewServer creates a new server
	NewServer func(...Option) Server = newNamingServer
)

// NewID 为服务实例生成一个唯一 ID。
// 历史上用的是包级变量 DefaultID,同一进程内多个服务会共享同一个节点 ID,
// 注册到 etcd/nacos 时互相覆盖,所以改为每个实例单独生成。
func NewID() string {
	return uuid.New().String()
}

func newNamingServer(opts ...Option) Server {
	options := newOptions(opts...)

	return &namingResolver{
		opts: options,
	}
}
