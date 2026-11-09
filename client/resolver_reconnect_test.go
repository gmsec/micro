package client

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gmsec/micro/naming"
	"google.golang.org/grpc/resolver"
)

var errFakeNaming = errors.New("fake naming failure")

type fakeStep struct {
	updates []*naming.Update
	err     error
}

// fakeWatcher 按脚本返回事件;脚本用完后阻塞直到 Close,
// 用来模拟"注册中心暂时没有新事件"。
type fakeWatcher struct {
	mu     sync.Mutex
	steps  []fakeStep
	i      int
	closed chan struct{}
	once   sync.Once
}

func newFakeWatcher(steps ...fakeStep) *fakeWatcher {
	return &fakeWatcher{steps: steps, closed: make(chan struct{})}
}

func (f *fakeWatcher) Next() ([]*naming.Update, error) {
	f.mu.Lock()
	i := f.i
	if i >= len(f.steps) {
		f.mu.Unlock()
		<-f.closed
		return nil, context.Canceled
	}
	f.i++
	f.mu.Unlock()

	s := f.steps[i]
	return s.updates, s.err
}

func (f *fakeWatcher) Close() { f.once.Do(func() { close(f.closed) }) }

// fakeClientConn 嵌入 resolver.ClientConn,只覆盖测试关心的 UpdateState。
// 官方文档也是这么推荐的:接口以后可能加方法,嵌入可以让测试不跟着改。
type fakeClientConn struct {
	resolver.ClientConn
	mu    sync.Mutex
	addrs []string
}

func (c *fakeClientConn) UpdateState(s resolver.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addrs = c.addrs[:0]
	for _, a := range s.Addresses {
		c.addrs = append(c.addrs, a.Addr)
	}
	return nil
}

func (c *fakeClientConn) ReportError(error) {}

func (c *fakeClientConn) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.addrs...)
}

func hasAddr(addrs []string, want string) bool {
	for _, a := range addrs {
		if a == want {
			return true
		}
	}
	return false
}

func waitUntil(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// TestResolverReconnect 验证 watcher 出错后 resolver 不会退出,
// 而是保留旧地址、退避后重建 watcher 继续推送。
func TestResolverReconnect(t *testing.T) {
	first := newFakeWatcher(
		fakeStep{err: errFakeNaming}, // 第一次就失败,触发重建
	)
	second := newFakeWatcher(
		fakeStep{updates: []*naming.Update{{Op: naming.Add, Addr: "127.0.0.1:20001"}}},
	)

	cc := &fakeClientConn{}
	r := &myResolver{
		target:     resolver.Target{URL: url.URL{Path: "/svc"}},
		cc:         cc,
		watcher:    first,
		addrsStore: make(map[string]*naming.Update),
		done:       make(chan struct{}),
		resolve: func() (naming.Watcher, error) {
			return second, nil
		},
	}
	r.start()
	defer r.Close()

	// 首次退避 1s,留足余量
	if !waitUntil(5*time.Second, func() bool {
		return hasAddr(cc.snapshot(), "127.0.0.1:20001")
	}) {
		t.Fatalf("watcher 重建后应继续推送地址,实际: %v", cc.snapshot())
	}
}

// TestResolverKeepsAddrsWhileWatcherDown 验证重建期间旧地址表不会被清空,
// 否则注册中心抖一下就会让在途请求全部失败。
func TestResolverKeepsAddrsWhileWatcherDown(t *testing.T) {
	first := newFakeWatcher(
		fakeStep{updates: []*naming.Update{{Op: naming.Add, Addr: "127.0.0.1:20002"}}},
		fakeStep{err: errFakeNaming},
	)
	// 重建永远失败,这样地址表会一直停留在旧值
	cc := &fakeClientConn{}
	r := &myResolver{
		target:     resolver.Target{URL: url.URL{Path: "/svc"}},
		cc:         cc,
		watcher:    first,
		addrsStore: make(map[string]*naming.Update),
		done:       make(chan struct{}),
		resolve: func() (naming.Watcher, error) {
			return nil, errFakeNaming
		},
	}
	r.start()
	defer r.Close()

	if !waitUntil(3*time.Second, func() bool {
		return hasAddr(cc.snapshot(), "127.0.0.1:20002")
	}) {
		t.Fatalf("首次推送应包含旧地址,实际: %v", cc.snapshot())
	}

	// 出错之后仍然保留
	time.Sleep(1500 * time.Millisecond)
	if got := cc.snapshot(); !hasAddr(got, "127.0.0.1:20002") {
		t.Fatalf("watcher 不可用期间应保留最后一份地址,实际: %v", got)
	}
}
