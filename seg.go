package go_kcp

import (
	"container/list"
	"encoding/binary"
	"errors"
)

var (
	ErrorSegSizeInvalid     = errors.New("invalid seg data size")
	ErrorSegCmdTypeInvalid  = errors.New("invalid seg cmd type")
	ErrorSegInvalidPeekSize = errors.New("invalid seg peek size")
	ErrorHasPartPeekSize    = errors.New("only has  part seg peek size")
	ErrorNeedSizeOverHas    = errors.New("need size over has")
)

type ISeg struct {
	node     *list.Element //双向链表定义的队列 用于发送和接受队列的缓冲
	conv     uint32        // conversation ，会话序列号：接收到的数据包与发送的一致才接收此数据包
	cmd      uint32        //command，指令类型：代表这个segment的类型
	frg      uint32        //fragment 分段序号
	wnd      uint32        //接受窗口大小
	ts       uint32        //timestamp，发送的时间戳
	sn       uint32        //sequence number ， segment序号
	una      uint32        //  unacknowledged，当前未收到的序号，即代表这个序号之前的包都收到
	len      uint32        // 数据长度
	resendts uint32        // 重发的时间戳
	rto      int32         // 超时重传的时间间隔
	fastack  uint32        // ack跳过的次数，用于快速重传
	xmit     uint32        // 发送的次数（次数为1则是第一次发送，次数>=2则为重传）
	data     []byte        // 用户数据
}

func (seg *ISeg) Decode(data []byte) (err error) {
	seg.conv = binary.BigEndian.Uint32(data[:4])
	seg.cmd = uint32(data[4])
	seg.frg = uint32(data[5])
	seg.wnd = uint32(binary.BigEndian.Uint16(data[6:8]))
	seg.ts = binary.BigEndian.Uint32(data[8:12])
	seg.sn = binary.BigEndian.Uint32(data[12:16])
	seg.una = binary.BigEndian.Uint32(data[16:20])
	seg.len = binary.BigEndian.Uint32(data[20:24])
	if uint32(len(data)-IKCP_OVERHEAD) < seg.len || seg.len < 0 {
		return ErrorSegSizeInvalid
	}
	seg.data = data[IKCP_OVERHEAD : IKCP_OVERHEAD+seg.len]
	return nil
}

func (seg *ISeg) Encode() (data []byte) {
	length := 4 + 1 + 1 + 2 + 4 + 4 + 4 + 4 //==IKCP_OVERHEAD
	data = make([]byte, length)
	binary.BigEndian.PutUint32(data[:4], seg.conv)
	data[4] = uint8(seg.cmd)
	data[5] = uint8(seg.frg)
	binary.BigEndian.PutUint16(data[6:8], uint16(seg.wnd))
	binary.BigEndian.PutUint32(data[8:12], seg.ts)
	binary.BigEndian.PutUint32(data[12:16], seg.sn)
	binary.BigEndian.PutUint32(data[16:20], seg.una)
	binary.BigEndian.PutUint32(data[20:24], seg.len)
	return data
}
