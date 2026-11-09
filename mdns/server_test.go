package mdns

import (
	"fmt"
	"testing"
	"time"
)

func TestServer_StartStop(t *testing.T) {
	s := makeService(t)
	serv, err := NewServer(&Config{Zone: s})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer serv.Shutdown()
}

func TestServer_Lookup(t *testing.T) {
	serv, err := NewServer(&Config{Zone: makeServiceWithServiceName(t, "_foobar._tcp")})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer serv.Shutdown()

	entries := make(chan *ServiceEntry, 1)
	found := false
	doneCh := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		defer close(doneCh)
		select {
		case e := <-entries:
			if e.Name != "hostname._foobar._tcp.local." {
				errCh <- fmt.Errorf("bad name: %v", e)
				return
			}
			if e.Port != 80 {
				errCh <- fmt.Errorf("bad port: %v", e)
				return
			}
			if e.Info != "Local web server" {
				errCh <- fmt.Errorf("bad info: %v", e)
				return
			}
			found = true

		case <-time.After(80 * time.Millisecond):
			errCh <- fmt.Errorf("timeout")
		}
	}()

	params := &QueryParam{
		Service: "_foobar._tcp",
		Domain:  "local",
		Timeout: 50 * time.Millisecond,
		Entries: entries,
	}
	err = Query(params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	<-doneCh
	select {
	case err := <-errCh: // 子 goroutine 里的错误不能在子 goroutine 里直接 Fatal
		t.Fatal(err)
	default:
	}
	if !found {
		t.Fatalf("record not found")
	}
}
