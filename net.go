package go_kcp

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var id uint32

type listener struct {
	*net.UDPConn
	lock sync.Mutex
	ss   map[net.Addr]*conn
}

func (l *listener) Addr() net.Addr {
	return l.UDPConn.RemoteAddr()
}

func (l *listener) Accept() (net.Conn, error) {
	return newConn(l.UDPConn), nil
}

func Listen(network, address string) (net.Listener, error) {
	addr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}
	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	return &listener{UDPConn: l, ss: map[net.Addr]*conn{}}, nil
}

type conn struct {
	kcp    *IKcp
	ticker *time.Ticker
	mu     sync.Mutex
	err    error
}

func (c *conn) LocalAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

func (c *conn) SetDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (c *conn) SetReadDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (c *conn) RemoteAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

func (c *conn) Read(b []byte) (n int, err error) {
	for {
		if c.err != nil {
			return 0, c.err
		}
		c.mu.Lock()
		n, err = c.kcp.Recv(b, false)
		c.mu.Unlock()
		if err != nil {
			return 0, err
		}
		if n != 0 {
			return n, err
		}
	}
}

func (c *conn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	n, err = c.kcp.Send(b)
	c.mu.Unlock()
	return 0, err
}

func (c *conn) handleMsg() {
	var buffer [4096]byte
	for {
		n, _, err := c.UDPConn.ReadFrom(buffer[:])
		if err != nil {
			c.err = err
			break
		}
		c.mu.Lock()
		err = c.kcp.Input(buffer[:n])
		c.mu.Unlock()
		if err != nil {
			c.err = err
			break
		}
	}
}

func (c *conn) tick() {
	c.ticker = time.NewTicker(10 * time.Millisecond)
	select {
	case <-c.ticker.C:
		c.mu.Lock()
		c.kcp.Flush()
		c.mu.Unlock()
	}
}

func (c *conn) Close() error {
	c.ticker.Stop()
	return c.UDPConn.Close()
}

func newConn(c *net.UDPConn) net.Conn {
	con := &conn{UDPConn: c, kcp: NewIKcp(atomic.LoadUint32(&id), "")}
	con.kcp.SendMsg = func(buf []byte) (n int, err error) {
		return con.Write(buf)
	}
	go con.tick()
	go con.handleMsg()
	return con
}

func Dail(network, address string) (net.Conn, error) {
	addr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return newConn(c), nil
}
