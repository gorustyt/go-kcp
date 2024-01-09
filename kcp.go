package go_kcp

import (
	"container/list"
	"errors"
	"log"
)

const (
	IKCP_RTO_NDL       = 30  // no delay min rto
	IKCP_RTO_MIN       = 100 // normal min rto
	IKCP_RTO_DEF       = 200
	IKCP_RTO_MAX       = 60000
	IKCP_CMD_PUSH      = 81 // cmd: push data
	IKCP_CMD_ACK       = 82 // cmd: ack
	IKCP_CMD_WASK      = 83 // cmd: window probe (ask)
	IKCP_CMD_WINS      = 84 // cmd: window size (tell)
	IKCP_ASK_SEND      = 1  // need to send IKCP_CMD_WASK
	IKCP_ASK_TELL      = 2  // need to send IKCP_CMD_WINS
	IKCP_WND_SND       = 32
	IKCP_WND_RCV       = 128 // must >= max fragment size
	IKCP_MTU_DEF       = 1400
	IKCP_ACK_FAST      = 3
	IKCP_INTERVAL      = 100
	IKCP_OVERHEAD      = 24
	IKCP_DEADLINK      = 20
	IKCP_THRESH_INIT   = 2
	IKCP_THRESH_MIN    = 2
	IKCP_PROBE_INIT    = 7000   // 7 secs to probe window size
	IKCP_PROBE_LIMIT   = 120000 // up to 120 secs to probe window
	IKCP_FASTACK_LIMIT = 5      // max times to trigger fastack

)

type IConfig struct {
	Define0               bool
	Define1               bool
	IKCP_FASTACK_CONSERVE bool
}
type IKcp struct {
	*IConfig
	conv       uint32 // conv 会话ID，
	mtu        uint32 //  mtu 最大传输单元，
	mss        uint32 //  mss 最大分片大小，不大于mtu，
	state      int32  //// state 连接状态，0xFFFFFFFF(即-1)表示断开链接
	snd_una    uint32 // snd_una   第一个未确认的包
	snd_nxt    uint32 // snd_nxt   待发送包的序号
	rcv_nxt    uint32 // rcv_nxt   待接收消息的序号为了保证包的顺序，接收方会维护一个接收窗口。
	ts_recent  uint32
	ts_lastack uint32
	ssthresh   uint32 //  ssthresh  拥塞窗口的阈值，以包为单位（TCP以字节为单位）
	rx_rttval  int32  //ack接收rtt浮动值
	rx_srtt    int32  //ack接收rtt平滑值(smoothed)
	rx_rto     int32  //由ack接收延迟计算出来的复原时间
	rx_minrto  int32  //rx_minrto：最小复原时间
	snd_wnd    uint32 //发送窗口大小
	rcv_wnd    uint32 //接收窗口大小
	rmt_wnd    uint32 //远端接收窗口大小
	cwnd       uint32 //拥塞窗口大小
	probe      uint32
	current    uint32     //当前时间戳
	interval   uint32     //内部flush刷新间隔
	ts_flush   uint32     //下次flush刷新时间戳
	xmit       uint32     //总共重发次数
	nodelay    int32      // nodelay  是否启动无延迟模式
	updated    uint32     // updated  是否调用过update函数的标识（KCP需要上层通过不断的ikcp_update和ikcp_check来驱动KCP的收发过程）
	ts_probe   uint32     // probe_wait 探查窗口需要等待的时间
	probe_wait uint32     // probe_wait 探查窗口需要等待的时间
	dead_link  uint32     // dead_link 最大重传次数，达到则改状态为连接中断
	incr       uint32     // incr   可发送的最大数据量
	snd_queue  *list.List // 发送消息的队列   作用是可以区分发送包的流模式或者段模式
	rcv_queue  *list.List // 接收消息的队列
	snd_buf    *list.List // 发送消息的缓存
	rcv_buf    *list.List // 接收消息的缓存
	acklist    []uint32   // 待发送的ack的列表
	ackcount   uint32     // 本次已收到的ack数量
	user       any        // 存储消息字节流的内存
	buffer     []byte
	fastresend int32 // 触发快速重传的最大次数
	fastlimit  int32
	nocwnd     int32 // nocwnd 取消拥塞控制 (丢包退让，慢启动)
	stream     int32 // stream 是否采用流传输模式
	logmask    int
	SendMsg    func(buf []byte) (n int, err error) //udp 发送消息模块
	writelog   func(log []byte, kcp *IKcp, user any)
}

