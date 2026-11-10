package client

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	pb "github.com/gmsec/micro/client/helloworld"
	"google.golang.org/grpc"
	pgrpc "google.golang.org/grpc"
)

// server is used to implement helloworld.GreeterServer.
type greeterServer struct{}

// SayHello implements helloworld.GreeterServer
func (g *greeterServer) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	if in.Name == "Error" {
		return nil, fmt.Errorf("error")
	}
	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}

func testPool(t *testing.T, size int, ttl time.Duration, idle int, ms int) {
	// setup server
	// 用 127.0.0.1 而非 ":0":后者得到 [::]:port,grpc 会把它交给
	// 环境中的 HTTP_PROXY 代理,代理无法连接通配地址而返回 502。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	s := pgrpc.NewServer()
	pb.RegisterGreeterServer(s, &greeterServer{})

	go s.Serve(l)
	defer s.Stop()

	// zero pool
	p := newPool(size, ttl, idle, ms, 3*time.Second)

	for i := 0; i < 10; i++ {
		// get a conn
		cc, err := p.getConn(l.Addr().String(), grpc.WithInsecure())
		if err != nil {
			t.Fatal(err)
		}

		rsp := pb.HelloReply{}

		err = cc.Invoke(context.TODO(), "/helloworld.Greeter/SayHello", &pb.HelloRequest{Name: "John"}, &rsp)
		if err != nil {
			t.Fatal(err)
		}

		if rsp.Message != "Hello John" {
			t.Fatalf("Got unexpected response %v", rsp.Message)
		}

		// release the conn
		p.release(l.Addr().String(), cc, nil)

		p.Lock()
		if i := p.conns[l.Addr().String()].count; i > size {
			p.Unlock()
			t.Fatalf("pool size %d is greater than expected %d", i, size)
		}
		p.Unlock()
	}
}

func TestGRPCPool(t *testing.T) {
	testPool(t, 0, time.Minute, 10, 2)
	testPool(t, 2, time.Minute, 10, 1)
}

// TestIPAddrClientPoolReuse 验证直连模式(NewIPAddrClient + WithServiceIps)
// 的连接池配置不会在 Init 后退化成 0,且连续两次 Next() 复用同一个连接。
func TestIPAddrClientPoolReuse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	s := pgrpc.NewServer()
	pb.RegisterGreeterServer(s, &greeterServer{})
	go s.Serve(l)
	defer s.Stop()

	c := NewIPAddrClient()
	if err := c.Init(WithServiceIps([]string{l.Addr().String()})); err != nil {
		t.Fatalf("Init: %v", err)
	}

	nr, ok := c.(*namingResolver)
	if !ok {
		t.Fatalf("unexpected client type %T", c)
	}
	nr.pool.Lock()
	size, ttl := nr.pool.size, nr.pool.ttl
	nr.pool.Unlock()
	if size <= 0 || ttl < 1 {
		t.Fatalf("直连池默认值丢失: size=%d ttl=%d", size, ttl)
	}

	conn1, err := c.Next()
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	defer conn1.Close()

	conn2, err := c.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	defer conn2.Close()

	if conn1.ClientConn != conn2.ClientConn {
		t.Fatal("直连连接池未复用:两次 Next 拿到了不同的 ClientConn")
	}
}
