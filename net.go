package go_kcp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errTimeout = errors.New("timeout")
)

type accept struct {
	*conn
	err error
}

var id uint32

type listener struct {
	*net.UDPConn
	mu      sync.Mutex
	ss      map[string]*conn
	actChan chan *accept
}

func (l *listener) Addr() net.Addr {
	return l.UDPConn.RemoteAddr()
}

func (l *listener) Accept() (net.Conn, error) {
	v := <-l.actChan
	return v.conn, v.err
}

func (l *listener) Close() error {
	err := l.UDPConn.Close()
	if err != nil {
		Info("listener close error %v", err)
	}
	for _, v := range l.ss {
		v.err = io.ErrClosedPipe
		err = v.Close()
		if err != nil {
			Info("listener conn close error %v", err)
		}
	}
	return nil
}

func (l *listener) removeConn(addr net.Addr) {
	l.mu.Lock()
	v := l.ss[addr.String()]
	v.err = io.ErrClosedPipe
	delete(l.ss, addr.String())
	l.mu.Unlock()
}

func (l *listener) handleConn() {
	var buffer [2048]byte
	for {
		n, addr, err := l.UDPConn.ReadFrom(buffer[:])
		if err != nil {
			l.actChan <- &accept{err: err}
		}
		l.mu.Lock()
		s, ok := l.ss[addr.String()]
		if !ok {
			s = newConn(addr, l, l.UDPConn)
			if len(l.actChan) < cap(l.actChan) {
				l.actChan <- &accept{conn: s}
			} else {
				Info("conn is limit %v", cap(l.actChan))
			}
			l.ss[addr.String()] = s
		}
		err = s.Input(buffer[:n])
		if err != nil {
			s.err = err
		}
		l.mu.Unlock()
	}
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
	li := &listener{UDPConn: l, ss: map[string]*conn{}, actChan: make(chan *accept, 128)}
	go li.handleConn()
	return li, nil
}

type conn struct {
	udpConn      *net.UDPConn
	remote       net.Addr
	l            *listener
	kcp          *IKcp
	mu           sync.Mutex
	close        chan struct{}
	err          error
	chReadEvent  chan struct{}
	chWriteEvent chan struct{}
	readTimeOut  time.Time
	writeTimeOut time.Time
	buffer       bytes.Buffer
}

func (c *conn) LocalAddr() net.Addr {
	return c.udpConn.LocalAddr()
}

func (c *conn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readTimeOut = t
	c.writeTimeOut = t
	c.notifyReadEvent()
	c.notifyWriteEvent()
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readTimeOut = t
	c.notifyReadEvent()
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeTimeOut = t
	c.notifyWriteEvent()
	return nil
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remote
}

func (c *conn) Read(b []byte) (n int, err error) {
	for {
		if c.err != nil {
			return 0, c.err
		}
		c.mu.Lock()
		if size := c.kcp.Peek(); size > 0 {
			buffer := GetBufferFromPool(4096)
			for size > 0 { //争取一口气读完，释放底层kcp的窗口
				n, err = c.kcp.Recv(buffer, false)
				if err != nil {
					Error("read bytes from kcp queue error")
				}
				if n == 0 {
					panic(fmt.Sprintf("invalid size len:0"))
				}
				size -= n
				c.buffer.Write(buffer[:n])
			}
			PutBufferToPool(buffer)
			n, err = c.buffer.Read(b)
			c.mu.Unlock()
			return n, err
		}

		var timeout *time.Timer
		var ch <-chan time.Time
		if !c.readTimeOut.IsZero() {
			if time.Now().After(c.readTimeOut) {
				c.mu.Unlock()
				return 0, errTimeout
			}

			delay := time.Until(c.readTimeOut)
			timeout = time.NewTimer(delay)
			ch = timeout.C
		}
		c.mu.Unlock()
		select {
		case <-c.chReadEvent:
		case <-ch:
			if timeout != nil {
				timeout.Stop()
			}
			return 0, errTimeout
		case <-c.close:
			return 0, io.ErrClosedPipe
		}
	}
}

func (c *conn) Write(b []byte) (n int, err error) {
	if c.err != nil {
		return 0, c.err
	}
	c.mu.Lock()
	n, err = c.kcp.Send(b)
	c.mu.Unlock()
	select {
	case <-c.chWriteEvent:
		return 0, nil
	case <-c.close:
		return 0, io.ErrClosedPipe
	}
}

func (c *conn) notifyReadEvent() {
	select {
	case c.chReadEvent <- struct{}{}:
	default:
	}
}

func (c *conn) notifyWriteEvent() {
	select {
	case c.chWriteEvent <- struct{}{}:
	default:
	}
}

func (c *conn) tick() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.close:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			c.kcp.Update(uint32(now.UnixMilli()))
			waitsnd := c.kcp.WaitSnd()
			if waitsnd < int(c.kcp.snd_wnd) && waitsnd < int(c.kcp.rmt_wnd) {
				c.notifyWriteEvent()
			}
			c.mu.Unlock()
		}
	}
}

func (c *conn) readLoop() {
	var buffer [2048]byte
	for {
		select {
		case <-c.close:
			return
		default:
		}
		n, _, err := c.udpConn.ReadFrom(buffer[:])
		if err != nil {
			c.err = err
			break
		}
		c.mu.Lock()
		c.err = c.Input(buffer[:n])
		if c.err != nil {
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
	}
}

func (c *conn) Input(data []byte) error {
	err := c.kcp.Input(data)
	c.err = err
	if n := c.kcp.Peek(); n > 0 { //通过查看数据大小，通知读取协程有数据可以去读了
		c.notifyReadEvent()
	}

	waitsnd := c.kcp.WaitSnd()
	//发送队列包的数量小于玩家的发送窗口，同时小于对端的接收窗口
	if waitsnd < int(c.kcp.snd_wnd) && waitsnd < int(c.kcp.rmt_wnd) { //通知玩家能写数据了
		c.notifyWriteEvent()
	}
	return err
}

func (c *conn) NoDelay(noDelay int32, interval int32, resend int32, nc bool) {
	c.mu.Lock()
	c.kcp.NoDelay(noDelay, interval, resend, nc)
	c.mu.Unlock()
}

func (c *conn) SetMtu(mtu uint32) error {
	c.mu.Lock()
	defer c.mu.Lock()
	return c.kcp.SetMtu(mtu)
}

func (c *conn) Close() error {
	sync.OnceFunc(func() {
		close(c.close)
	})
	if c.l != nil {
		c.l.removeConn(c.remote)
		return c.err
	} else {
		return c.udpConn.Close()
	}
}

func newConn(remote net.Addr, l *listener, udpConn *net.UDPConn) *conn {
	con := &conn{kcp: NewIKcp(atomic.LoadUint32(&id), ""), l: l,
		remote:       remote,
		udpConn:      udpConn,
		chWriteEvent: make(chan struct{}, 1),
		chReadEvent:  make(chan struct{}, 1),
		close:        make(chan struct{}, 1),
	}
	con.kcp.SendMsg = func(buf []byte) (n int, err error) {
		return udpConn.WriteTo(buf, remote)
	}
	if l == nil {
		go con.readLoop()
	}
	go con.tick()
	return con
}

func Dial(network, address string) (net.Conn, error) {
	remote, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}
	c, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	co := newConn(remote, nil, c)
	return co, binary.Read(rand.Reader, binary.BigEndian, &co.kcp.conv)
}