func NewIKcp(id uint32, user any) *IKcp {
	kcp := &IKcp{}
	kcp.conv = id
	kcp.user = user
	kcp.snd_una = 0
	kcp.snd_nxt = 0
	kcp.rcv_nxt = 0
	kcp.ts_recent = 0
	kcp.ts_lastack = 0
	kcp.ts_probe = 0
	kcp.probe_wait = 0
	kcp.snd_wnd = IKCP_WND_SND
	kcp.rcv_wnd = IKCP_WND_RCV
	kcp.rmt_wnd = IKCP_WND_RCV
	kcp.cwnd = 0
	kcp.incr = 0
	kcp.probe = 0
	kcp.mtu = IKCP_MTU_DEF
	kcp.mss = kcp.mtu - IKCP_OVERHEAD
	kcp.stream = 0
	kcp.buffer = make([]byte, (kcp.mtu+IKCP_OVERHEAD)*3)
	kcp.snd_queue = list.New()
	kcp.rcv_queue = list.New()
	kcp.snd_buf = list.New()
	kcp.rcv_buf = list.New()
	kcp.state = 0
	kcp.acklist = nil
	kcp.ackcount = 0
	kcp.rx_srtt = 0
	kcp.rx_rttval = 0
	kcp.rx_rto = IKCP_RTO_DEF
	kcp.rx_minrto = IKCP_RTO_MIN
	kcp.current = 0
	kcp.interval = IKCP_INTERVAL
	kcp.ts_flush = IKCP_INTERVAL
	kcp.nodelay = 0
	kcp.updated = 0
	kcp.logmask = 0
	kcp.ssthresh = IKCP_THRESH_INIT
	kcp.fastresend = 0
	kcp.fastlimit = IKCP_FASTACK_LIMIT
	kcp.nocwnd = 0
	kcp.xmit = 0
	kcp.dead_link = IKCP_DEADLINK
	return kcp
}

// 控制滑动窗口大小
func (kcp *IKcp) SetWndSize(sndwnd, rcvwnd uint32) {
	if sndwnd > 0 {
		kcp.snd_wnd = sndwnd
	}
	if rcvwnd > 0 { // must >= max fragment size
		kcp.rcv_wnd = max(rcvwnd, IKCP_WND_RCV)
	}
}

func (kcp *IKcp) NoDelay(noDelay int32, interval int32, resend int32, nc int32) {
	if noDelay >= 0 {
		kcp.nodelay = noDelay
		if noDelay != 0 {
			kcp.rx_minrto = IKCP_RTO_NDL
		} else {
			kcp.rx_minrto = IKCP_RTO_MIN
		}
	}
	if interval >= 0 {
		if interval > 5000 {
			interval = 5000
		} else if interval < 10 {
			interval = 10
		}
		kcp.interval = uint32(interval)
	}
	if resend >= 0 {
		kcp.fastresend = resend
	}
	if nc >= 0 {
		kcp.nocwnd = nc
	}

}

func (kcp *IKcp) SetMtu(mtu uint32) error {
	if mtu < 50 || mtu < IKCP_OVERHEAD {
		return errors.New("ERROR_OVERHEAD")
	}

	buffer := make([]byte, (mtu+IKCP_OVERHEAD)*3)
	kcp.mtu = mtu
	kcp.mss = kcp.mtu - IKCP_OVERHEAD
	kcp.buffer = buffer
	return nil
}

