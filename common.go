package go_kcp

import (
	"cmp"
	"errors"
)

func itImeDiff(later, earlier uint32) int64 {
	return int64(later) - int64(earlier)
}

func bound[T cmp.Ordered](lower, middle, upper T) T {
	return min(max(lower, middle), upper)
}

var (
	ErrorMssInvalid      = errors.New("err mss is invalid")
	ErrorsFragLimit      = errors.New("frag over limit")
	ErrorsInvalidKcpHead = errors.New("invalid kcp head")
	ErrorsInvalidConv    = errors.New("invalid kcp conv")
)
