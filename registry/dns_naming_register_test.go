package registry

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRegister(t *testing.T) {
	dnsName := NewDNSNamingRegistry(WithTimeout(3*time.Second), WithAddrs(":0"), WithServiceName("test.server"), WithNodeID(uuid.New().String()))
	defer dnsName.Deregister()

	if err := dnsName.Register("0.0.0.0:12345", 12345); err != nil {
		t.Fatalf("Register: %v", err)
	}

	watch, err := dnsName.Resolve("test.server")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// 历史实现固定循环 100 次 Next(),而每次 Next() 都要等满一次查询超时(3s),
	// 这个用例单跑要 5 分钟。这里只取前几次,拿到结果就结束。
	const maxRound = 3
	for i := 0; i < maxRound; i++ {
		ups, e := watch.Next()
		if e != nil {
			t.Logf("Next(%d): %v", i, e)
			continue
		}
		if len(ups) > 0 {
			t.Logf("Next(%d): %v", i, ups)
			return
		}
	}
	// 机器不支持组播时拿不到任何邻居,这种情况不算测试失败
	t.Log("未拿到邻居列表(组播可能不可用)")
}