func (kcp *IKcp) Interval(interval uint32) {
	if interval > 5000 {
		interval = 5000
	} else if interval < 10 {
		interval = 10
	}
	kcp.interval = interval

}

func (kcp *IKcp) Update(current uint32) {
	var slap int32

	kcp.current = current

	if kcp.updated == 0 {
		kcp.updated = 1
		kcp.ts_flush = kcp.current
	}

	slap = int32(itImeDiff(kcp.current, kcp.ts_flush))

	if slap >= 10000 || slap < -10000 {
		kcp.ts_flush = kcp.current
		slap = 0
	}

	if slap >= 0 {
		kcp.ts_flush += kcp.interval
		if itImeDiff(kcp.current, kcp.ts_flush) >= 0 {
			kcp.ts_flush = kcp.current + kcp.interval
		}
		kcp.Flush()
	}
}

var (
	ErrorQueueIsEmpty = errors.New("ErrorQueueIsEmpty")
)

func (kcp *IKcp) Peek() (length int) {
	if kcp.rcv_queue.Len() == 0 {
		return -1
	}

	seg := kcp.rcv_queue.Front().Value.(*ISeg)
	if seg.frg == 0 {
		return int(seg.len)
	}

	if kcp.rcv_queue.Len() < int(seg.frg)+1 {
		return -1
	}

	for p := kcp.rcv_queue.Front(); p != nil; p = p.Next() {
		seg = p.Value.(*ISeg)
		length += int(seg.len)
		if seg.frg == 0 {
			break
		}
	}

	return length
}

func (kcp *IKcp) Recv(buffer []byte, isPeek bool) (n int, err error) {
	isRecover := false
	var seg *ISeg
	if kcp.rcv_queue.Len() == 0 {
		return 0, nil
	}
	peeksize := kcp.Peek()

	if peeksize < 0 {
		return n, ErrorHasPartPeekSize
	}

	if peeksize > len(buffer) {
		return n, ErrorNeedSizeOverHas
	}

	if kcp.rcv_queue.Len() >= int(kcp.rcv_wnd) {
		isRecover = true
	}

	// merge fragment
	p := kcp.rcv_queue.Front()
	length := uint32(0)
	for p = kcp.rcv_queue.Front(); p != nil; {
		seg = p.Value.(*ISeg)
		p = p.Next()
		if len(buffer) > 0 {
			copy(buffer, seg.data)
		}

		length += seg.len
		fragment := seg.frg
		log.Printf("%v recv sn=%v", IKCP_LOG_RECV, seg.sn)
		if !isPeek {
			kcp.rcv_queue.Remove(p)
		}

		if fragment == 0 {
			break
		}

	}

	if int(length) != peeksize {
		return n, ErrorSegInvalidPeekSize
	}

	// move available data from rcv_buf . rcv_queue
	for kcp.rcv_buf.Len() != 0 {
		v := kcp.rcv_buf.Front()
		seg = v.Value.(*ISeg)
		if seg.sn == kcp.rcv_nxt && kcp.rcv_queue.Len() < int(kcp.rcv_wnd) {
			kcp.rcv_buf.Remove(v)
			kcp.rcv_queue.PushBack(seg)
			kcp.rcv_nxt++
		} else {
			break
		}
	}

	// fast recover
	if kcp.rcv_queue.Len() < int(kcp.rcv_wnd) && isRecover {
		// ready to send back IKCP_CMD_WINS in ikcp_flush
		// tell remote my window size
		kcp.probe |= IKCP_ASK_TELL
	}

	return int(length), nil
}
func (kcp *IKcp) GetUnusedWnd() uint32 {
	if uint32(kcp.rcv_queue.Len()) < kcp.rcv_wnd {
		return kcp.rcv_wnd - uint32(kcp.rcv_queue.Len())
	}
	return 0
}

