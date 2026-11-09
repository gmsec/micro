package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type pool struct {
	timeOut time.Duration
	size    int
	ttl     int64

	//  max streams on a *poolConn
	maxStreams int
	//  max idle conns
	maxIdle int

	sync.Mutex
	conns map[string]*streamsPool
}

type streamsPool struct {
	//  head of list
	head *poolConn
	//  busy conns list
	busy *poolConn
	//  the siza of list
	count int
	//  idle conn
	idle int
}

type poolConn struct {
	//  grpc conn
	*grpc.ClientConn
	err  error
	addr string

	//  pool and streams pool
	pool    *pool
	sp      *streamsPool
	streams int
	created int64

	//  list
	pre  *poolConn
	next *poolConn
	in   bool
}

func newPool(size int, ttl time.Duration, idle int, ms int, timeOut time.Duration) *pool {
	if ms <= 0 {
		ms = 1
	}
	if idle < 0 {
		idle = 0
	}
	if size < 0 { // size==0 表示不池化(每次建连、用完即关),保持原有语义
		size = 0
	}
	if timeOut <= 0 {
		timeOut = DefaultPoolTimeout
	}
	// ttl 以秒存储。不足 1s 时会被截断为 0,
	// 导致 getConn 里"now-created > ttl"恒成立、连接永远被当成过期,所以这里保底 1s。
	sec := int64(ttl.Seconds())
	if sec <= 0 {
		sec = 1
	}
	return &pool{
		size:       size,
		ttl:        sec,
		timeOut:    timeOut,
		maxStreams: ms,
		maxIdle:    idle,
		conns:      make(map[string]*streamsPool),
	}
}

func (p *pool) getConn(addr string, opts ...grpc.DialOption) (*poolConn, error) {
	now := time.Now().Unix()
	p.Lock()
	sp, ok := p.conns[addr]
	if !ok {
		sp = &streamsPool{head: &poolConn{}, busy: &poolConn{}, count: 0, idle: 0}
		p.conns[addr] = sp
	}
	//  while we have conns check streams and then return one
	//  otherwise we'll create a new conn
	conn := sp.head.next
	for conn != nil {
		//  check conn state
		// https://github.com/grpc/grpc/blob/master/doc/connectivity-semantics-and-api.md
		switch conn.GetState() {
		case connectivity.Connecting:
			conn = conn.next
			continue
		case connectivity.Shutdown:
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				if sp.idle > 0 {
					sp.idle--
				}
			}
			conn = next
			continue
		case connectivity.TransientFailure:
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				conn.ClientConn.Close()
				if sp.idle > 0 {
					sp.idle--
				}
			}
			conn = next
			continue
		case connectivity.Ready:
		case connectivity.Idle:
		}
		//  a old conn
		if now-conn.created > p.ttl {
			next := conn.next
			if conn.streams == 0 {
				removeConn(conn)
				conn.ClientConn.Close()
				if sp.idle > 0 {
					sp.idle--
				}
			}
			conn = next
			continue
		}
		//  a busy conn
		if conn.streams >= p.maxStreams {
			next := conn.next
			removeConn(conn)
			addConnAfter(conn, sp.busy)
			conn = next
			continue
		}
		//  a idle conn
		if conn.streams == 0 && sp.idle > 0 {
			sp.idle--
		}
		//  a good conn
		conn.streams++
		p.Unlock()
		return conn, nil
	}
	p.Unlock()

	//  create new conn
	p.Lock()
	timeOut := p.timeOut // 在锁内读取,避免与 Init 并发修改产生数据竞争
	p.Unlock()
	if timeOut <= 0 {
		timeOut = DefaultPoolTimeout
	}
	ctx, cel := context.WithTimeout(context.Background(), timeOut)
	defer cel()
	cc, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		//if err == context.DeadlineExceeded { // 超时
		return nil, fmt.Errorf("[%v]:%v", addr, err.Error())
	}
	conn = &poolConn{cc, nil, addr, p, sp, 1, time.Now().Unix(), nil, nil, false}

	//  add conn to streams pool
	p.Lock()
	if sp.count < p.size {
		addConnAfter(conn, sp.head)
	}
	p.Unlock()

	return conn, nil
}

func (p *pool) release(addr string, conn *poolConn, err error) {
	if conn == nil {
		return
	}
	pl, sp, created := conn.pool, conn.sp, conn.created
	if pl == nil || sp == nil || conn.ClientConn == nil {
		return
	}

	pl.Lock()
	//  try to add conn
	if !conn.in && sp.count < pl.size {
		addConnAfter(conn, sp.head)
	}
	if !conn.in {
		pl.Unlock()
		conn.ClientConn.Close()
		return
	}
	//  a busy conn
	if conn.streams >= pl.maxStreams {
		removeConn(conn)
		addConnAfter(conn, sp.head)
	}
	conn.streams--
	//  if streams == 0, we can do something
	if conn.streams == 0 {
		//  1. it has errored
		//  2. too many idle conn or
		//  3. conn is too old
		now := time.Now().Unix()
		if err != nil || sp.idle >= pl.maxIdle || now-created > pl.ttl {
			removeConn(conn)
			pl.Unlock()
			conn.ClientConn.Close()
			return
		}
		sp.idle++
	}
	pl.Unlock()
}

// Close 归还连接。
func (conn *poolConn) Close() {
	if conn == nil {
		return
	}
	conn.pool.release(conn.addr, conn, conn.err)
}

// CloseWithError 归还连接,并把本次调用的错误记录下来:
// 出错的连接不会被放回空闲池,避免后续请求复用到坏连接。
func (conn *poolConn) CloseWithError(err error) {
	if conn == nil {
		return
	}
	if err != nil {
		conn.err = err
	}
	conn.Close()
}

func removeConn(conn *poolConn) {
	if conn.pre != nil {
		conn.pre.next = conn.next
	}
	if conn.next != nil {
		conn.next.pre = conn.pre
	}
	conn.pre = nil
	conn.next = nil
	conn.in = false
	conn.sp.count--
	return
}

func addConnAfter(conn *poolConn, after *poolConn) {
	conn.next = after.next
	conn.pre = after
	if after.next != nil {
		after.next.pre = conn
	}
	after.next = conn
	conn.in = true
	conn.sp.count++
	return
}
