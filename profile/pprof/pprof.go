// Package pprof provides a pprof profiler
package pprof

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/gmsec/micro/profile"
	"github.com/xxjwxc/public/tools"
)

type profiler struct {
	opts profile.Options

	sync.Mutex
	running bool
	exit    chan bool
	wg      sync.WaitGroup

	// where the cpu profile is written
	cpuFile *os.File
	// where the mem profile is written
	memFile *os.File
}

func (p *profiler) writeHeap(f *os.File) {
	defer p.wg.Done()
	defer f.Close()

	t := time.NewTicker(time.Second * 30)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			runtime.GC()
			pprof.WriteHeapProfile(f)
		case <-p.exit:
			return
		}
	}
}

func (p *profiler) Start() error {
	p.Lock()
	defer p.Unlock()

	if p.running {
		return nil
	}

	tmp := "/tmp"
	if runtime.GOOS == "windows" { // windows系统
		tmp = filepath.Join(tools.GetCurrentDirectory(), "/tmp")
		tools.BuildDir(tmp + "/")
	}
	// create exit channel
	p.exit = make(chan bool)

	cpuFile := filepath.Join(tmp, "cpu.pprof")
	memFile := filepath.Join(tmp, "mem.pprof")

	if len(p.opts.Name) > 0 {
		cpuFile = filepath.Join(tmp, p.opts.Name+".cpu.pprof")
		memFile = filepath.Join(tmp, p.opts.Name+".mem.pprof")
	}

	f1, err := os.Create(cpuFile)
	if err != nil {
		return err
	}

	f2, err := os.Create(memFile)
	if err != nil {
		f1.Close() // 避免 cpu 文件句柄泄漏
		return err
	}

	// start cpu profiling
	if err := pprof.StartCPUProfile(f1); err != nil {
		f1.Close()
		f2.Close()
		return err
	}

	// write the heap periodically
	p.wg.Add(1)
	go p.writeHeap(f2)

	// set cpu file
	p.cpuFile = f1
	// set mem file
	p.memFile = f2

	p.running = true

	return nil
}

func (p *profiler) Stop() error {
	p.Lock()
	defer p.Unlock()

	if !p.running || p.exit == nil { // 未启动时直接返回,避免 close(nil) panic
		return nil
	}

	close(p.exit)
	pprof.StopCPUProfile()
	p.cpuFile.Close()

	// 等 writeHeap 退出并关掉 mem 文件,避免它写向已关闭的文件
	p.wg.Wait()

	p.running = false
	p.exit = nil
	p.cpuFile = nil
	p.memFile = nil
	return nil
}

func (p *profiler) String() string {
	return "pprof"
}

// NewProfile new profile
func NewProfile(opts ...profile.Option) profile.Profile {
	var options profile.Options
	for _, o := range opts {
		o(&options)
	}
	p := new(profiler)
	p.opts = options
	return p
}