func (kcp *IKcp) Flush() {
	current := kcp.current
	ptr := uint32(0)
	buffer := uint32(0)
	var size uint32
	var resent, cwnd uint32
	var rtomin uint32
	change := 0
	lost := 0

	// 'ikcp_update' haven't been called.
	if kcp.updated == 0 {
		return
	}
	var seg ISeg
	seg.conv = kcp.conv
	seg.cmd = IKCP_CMD_ACK
	seg.frg = 0
	seg.wnd = kcp.GetUnusedWnd()
	seg.una = kcp.rcv_nxt
	seg.len = 0
	seg.sn = 0
	seg.ts = 0

	// flush acknowledges
	for i := 0; i < int(kcp.ackcount); i++ {
		size = ptr - buffer
		if uint32(size)+IKCP_OVERHEAD > kcp.mtu {
			kcp.Output(kcp.buffer[buffer:ptr])
			ptr = buffer
		}
		seg.sn, seg.ts = kcp.AckGet(i)
		data := seg.Encode()
		copy(kcp.buffer[ptr:], data)
		ptr += uint32(len(data))
	}

	kcp.ackcount = 0

	// probe window size (if remote window size equals zero)
	if kcp.rmt_wnd == 0 {
		if kcp.probe_wait == 0 {
			kcp.probe_wait = IKCP_PROBE_INIT
			kcp.ts_probe = kcp.current + kcp.probe_wait
		} else {
			if itImeDiff(kcp.current, kcp.ts_probe) >= 0 {
				if kcp.probe_wait < IKCP_PROBE_INIT {
					kcp.probe_wait = IKCP_PROBE_INIT
				}

				kcp.probe_wait += kcp.probe_wait / 2
				if kcp.probe_wait > IKCP_PROBE_LIMIT {
					kcp.probe_wait = IKCP_PROBE_LIMIT
				}

				kcp.ts_probe = kcp.current + kcp.probe_wait
				kcp.probe |= IKCP_ASK_SEND
			}
		}
	} else {
		kcp.ts_probe = 0
		kcp.probe_wait = 0
	}

	// flush window probing commands
	if kcp.probe&IKCP_ASK_SEND != 0 {
		seg.cmd = IKCP_CMD_WASK
		size = ptr - buffer
		if size+IKCP_OVERHEAD > kcp.mtu {
			kcp.Output(kcp.buffer[buffer:ptr])
			ptr = buffer
		}
		data := seg.Encode()
		copy(kcp.buffer[ptr:], data)
		ptr += uint32(len(data))
	}

	// flush window probing commands
	if kcp.probe&IKCP_ASK_TELL != 0 {
		seg.cmd = IKCP_CMD_WINS
		size = ptr - buffer
		if size+IKCP_OVERHEAD > kcp.mtu {
			kcp.Output(kcp.buffer[buffer:ptr])
			ptr = buffer
		}
		data := seg.Encode()
		copy(kcp.buffer[ptr:], data)
		ptr += uint32(len(data))
	}

	kcp.probe = 0

	// calculate window size
	cwnd = min(kcp.snd_wnd, kcp.rmt_wnd)
	if kcp.nocwnd == 0 {
		cwnd = min(kcp.cwnd, cwnd)
	}

	// move data from snd_queue to snd_buf
	for itImeDiff(kcp.snd_nxt, kcp.snd_una+cwnd) < 0 {

		if kcp.snd_queue.Len() == 0 {
			break
		}
		v := kcp.snd_queue.Front()
		kcp.snd_queue.Remove(v)
		newseg := v.Value.(*ISeg)
		newseg.node = kcp.snd_buf.PushBack(newseg)
		newseg.conv = kcp.conv
		newseg.cmd = IKCP_CMD_PUSH
		newseg.wnd = seg.wnd
		newseg.ts = current
		newseg.sn = kcp.snd_nxt
		kcp.snd_nxt++
		newseg.una = kcp.rcv_nxt
		newseg.resendts = current
		newseg.rto = kcp.rx_rto
		newseg.fastack = 0
		newseg.xmit = 0
	}

	// calculate resent
	resent = uint32(0xffffffff)
	if kcp.fastresend > 0 {
		resent = uint32(kcp.fastresend)
	}
	rtomin = 0
	if kcp.nodelay == 0 {
		rtomin = uint32(kcp.rx_rto >> 3)
	}

	// flush data segments
	for p := kcp.snd_buf.Front(); p != nil; p = p.Next() {
		segment := p.Value.(*ISeg)
		needsend := false
		if segment.xmit == 0 {
			needsend = true
			segment.xmit++
			segment.rto = kcp.rx_rto
			segment.resendts = current + uint32(segment.rto) + rtomin
		} else if itImeDiff(current, segment.resendts) >= 0 {
			needsend = true
			segment.xmit++
			kcp.xmit++
			if kcp.nodelay == 0 {
				segment.rto += max(segment.rto, kcp.rx_rto)
			} else {
				step := kcp.rx_rto
				if kcp.nodelay < 2 {
					step = segment.rto
				}
				segment.rto += step / 2
			}
			segment.resendts = current + uint32(segment.rto)
			lost = 1
		} else if segment.fastack >= resent { //触发快速重传
			if int32(segment.xmit) <= kcp.fastlimit || kcp.fastlimit <= 0 {
				needsend = true
				segment.xmit++
				segment.fastack = 0
				segment.resendts = current + uint32(segment.rto)
				change++
			}
		}

		if needsend {
			var need uint32
			segment.ts = current
			segment.wnd = seg.wnd
			segment.una = kcp.rcv_nxt

			size = ptr - buffer
			need = uint32(IKCP_OVERHEAD) + segment.len

			if size+need > kcp.mtu {
				kcp.Output(kcp.buffer[buffer:ptr])
				ptr = buffer
			}

			data := segment.Encode()
			copy(kcp.buffer[ptr:], data) //TODO
			if segment.len > 0 {
				copy(kcp.buffer[ptr:], segment.data)
				ptr += segment.len
			}

			if segment.xmit >= kcp.dead_link {
				kcp.state = -1
			}
		}
	}

	// flash remain segments
	size = ptr - buffer
	if size > 0 {
		kcp.Output(kcp.buffer[buffer:ptr])
		ptr = buffer
	}

	if change != 0 { //快恢复，触发快速重传机制
		inflight := kcp.snd_nxt - kcp.snd_una
		kcp.ssthresh = inflight / 2 //表示网络可能出现了阻塞,值变为1半
		if kcp.ssthresh < IKCP_THRESH_MIN {
			kcp.ssthresh = IKCP_THRESH_MIN
		}

		kcp.cwnd = kcp.ssthresh + resent //加  resent 代表快速重传时已经确认接收到了  resent 个重复的数据包
		kcp.incr = kcp.cwnd * kcp.mss
	}

	if lost != 0 { //慢启动，发生了超时重传，使用拥塞发生算法。
		kcp.ssthresh = cwnd / 2
		if kcp.ssthresh < IKCP_THRESH_MIN {
			kcp.ssthresh = IKCP_THRESH_MIN
		}
		kcp.cwnd = 1
		kcp.incr = kcp.mss
	}

	if kcp.cwnd < 1 {
		kcp.cwnd = 1
		kcp.incr = kcp.mss
	}
}

