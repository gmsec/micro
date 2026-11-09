package registry

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gmsec/micro/naming"
)

// mdnsSnapshot 在后台持续消费 watcher,返回当前存活地址的快照函数。
func mdnsSnapshot(w naming.Watcher) func() map[string]bool {
	var (
		mu   sync.Mutex
		live = make(map[string]bool)
	)
	go func() {
		for {
			ups, err := w.Next()
			if err != nil {
				return
			}
			mu.Lock()
			for _, u := range ups {
				switch u.Op {
				case naming.Add:
					live[u.Addr] = true
				case naming.Delete:
					delete(live, u.Addr)
				}
			}
			mu.Unlock()
		}
	}()

	return func() map[string]bool {
		mu.Lock()
		defer mu.Unlock()
		cp := make(map[string]bool, len(live))
		for k, v := range live {
			cp[k] = v
		}
		return cp
	}
}

func mdnsEventually(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// TestMDNSConformance 覆盖 mDNS 的三条核心语义:
// 注册可见、多实例可见、实例下线后被移除。
//
// 注意 mDNS 的移除不是实时的:mdns.Query 只收主动查询的应答,
// 收不到对端主动组播的 goodbye,所以只能靠"连续一个容忍窗口都查不到"判删。
// 测试里把容忍窗口缩短到 500ms 来验证这条链路。
func TestMDNSConformance(t *testing.T) {
	oldGrace, oldPoll := mdnsGraceWindow, mdnsPollInterval
	mdnsGraceWindow = 500 * time.Millisecond
	mdnsPollInterval = 100 * time.Millisecond
	defer func() { mdnsGraceWindow, mdnsPollInterval = oldGrace, oldPoll }()

	service := fmt.Sprintf("conformance-mdns-%d", time.Now().UnixNano())

	reg1 := NewDNSNamingRegistry(WithServiceName(service), WithTimeout(200*time.Millisecond))
	addr1 := "127.0.0.1:19101"
	if err := reg1.Register(addr1, nil); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	defer reg1.Deregister()

	w, err := reg1.Resolve(service)
	if err != nil {
		t.Fatalf("Resolve 失败: %v", err)
	}
	defer w.Close()

	snap := mdnsSnapshot(w)

	if !mdnsEventually(10*time.Second, func() bool { return snap()[addr1] }) {
		t.Skip("当前环境无法通过组播发现自己注册的实例(mDNS 不可用)")
	}

	// 第二个实例:新出现的地址应该以 Add 事件推送
	reg2 := NewDNSNamingRegistry(WithServiceName(service), WithTimeout(200*time.Millisecond))
	addr2 := "127.0.0.1:19102"
	if err := reg2.Register(addr2, nil); err != nil {
		t.Fatalf("第二个实例注册失败: %v", err)
	}
	if !mdnsEventually(10*time.Second, func() bool { return snap()[addr1] && snap()[addr2] }) {
		t.Fatal("第二个实例应能被发现")
	}

	// 注销后:查不到该地址,超过容忍窗口应推送 Delete
	if err := reg2.Deregister(); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if !mdnsEventually(10*time.Second, func() bool { return snap()[addr1] && !snap()[addr2] }) {
		t.Fatal("注销后实例应被移除")
	}
}
