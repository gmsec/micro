package main

import (
	"context"
	"flag"
	"fmt"
	proto "gmicro/rpc"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gmsec/micro"
	"github.com/xxjwxc/gowp/workpool"
	"github.com/xxjwxc/public/mylog"
)

var tag string

func init() {
	addFlag(flag.CommandLine)
}

func addFlag(fs *flag.FlagSet) {
	// env
	fs.StringVar(&tag, "tag", "client", "service or client")
}

func main() {
	flag.Parse()
	// reg := registry.NewDNSNamingRegistry()
	// 初始化服务
	service := micro.NewService(
		micro.WithName("lp.srv.eg1"),
		// micro.WithRegisterTTL(time.Second*30),      //指定服务注册时间
		micro.WithRegisterInterval(time.Second*15), //让服务在指定时间内重新注册
		//micro.WithRegistryNaming(reg),
	)
	if tag == "server" {
		proto.RegisterHelloServer(service.Server(), &hello{})
		// run server
		if err := service.Run(); err != nil {
			panic(err)
		}
		fmt.Println("stop service")
	} else {
		go func() {
			wp := workpool.New(200)     //设置最大线程数
			for i := 0; i < 2000; i++ { //开启20个请求
				wp.Do(func() error {
					run()
					return nil
				})
			}

			wp.Wait()
			fmt.Println("down")
		}()

	}

	wait()
}

func run() {
	micro.SetClientServiceName(proto.GetHelloName(), "lp.srv.eg1") // set client group
	say := proto.GetHelloClient()

	var request proto.HelloRequest
	r := rand.Intn(500)
	request.Name = fmt.Sprintf("%v", r)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		resp, err := say.SayHello(ctx, &request)
		if err != nil {
			mylog.Error(err)
			fmt.Println("==========err:", err)
		}
		fmt.Println(resp)
		time.Sleep(1 * time.Second)
	}
}

func wait() {
	// Go signal notification works by sending `os.Signal`
	// values on a channel. We'll create a channel to
	// receive these notifications (we'll also make one to
	// notify us when the program can exit).
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	// `signal.Notify` registers the given channel to
	// receive notifications of the specified signals.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// This goroutine executes a blocking receive for
	// signals. When it gets one it'll print it out
	// and then notify the program that it can finish.
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()
	// The program will wait here until it gets the
	// expected signal (as indicated by the goroutine
	// above sending a value on `done`) and then exit.
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")

	fmt.Println("down")
}
