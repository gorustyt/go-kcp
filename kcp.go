package go_kcp

import (
	"container/list"
	"encoding/binary"
	"errors"
	"time"
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
 type  ISeg struct {
	node *list.Element
 conv uint32
 cmd uint32
 frg uint32
 wnd uint32
 ts uint32
 sn uint32
 una uint32
 len uint32
 resendts uint32
 rto int32
 fastack uint32
 xmit uint32
 data[1]byte
};

func (seg*ISeg)Encode()(data []byte){
length:=4+1+1+2+4+4+4+4
data=make([]byte,length)
binary.BigEndian.PutUint32(data[:4],seg.conv)
data[4]=uint8(seg.cmd)
data[5]=uint8(seg.frg)
binary.BigEndian.PutUint16(data[6:8],uint16(seg.wnd))
binary.BigEndian.PutUint32(data[8:12],seg.ts)
binary.BigEndian.PutUint32(data[12:16],seg.sn)
binary.BigEndian.PutUint32(data[16:20],seg.una)
binary.BigEndian.PutUint32(data[20:24],seg.len)
return data
}

type IConfig struct {
	Define0 bool
	Define1 bool
	IKCP_FASTACK_CONSERVE bool

}
type IKcp struct {
	*IConfig
	conv, mtu, mss, state                  uint32
	snd_una, snd_nxt, rcv_nxt              uint32
	ts_recent, ts_lastack, ssthresh        uint32
	rx_rttval, rx_srtt, rx_rto, rx_minrto  int32
	snd_wnd, rcv_wnd, rmt_wnd, cwnd, probe uint32
	current, interval, ts_flush, xmit      uint32
	nrcv_buf, nsnd_buf                     uint32
	nrcv_que, nsnd_que                     uint32
	nodelay, updated                       uint32
	ts_probe, probe_wait                   uint32
	dead_link, incr                        uint32
	snd_queue                              list.List
	rcv_queue                              list.List
	snd_buf                                list.List
	rcv_buf                                list.List
	acklist                                *uint32
	ackcount                               uint32
	ackblock                               uint32
	user                                   any
	buffer                                 []byte
	fastresend                             int32
	fastlimit                              int32
	nocwnd, stream                         int32
	logmask                                int
	output                                 func(buf []byte, len int32, kcp *IKcp, user any) int
	writelog                               func(log []byte, kcp *IKcp, user any)
}

func NewIKcb(id uint32, user any) *IKcp {
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
	kcp.snd_queue = list.List{}
	kcp.rcv_queue = list.List{}
	kcp.snd_buf = list.List{}
	kcp.rcv_buf = list.List{}
	kcp.nrcv_buf = 0
	kcp.nsnd_buf = 0
	kcp.nrcv_que = 0
	kcp.nsnd_que = 0
	kcp.state = 0
	kcp.acklist = nil
	kcp.ackblock = 0
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
	kcp.output = nil
	kcp.writelog = nil
	return kcp
}

// 控制滑动窗口大小
func (k *IKcp) SetWndSize(sndwnd, rcvwnd uint32) {
	if sndwnd > 0 {
		k.snd_wnd = sndwnd
	}
	if rcvwnd > 0 { // must >= max fragment size
		k.rcv_wnd = Max(rcvwnd, IKCP_WND_RCV)
	}
}

func (k *IKcp) NoDelay(noDelay uint32, interval uint32, resend int32, nc int32) {
	if noDelay >= 0 {
		k.nodelay = noDelay
		if noDelay != 0 {
			k.rx_minrto = IKCP_RTO_NDL
		} else {
			k.rx_minrto = IKCP_RTO_MIN
		}
	}
	if interval >= 0 {
		if interval > 5000 {
			interval = 5000
		} else if interval < 10 {
			interval = 10
		}
		k.interval = interval
	}
	if resend >= 0 {
		k.fastresend = resend
	}
	if nc >= 0 {
		k.nocwnd = nc
	}

}

func (k *IKcp) SetMtu(mtu uint32) error {
	if mtu < 50 || mtu < IKCP_OVERHEAD {
		return errors.New("ERROR_OVERHEAD")
	}

	buffer := make([]byte, (mtu+IKCP_OVERHEAD)*3)
	k.mtu = mtu
	k.mss = k.mtu - IKCP_OVERHEAD
	k.buffer = buffer
	return nil
}

func (k *IKcp) Interval(interval uint32) {
	if interval > 5000 {
		interval = 5000
	} else if interval < 10 {
		interval = 10
	}
	k.interval = interval

}

func (k *IKcp) Update(current uint32) {
	var slap int32

	k.current = current

	if k.updated == 0 {
		k.updated = 1
		k.ts_flush = k.current
	}

	slap = int32(itImeDiff(k.current, k.ts_flush))

	if slap >= 10000 || slap < -10000 {
		k.ts_flush = k.current
		slap = 0
	}

	if slap >= 0 {
		k.ts_flush += k.interval
		if itImeDiff(k.current, k.ts_flush) >= 0 {
			k.ts_flush = k.current + k.interval
		}
		k.Flush()
	}
}

var (
	ErrorQueueIsEmpty=errors.New("ErrorQueueIsEmpty")
)
func (kcp *IKcp)Peek()int  {
var p list.List
var  seg ISeg
length := 0;
if (kcp.rcv_queue.Len()==0) {return -1;}

seg = iqueue_entry(kcp.rcv_queue.next, IKCPSEG, node);
if (seg.frg == 0){ return seg.len;}

if (kcp.nrcv_que < seg.frg + 1) {return -1;}

for (p = kcp.rcv_queue.next; p != &kcp.rcv_queue; p = p.next) {
seg = iqueue_entry(p, IKCPSEG, node);
length += seg.len;
if (seg.frg == 0) {break;}
}

return length;
}
func (kcp *IKcp) Recv( buffer []byte,  len int) (n int ,err error){
var p list.List
var ispeek int
if len<0{
	ispeek=1
}
var  peeksize int
var  recover  int
var  seg ISeg
if (kcp.rcv_queue.Len()==0){
	return 0,ErrorQueueIsEmpty
}


if (len < 0) {len = -len;}

peeksize = kcp.Peek()

if (peeksize < 0){return -2;}


if (peeksize > len){return -3;}


if (kcp.nrcv_que >= kcp.rcv_wnd){recover = 1;}


// merge fragment
p:=kcp.rcv_queue.Front()
length:=0
for (len = 0, p = kcp.rcv_queue.next; p != &kcp.rcv_queue; ) {
var  fragment int
seg = iqueue_entry(p, IKCPSEG, node);
p = p.next;

if (buffer) {
memcpy(buffer, seg.data, seg.len);
buffer += seg.len;
}

len += seg.len;
fragment = seg.frg;

if (ikcp_canlog(kcp, IKCP_LOG_RECV)) {
ikcp_log(kcp, IKCP_LOG_RECV, "recv sn=%lu", (unsigned long)seg.sn);
}

if (ispeek == 0) {
iqueue_del(&seg.node);
ikcp_segment_delete(kcp, seg);
kcp.nrcv_que--;
}

if (fragment == 0){break;}

}

assert(len == peeksize);

// move available data from rcv_buf . rcv_queue
for  (! iqueue_is_empty(&kcp.rcv_buf)) {
seg = iqueue_entry(kcp.rcv_buf.next, IKCPSEG, node);
if (seg.sn == kcp.rcv_nxt && kcp.nrcv_que < kcp.rcv_wnd) {
iqueue_del(&seg.node);
kcp.nrcv_buf--;
iqueue_add_tail(&seg.node, &kcp.rcv_queue);
kcp.nrcv_que++;
kcp.rcv_nxt++;
}	else {
break;
}
}

// fast recover
if (kcp.nrcv_que < kcp.rcv_wnd && recover) {
// ready to send back IKCP_CMD_WINS in ikcp_flush
// tell remote my window size
kcp.probe |= IKCP_ASK_TELL;
}

return len;
}
func (kcp *IKcp)  GetUnusedWnd()uint32{
if (kcp.nrcv_que < kcp.rcv_wnd) {
return kcp.rcv_wnd - kcp.nrcv_que;
}
return 0;
}

func (kcp *IKcp) Flush() {
current := kcp.current;
buffer := kcp.buffer;
char *ptr = buffer;
var  size, i int32
var resent, cwnd uint32
var  rtomin uint32
var p list.List
 change := 0;
 lost := 0;
var  seg ISeg

// 'ikcp_update' haven't been called.
if (kcp.updated == 0) {return;}

seg.conv = kcp.conv;
seg.cmd = IKCP_CMD_ACK;
seg.frg = 0;
seg.wnd = kcp.GetUnusedWnd();
seg.una = kcp.rcv_nxt;
seg.len = 0;
seg.sn = 0;
seg.ts = 0;

// flush acknowledges
count := kcp.ackcount;
for i = 0; i < count; i++ {
size =int32(ptr - buffer);
if (size + IKCP_OVERHEAD > kcp.mtu) {
kcp.Output( buffer, size);
ptr = buffer;
}
kcp.AckGet( i, &seg.sn, &seg.ts);
ptr = ikcp_encode_seg(ptr, &seg);
}

kcp.ackcount = 0;

// probe window size (if remote window size equals zero)
if (kcp.rmt_wnd == 0) {
if (kcp.probe_wait == 0) {
kcp.probe_wait = IKCP_PROBE_INIT;
kcp.ts_probe = kcp.current + kcp.probe_wait;
} else {
if (itImeDiff(kcp.current, kcp.ts_probe) >= 0) {
if (kcp.probe_wait < IKCP_PROBE_INIT){kcp.probe_wait = IKCP_PROBE_INIT;}

kcp.probe_wait += kcp.probe_wait / 2;
if (kcp.probe_wait > IKCP_PROBE_LIMIT){
	kcp.probe_wait = IKCP_PROBE_LIMIT;
}

kcp.ts_probe = kcp.current + kcp.probe_wait;
kcp.probe |= IKCP_ASK_SEND;
}
}
}	else {
kcp.ts_probe = 0;
kcp.probe_wait = 0;
}

// flush window probing commands
if (kcp.probe & IKCP_ASK_SEND!=0) {
seg.cmd = IKCP_CMD_WASK;
size = (int)(ptr - buffer);
if (size + IKCP_OVERHEAD > kcp.mtu) {
ikcp_output(kcp, buffer, size);
ptr = buffer;
}
ptr = ikcp_encode_seg(ptr, &seg);
}

// flush window probing commands
if (kcp.probe & IKCP_ASK_TELL) {
seg.cmd = IKCP_CMD_WINS;
size = (ptr - buffer);
if (size + IKCP_OVERHEAD > kcp.mtu) {
kcp.Output(kcp, buffer, size);
ptr = buffer;
}
ptr = ikcp_encode_seg(ptr, &seg);
}

kcp.probe = 0;

// calculate window size
cwnd = Min(kcp.snd_wnd, kcp.rmt_wnd);
if (kcp.nocwnd == 0){ cwnd = Min(kcp.cwnd, cwnd);}

// move data from snd_queue to snd_buf
for  (_itimediff(kcp.snd_nxt, kcp.snd_una + cwnd) < 0) {
var newseg ISeg
if (kcp.snd_queue.Len()==0) {break;}

newseg = iqueue_entry(kcp.snd_queue.next, IKCPSEG, node);

iqueue_del(&newseg.node);
iqueue_add_tail(&newseg.node, &kcp.snd_buf);
kcp.nsnd_que--;
kcp.nsnd_buf++;

newseg.conv = kcp.conv;
newseg.cmd = IKCP_CMD_PUSH;
newseg.wnd = seg.wnd;
newseg.ts = current;
newseg.sn = kcp.snd_nxt;
		kcp.snd_nxt++
newseg.una = kcp.rcv_nxt;
newseg.resendts = current;
newseg.rto =kcp.rx_rto;
newseg.fastack = 0;
newseg.xmit = 0;
}

// calculate resent
resent = (kcp.fastresend > 0)? (IUINT32)kcp.fastresend : 0xffffffff;
rtomin = (kcp.nodelay == 0)? (kcp.rx_rto >> 3) : 0;

// flush data segments
for (p = kcp.snd_buf.next; p != &kcp.snd_buf; p = p.next) {
IKCPSEG *segment = iqueue_entry(p, IKCPSEG, node);
int needsend = 0;
if (segment.xmit == 0) {
needsend = 1;
segment.xmit++;
segment.rto = kcp.rx_rto;
segment.resendts = current + segment.rto + rtomin;
} else if (itImeDiff(current, segment.resendts) >= 0) {
needsend = 1;
segment.xmit++;
kcp.xmit++;
if (kcp.nodelay == 0) {
segment.rto += max(segment.rto, (IUINT32)kcp.rx_rto);
}	else {
IINT32 step = (kcp.nodelay < 2)?
((IINT32)(segment.rto)) : kcp.rx_rto;
segment.rto += step / 2;
}
segment.resendts = current + segment.rto;
lost = 1;
} else if (segment.fastack >= resent) {
if ((int)segment.xmit <= kcp.fastlimit ||
kcp.fastlimit <= 0) {
needsend = 1;
segment.xmit++;
segment.fastack = 0;
segment.resendts = current + segment.rto;
change++;
}
}

if (needsend) {
int need;
segment.ts = current;
segment.wnd = seg.wnd;
segment.una = kcp.rcv_nxt;

size = (int)(ptr - buffer);
need = IKCP_OVERHEAD + segment.len;

if (size + need > kcp.mtu) {
kcp.Output( buffer, size);
ptr = buffer;
}

ptr = ikcp_encode_seg(ptr, segment);

if (segment.len > 0) {
memcpy(ptr, segment.data, segment.len);
ptr += segment.len;
}

if (segment.xmit >= kcp.dead_link) {
kcp.state = (IUINT32)-1;
}
}
}

// flash remain segments
size = (int)(ptr - buffer);
if (size > 0) {
kcp.Output(kcp, buffer, size);
}

// update ssthresh
if (change) {
inflight := kcp.snd_nxt - kcp.snd_una;
kcp.ssthresh = inflight / 2;
if (kcp.ssthresh < IKCP_THRESH_MIN)
kcp.ssthresh = IKCP_THRESH_MIN;
kcp.cwnd = kcp.ssthresh + resent;
kcp.incr = kcp.cwnd * kcp.mss;
}

if (lost!=0) {
kcp.ssthresh = cwnd / 2;
if (kcp.ssthresh < IKCP_THRESH_MIN)
kcp.ssthresh = IKCP_THRESH_MIN;
kcp.cwnd = 1;
kcp.incr = kcp.mss;
}

if (kcp.cwnd < 1) {
kcp.cwnd = 1;
kcp.incr = kcp.mss;
}
}


//---------------------------------------------------------------------
// user/upper level send, returns below zero for error
//---------------------------------------------------------------------
func (kcp *IKcp)Send( buffer []byte)int {
var seg ISeg
var count, i int
 sent := 0;

if (kcp.mss <= 0){
	return -2
}
if (len(buffer) < 0) {return -1;}


// append to previous segment in streaming mode (if possible)
if (kcp.stream != 0) {
if (kcp.snd_queue.Len()!=0) {
IKCPSEG *old = iqueue_entry(kcp.snd_queue.prev, IKCPSEG, node);
if (old.len < kcp.mss) {
 capacity := kcp.mss - old.len;
 extend := (len(buffer) < capacity)? len : capacity;
iqueue_add_tail(&seg.node, &kcp.snd_queue);
memcpy(seg.data, old.data, old.len);
if (len(buffer)>0) {
memcpy(seg.data + old.len, buffer, extend);
buffer += extend;
}
seg.len = old.len + extend;
seg.frg = 0;
len -= extend;
iqueue_del_init(&old.node);
ikcp_segment_delete(kcp, old);
sent = extend;
}
}
if (len(buffer) <= 0) {
return sent;
}
}

if (len(buffer) <= kcp.mss) {
	count = 1;
} else {
	count = (len(buffer) + kcp.mss - 1) / kcp.mss;
}

if (count >= IKCP_WND_RCV) {
if (kcp.stream != 0 && sent > 0){
	return sent;
}

return -2;
}

if (count == 0) {count = 1;}

// fragment
for i = 0; i < count; i++ {
size :=  len(buffer)
if len(buffer) > int(kcp.mss){
	size=int(kcp.mss)
}
seg = ISeg{}
if ( len(buffer) > 0) {
memcpy(seg.data, buffer, size);
seg.data=buffer
}
seg.len = uint32(size);
if (kcp.stream == 0){
	seg.frg =uint32 (count - i - 1)
}

iqueue_init(&seg.node);
iqueue_add_tail(&seg.node, &kcp.snd_queue);
kcp.nsnd_que++;
if (len(buffer)>0) {
buffer += size;
}
len -= size;
sent += size;
}

return sent;
}


//---------------------------------------------------------------------
// parse ack
//---------------------------------------------------------------------
func (kcp *IKcp)UpdateAck( rtt int32) {
 rto := int32(0);
if (kcp.rx_srtt == 0) {
kcp.rx_srtt = rtt;
kcp.rx_rttval = rtt / 2;
}	else {
 delta := rtt - kcp.rx_srtt;
if (delta < 0){ delta = -delta;}
kcp.rx_rttval = (3 * kcp.rx_rttval + delta) / 4;
kcp.rx_srtt = (7 * kcp.rx_srtt + rtt) / 8;
if (kcp.rx_srtt < 1) {kcp.rx_srtt = 1;}
}
rto = kcp.rx_srtt + int32(max(kcp.interval, uint32(4 * kcp.rx_rttval)));
kcp.rx_rto = bound(kcp.rx_minrto, rto, IKCP_RTO_MAX);
}

func (kcp *IKcp) ShrinkBuf() {
struct IQUEUEHEAD *p = kcp.snd_buf.next;
if (p != &kcp.snd_buf) {
IKCPSEG *seg = iqueue_entry(p, IKCPSEG, node);
kcp.snd_una = seg.sn;
}	else {
kcp.snd_una = kcp.snd_nxt;
}
}

func (kcp *IKcp)ParseAck( sn uint32){
struct IQUEUEHEAD *p, *next;

if (itImeDiff(sn, kcp.snd_una) < 0 || itImeDiff(sn, kcp.snd_nxt) >= 0){
	return;
}


for (p = kcp.snd_buf.next; p != &kcp.snd_buf; p = next) {
IKCPSEG *seg = iqueue_entry(p, IKCPSEG, node);
next = p.next;
if (sn == seg.sn) {
iqueue_del(p);
ikcp_segment_delete(kcp, seg);
kcp.nsnd_buf--;
break;
}
if (itImeDiff(sn, seg.sn) < 0) {
break;
}
}
}

func (kcp *IKcp)ParseUna(  una uint32){
struct IQUEUEHEAD *p, *next;
for (p = kcp.snd_buf.next; p != &kcp.snd_buf; p = next) {
IKCPSEG *seg = iqueue_entry(p, IKCPSEG, node);
next = p.next;
if (itImeDiff(una, seg.sn) > 0) {
iqueue_del(p);
ikcp_segment_delete(kcp, seg);
kcp.nsnd_buf--;
}	else {
break;
}
}
}

func (kcp *IKcp)ParseFastAck(  sn uint32,  ts uint32){
struct IQUEUEHEAD *p, *next;

if (itImeDiff(sn, kcp.snd_una) < 0 || itImeDiff(sn, kcp.snd_nxt) >= 0)
return;

for (p = kcp.snd_buf.next; p != &kcp.snd_buf; p = next) {
IKCPSEG *seg = iqueue_entry(p, IKCPSEG, node);
next = p.next;
if (itImeDiff(sn, seg.sn) < 0) {
break;
} else if (sn != seg.sn) {
#ifndef IKCP_FASTACK_CONSERVE
seg.fastack++;
#else
if (itImeDiff(ts, seg.ts) >= 0)
seg.fastack++;
#endif
}
}
}


//---------------------------------------------------------------------
// ack append
//---------------------------------------------------------------------
func (kcp *IKcp)AckPush(  sn uint32,  ts uint32){
 newsize := kcp.ackcount + 1;
IUINT32 *ptr;

if (newsize > kcp.ackblock) {
IUINT32 *acklist;
IUINT32 newblock;

for (newblock = 8; newblock < newsize; newblock <<= 1);
acklist = (IUINT32*)ikcp_malloc(newblock * sizeof(IUINT32) * 2);

if (acklist == nil) {
assert(acklist != nil);
abort();
}

if (kcp.acklist != nil) {
IUINT32 x;
for (x = 0; x < kcp.ackcount; x++) {
acklist[x * 2 + 0] = kcp.acklist[x * 2 + 0];
acklist[x * 2 + 1] = kcp.acklist[x * 2 + 1];
}
ikcp_free(kcp.acklist);
}

kcp.acklist = acklist;
kcp.ackblock = newblock;
}

ptr = &kcp.acklist[kcp.ackcount * 2];
ptr[0] = sn;
ptr[1] = ts;
kcp.ackcount++;
}

func (kcp *IKcp)AckGet( p int, sn []uint32, ts []uint32){
if (sn) {
	sn[0] = kcp.acklist[p * 2 + 0];
}
if (ts) {
	ts[0] = kcp.acklist[p * 2 + 1];
}
}

func (kcp *IKcp)ParseData( newseg *ISeg){
struct IQUEUEHEAD *p, *prev;
IUINT32 sn = newseg.sn;
int repeat = 0;

if (_itimediff(sn, kcp.rcv_nxt + kcp.rcv_wnd) >= 0 ||
_itimediff(sn, kcp.rcv_nxt) < 0) {
ikcp_segment_delete(kcp, newseg);
return;
}

for (p = kcp.rcv_buf.prev; p != &kcp.rcv_buf; p = prev) {
IKCPSEG *seg = iqueue_entry(p, IKCPSEG, node);
prev = p.prev;
if (seg.sn == sn) {
repeat = 1;
break;
}
if (_itimediff(sn, seg.sn) > 0) {
break;
}
}

if (repeat == 0) {
iqueue_init(&newseg.node);
iqueue_add(&newseg.node, p);
kcp.nrcv_buf++;
}	else {
ikcp_segment_delete(kcp, newseg);
}

#if 0
ikcp_qprint("rcvbuf", &kcp.rcv_buf);
printf("rcv_nxt=%lu\n", kcp.rcv_nxt);
#endif

// move available data from rcv_buf . rcv_queue
for  (! iqueue_is_empty(&kcp.rcv_buf)) {
IKCPSEG *seg = iqueue_entry(kcp.rcv_buf.next, IKCPSEG, node);
if (seg.sn == kcp.rcv_nxt && kcp.nrcv_que < kcp.rcv_wnd) {
iqueue_del(&seg.node);
kcp.nrcv_buf--;
iqueue_add_tail(&seg.node, &kcp.rcv_queue);
kcp.nrcv_que++;
kcp.rcv_nxt++;
}	else {
break;
}
}

#if 0
ikcp_qprint("queue", &kcp.rcv_queue);
printf("rcv_nxt=%lu\n", kcp.rcv_nxt);
#endif

#if 1
//	printf("snd(buf=%d, queue=%d)\n", kcp.nsnd_buf, kcp.nsnd_que);
//	printf("rcv(buf=%d, queue=%d)\n", kcp.nrcv_buf, kcp.nrcv_que);
#endif
}


//---------------------------------------------------------------------
// input data
//---------------------------------------------------------------------
func (kcp *IKcp)Input( data []byte)int {
prev_una := kcp.snd_una;
maxack := uint32(0)
latest_ts := uint32(0)
 flag := 0;

if (ikcp_canlog(kcp, IKCP_LOG_INPUT)) {
ikcp_log(kcp, IKCP_LOG_INPUT, "[RI] %d bytes", (int)size);
}

if (len(data) == 0 || len(data) < IKCP_OVERHEAD) {return -1;}

for (1) {
IUINT32 ts, sn, len, una, conv;
IUINT16 wnd;
IUINT8 cmd, frg;
IKCPSEG *seg;

if (size < (int)IKCP_OVERHEAD) break;

data = ikcp_decode32u(data, &conv);
if (conv != kcp.conv) return -1;

data = ikcp_decode8u(data, &cmd);
data = ikcp_decode8u(data, &frg);
data = ikcp_decode16u(data, &wnd);
data = ikcp_decode32u(data, &ts);
data = ikcp_decode32u(data, &sn);
data = ikcp_decode32u(data, &una);
data = ikcp_decode32u(data, &len);

size -= IKCP_OVERHEAD;

if ((long)size < (long)len || (int)len < 0) return -2;

if (cmd != IKCP_CMD_PUSH && cmd != IKCP_CMD_ACK &&
cmd != IKCP_CMD_WASK && cmd != IKCP_CMD_WINS)
return -3;

kcp.rmt_wnd = wnd;
ikcp_parse_una(kcp, una);
ikcp_shrink_buf(kcp);

if (cmd == IKCP_CMD_ACK) {
if (_itimediff(kcp.current, ts) >= 0) {
ikcp_update_ack(kcp, _itimediff(kcp.current, ts));
}
ikcp_parse_ack(kcp, sn);
ikcp_shrink_buf(kcp);
if (flag == 0) {
flag = 1;
maxack = sn;
latest_ts = ts;
}	else {
if (_itimediff(sn, maxack) > 0) {
#ifndef IKCP_FASTACK_CONSERVE
maxack = sn;
latest_ts = ts;
#else
if (_itimediff(ts, latest_ts) > 0) {
maxack = sn;
latest_ts = ts;
}
#endif
}
}
if (ikcp_canlog(kcp, IKCP_LOG_IN_ACK)) {
ikcp_log(kcp, IKCP_LOG_IN_ACK,
"input ack: sn=%lu rtt=%ld rto=%ld", (unsigned long)sn,
(long)_itimediff(kcp.current, ts),
(long)kcp.rx_rto);
}
}
else if (cmd == IKCP_CMD_PUSH) {
if (ikcp_canlog(kcp, IKCP_LOG_IN_DATA)) {
ikcp_log(kcp, IKCP_LOG_IN_DATA,
"input psh: sn=%lu ts=%lu", (unsigned long)sn, (unsigned long)ts);
}
if (_itimediff(sn, kcp.rcv_nxt + kcp.rcv_wnd) < 0) {
ikcp_ack_push(kcp, sn, ts);
if (_itimediff(sn, kcp.rcv_nxt) >= 0) {
seg = ikcp_segment_new(kcp, len);
seg.conv = conv;
seg.cmd = cmd;
seg.frg = frg;
seg.wnd = wnd;
seg.ts = ts;
seg.sn = sn;
seg.una = una;
seg.len = len;

if (len > 0) {
memcpy(seg.data, data, len);
}

ikcp_parse_data(kcp, seg);
}
}
}
else if (cmd == IKCP_CMD_WASK) {
// ready to send back IKCP_CMD_WINS in ikcp_flush
// tell remote my window size
kcp.probe |= IKCP_ASK_TELL;
if (ikcp_canlog(kcp, IKCP_LOG_IN_PROBE)) {
ikcp_log(kcp, IKCP_LOG_IN_PROBE, "input probe");
}
}
else if (cmd == IKCP_CMD_WINS) {
// do nothing
if (ikcp_canlog(kcp, IKCP_LOG_IN_WINS)) {
ikcp_log(kcp, IKCP_LOG_IN_WINS,
"input wins: %lu", (unsigned long)(wnd));
}
}
else {
return -3;
}

data += len;
size -= len;
}

if (flag != 0) {
ikcp_parse_fastack(kcp, maxack, latest_ts);
}

if (_itimediff(kcp.snd_una, prev_una) > 0) {
if (kcp.cwnd < kcp.rmt_wnd) {
IUINT32 mss = kcp.mss;
if (kcp.cwnd < kcp.ssthresh) {
kcp.cwnd++;
kcp.incr += mss;
}	else {
if (kcp.incr < mss) kcp.incr = mss;
kcp.incr += (mss * mss) / kcp.incr + (mss / 16);
if ((kcp.cwnd + 1) * mss <= kcp.incr) {
#if 1
kcp.cwnd = (kcp.incr + mss - 1) / ((mss > 0)? mss : 1);
#else
kcp.cwnd++;
#endif
}
}
if (kcp.cwnd > kcp.rmt_wnd) {
kcp.cwnd = kcp.rmt_wnd;
kcp.incr = kcp.rmt_wnd * mss;
}
}
}

return 0;
}
