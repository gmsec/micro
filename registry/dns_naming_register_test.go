package registry

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRegister(t *testing.T) {
	dnsName := NewDNSNamingRegistry(WithTimeout(3*time.Second), WithAddrs(":0"), WithServiceName("test.server"), WithNodeID(uuid.New().String()))
	defer dnsName.Deregister()

	fmt.Println(dnsName.String())
	fmt.Println(dnsName.Register("0.0.0.0:12345", 12345))

	watch, err := dnsName.Resolve("test.server")
	fmt.Println(err)

	for i := 0; i < 100; i++ {
		up, e := watch.Next()
		fmt.Println(e)
		for _, v := range up {
			fmt.Println(v)
		}
	}
}