// 上层应用发送数据
func (kcp *IKcp) Send(buffer []byte) (sent int, err error) {
	var seg ISeg
	var count uint32
	var i uint32
	if kcp.mss <= 0 {
		return sent, ErrorMssInvalid
	}
	if len(buffer) == 0 { //没有数据发送
		return
	}

	totalLen := uint32(len(buffer))
	if kcp.stream != 0 {
		if kcp.snd_queue.Len() != 0 {
			olde := kcp.snd_queue.Back()
			old := olde.Value.(*ISeg) //获取最后一个待发送的包
			if old.len < kcp.mss {
				capacity := kcp.mss - old.len
				extend := capacity
				if len(buffer) < int(capacity) { //发送数据小于剩余容量
					extend = uint32(len(buffer)) //直接取发送数据大小
				}
				if len(buffer) > 0 { //将数据添加到最后一个包上面去
					seg.data = append(seg.data, buffer[:extend]...)
				}
				seg.len = old.len + extend
				seg.frg = 0
				totalLen -= extend
				sent = int(extend)
				if extend == uint32(len(buffer)) {
					buffer = buffer[:0]
				} else {
					buffer = buffer[extend:]
				}

			}
		}
		if len(buffer) == 0 {
			return sent, nil
		}
	}

	if uint32(len(buffer)) <= kcp.mss { //刚好够一个包
		count = 1
	} else {
		count = (uint32(len(buffer)) + kcp.mss - 1) / kcp.mss //相当于math.ceil()
	}

	if count >= IKCP_WND_RCV { //分包数量已经超过接收窗口最大值
		if kcp.stream != 0 && sent > 0 {
			return sent, nil
		}

		return sent, ErrorsFragLimit
	}

	if count == 0 {
		count = 1
	}

	// fragment
	for i = 0; i < count; i++ {
		size := uint32(len(buffer))
		if len(buffer) > int(kcp.mss) {
			size = kcp.mss
		}
		seg = ISeg{}
		if len(buffer) > 0 {
			seg.data = buffer[:size]
		}
		seg.len = size
		if kcp.stream == 0 { //分段id反过来，分配，这样可以通过frg==0来判断是不是最后一个包
			seg.frg = count - i - 1
		}
		seg.node = kcp.snd_queue.PushBack(&seg)
		if size < uint32(len(buffer)) {
			buffer = buffer[size:]
		}
		sent += int(size)
	}

	return sent, nil
}

