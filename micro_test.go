package micro

import (
	"fmt"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	// reg := registry.NewDNSNamingRegistry()
	// 初始化服务
	service := NewService(
		WithName("lp.srv.eg1"),
		// micro.WithRegisterTTL(time.Second*30),      //指定服务注册时间
		WithRegisterInterval(time.Second*15), //让服务在指定时间内重新注册
		//micro.WithRegistryNameing(reg),
	)

	// server
	go func() {
		// RegisterHelloServer(service.Server(), &hello{})
		// run server
		if err := service.Run(); err != nil {
			panic(err)
		}
		fmt.Println("stop service")
	}()

	// client
	SetClientServiceName("proto.Hello", "lp.srv.eg1") // set client group

}
