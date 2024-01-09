package go_kcp

import (
	"reflect"
	"testing"
)

func TestSeg(t *testing.T) {
	data := []byte("hello world")
	seg := &ISeg{
		conv:     111,
		cmd:      IKCP_CMD_ACK,
		frg:      1,
		wnd:      100,
		ts:       100,
		sn:       1,
		xmit:     1,
		fastack:  1,
		una:      100,
		resendts: 1000,
		len:      uint32(len(data)),
	}
	bytesData := seg.Encode()
	bytesData = append(bytesData, data...)
	seg1 := &ISeg{}
	if err := seg1.Decode(bytesData); err != nil {
		t.Error(err)
	}
	if reflect.DeepEqual(seg, seg1) {
		t.Errorf("encode data not equal decode data")
	}
}