// ---------------------------------------------------------------------
// parse ack
// ---------------------------------------------------------------------
func (kcp *IKcp) UpdateAck(rtt int32) {
	//RTT指的是数据发送时刻到接收到确认的时刻的差值，也就是包的往返时间。
	rto := int32(0)
	if kcp.rx_srtt == 0 {
		kcp.rx_srtt = rtt
		kcp.rx_rttval = rtt / 2
	} else {
		delta := rtt - kcp.rx_srtt
		if delta < 0 {
			delta = -delta
		}
		kcp.rx_rttval = (3*kcp.rx_rttval + delta) / 4
		kcp.rx_srtt = (7*kcp.rx_srtt + rtt) / 8
		if kcp.rx_srtt < 1 {
			kcp.rx_srtt = 1
		}
	}
	rto = kcp.rx_srtt + int32(max(kcp.interval, uint32(4*kcp.rx_rttval)))
	//RTO指超时重传时间
	kcp.rx_rto = bound(kcp.rx_minrto, rto, IKCP_RTO_MAX)

}

func (kcp *IKcp) ShrinkBuf() {
	p := kcp.snd_buf.Front()
	if p != nil {
		kcp.snd_una = p.Value.(*ISeg).sn
	} else {
		kcp.snd_una = kcp.snd_nxt
	}
}

func (kcp *IKcp) ParseAck(sn uint32) {
	if itImeDiff(sn, kcp.snd_una) < 0 || itImeDiff(sn, kcp.snd_nxt) >= 0 { //序列号应该在这个之间
		return
	}

	for p := kcp.snd_buf.Front(); p != nil; p = p.Next() { //删除已经收到ack 的消息
		seg := p.Value.(*ISeg)
		if sn == seg.sn {
			kcp.snd_buf.Remove(p)
			break
		}
		if itImeDiff(sn, seg.sn) < 0 {
			break
		}
	}
}

// 将小于una 的数据报全部删除
func (kcp *IKcp) ParseUna(una uint32) {
	for v := kcp.snd_buf.Front(); v != nil; v = v.Next() {
		if una > v.Value.(*ISeg).sn {
			kcp.snd_buf.Remove(v)
		} else {
			break
		}
	}
}

func (kcp *IKcp) ParseFastAck(sn uint32, ts uint32) {

	if itImeDiff(sn, kcp.snd_una) < 0 || itImeDiff(sn, kcp.snd_nxt) >= 0 {
		return
	}

	for p := kcp.snd_buf.Front(); p != nil; p = p.Next() {
		seg := p.Value.(*ISeg)
		if itImeDiff(sn, seg.sn) < 0 {
			break
		} else if sn != seg.sn {
			if kcp.IKCP_FASTACK_CONSERVE {
				seg.fastack++
			} else {
				if itImeDiff(ts, seg.ts) >= 0 {
					seg.fastack++
				}

			}
		}
	}
}

func (kcp *IKcp) AckPush(sn uint32, ts uint32) {
	kcp.acklist = append(kcp.acklist, sn, ts)
	kcp.ackcount++
}

func (kcp *IKcp) AckGet(p int) (sn, ts uint32) {
	sn = kcp.acklist[p*2+0]
	ts = kcp.acklist[p*2+1]
	return
}

func (kcp *IKcp) ParseData(newSeg *ISeg) {

	sn := newSeg.sn
	repeat := false

	if itImeDiff(sn, kcp.rcv_nxt+kcp.rcv_wnd) >= 0 || itImeDiff(sn, kcp.rcv_nxt) < 0 {
		return
	}
	p := kcp.rcv_buf.Front()
	for ; p != nil; p = p.Next() {
		seg := p.Value.(*ISeg)
		if seg.sn == sn {
			repeat = true
			break
		}
		if itImeDiff(sn, seg.sn) > 0 {
			break
		}
	}

	if !repeat {
		kcp.rcv_buf.InsertAfter(newSeg, p.Value.(*ISeg).node)
	}
	//#if 0
	//ikcp_qprint("rcvbuf", &kcp.rcv_buf);
	//printf("rcv_nxt=%lu\n", kcp.rcv_nxt);
	//#endif
	for v := kcp.rcv_buf.Front(); v != nil; v = v.Next() {
		seg := v.Value.(*ISeg)
		if seg.sn == kcp.rcv_nxt && uint32(kcp.rcv_queue.Len()) < kcp.rcv_wnd {
			kcp.rcv_buf.Remove(v)
			seg.node = kcp.rcv_queue.PushBack(seg)
			kcp.rcv_nxt++
		} else {
			break
		}
	}

	//#if 0
	//ikcp_qprint("queue", &kcp.rcv_queue);
	//printf("rcv_nxt=%lu\n", kcp.rcv_nxt);
	//#endif

	//#if 1
	////	printf("snd(buf=%d, queue=%d)\n", kcp.nsnd_buf, kcp.nsnd_que);
	////	printf("rcv(buf=%d, queue=%d)\n", kcp.nrcv_buf, kcp.nrcv_que);
	//#endif
}

// output segment
func (kcp *IKcp) Output(data []byte) {
	log.Printf("%v [RO] %v bytes", IKCP_LOG_OUTPUT, len(data))
	if len(data) == 0 {
		return
	}
	kcp.SendMsg(data)
}

// udp底层调用该模块去接收数据
func (kcp *IKcp) Input(data []byte) error {
	prev_una := kcp.snd_una
	maxack := uint32(0)
	latest_ts := uint32(0)
	flag := 0

	log.Printf("[RI] %d bytes", len(data))
	if len(data) == 0 || len(data) < IKCP_OVERHEAD { //如果传入数据为空或者长度小于包头的大小，数据错误
		return ErrorsInvalidKcpHead
	}
	for {
		seg := &ISeg{}
		if len(data) < IKCP_OVERHEAD { //可以处理的消息已经处理完了
			break
		}
		err := seg.Decode(data)
		if err != nil {
			return err
		}
		data = data[IKCP_OVERHEAD:]
		if seg.conv != kcp.conv { //错误的会话id
			return ErrorsInvalidConv
		}
		if seg.cmd != IKCP_CMD_PUSH && seg.cmd != IKCP_CMD_ACK && //错误的指令
			seg.cmd != IKCP_CMD_WASK && seg.cmd != IKCP_CMD_WINS {
			return ErrorSegCmdTypeInvalid
		}

		kcp.rmt_wnd = seg.wnd
		kcp.ParseUna(seg.una)
		kcp.ShrinkBuf()

		if seg.cmd == IKCP_CMD_ACK {
			if kcp.current >= seg.ts {
				kcp.UpdateAck(int32(itImeDiff(kcp.current, seg.ts)))
			}
			kcp.ParseAck(seg.sn)
			kcp.ShrinkBuf()
			if flag == 0 {
				flag = 1
				maxack = seg.sn
				latest_ts = seg.ts
			} else {
				if itImeDiff(seg.sn, maxack) > 0 {
					if kcp.IKCP_FASTACK_CONSERVE {
						maxack = seg.sn
						latest_ts = seg.ts
					} else {
						if itImeDiff(seg.ts, latest_ts) > 0 {
							maxack = seg.sn
							latest_ts = seg.ts
						}
					}
				}
			}
			log.Printf(
				"%v input ack: sn=%v rtt=%v rto=%v", IKCP_LOG_IN_ACK, seg.sn,
				itImeDiff(kcp.current, seg.ts),
				kcp.rx_rto)
		} else if seg.cmd == IKCP_CMD_PUSH {
			log.Printf("%v input psh: sn=%v ts=%v", IKCP_LOG_IN_DATA, seg.sn, seg.ts)
			if itImeDiff(seg.sn, kcp.rcv_nxt+kcp.rcv_wnd) < 0 {
				kcp.AckPush(seg.sn, seg.ts)
				kcp.ParseData(seg)
			}
		} else if seg.cmd == IKCP_CMD_WASK {
			// ready to send back IKCP_CMD_WINS in ikcp_flush
			// tell remote my window size
			kcp.probe |= IKCP_ASK_TELL
			log.Printf("%v input probe", IKCP_LOG_IN_PROBE)
		} else if seg.cmd == IKCP_CMD_WINS {
			// do nothing
			log.Printf("%v input wins: %v", IKCP_LOG_IN_WINS, seg.wnd)
		} else {
			return ErrorSegCmdTypeInvalid
		}

		data = data[seg.len:]
	}

	if flag != 0 {
		kcp.ParseFastAck(maxack, latest_ts)
	}

	if itImeDiff(kcp.snd_una, prev_una) > 0 {
		if kcp.cwnd < kcp.rmt_wnd {
			mss := kcp.mss
			if kcp.cwnd < kcp.ssthresh { //慢启动的算法：当发送方每收到一个 ACK，拥塞窗口 cwnd 的大小就会加 1。
				kcp.cwnd++
				kcp.incr += mss
			} else {
				if kcp.incr < mss {
					kcp.incr = mss
				}
				kcp.incr += (mss*mss)/kcp.incr + (mss / 16)
				if (kcp.cwnd+1)*mss <= kcp.incr {
					if kcp.Define1 {
						kcp.cwnd = 1
						if (kcp.incr+mss-1)/mss > 0 {
							kcp.cwnd = mss
						}
					} else {
						kcp.cwnd++
					}
				}
			}
			if kcp.cwnd > kcp.rmt_wnd {
				kcp.cwnd = kcp.rmt_wnd
				kcp.incr = kcp.rmt_wnd * mss
			}
		}
	}

	return nil
}
