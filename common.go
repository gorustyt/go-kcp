package go_kcp

import "errors"

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

func Max[T Number](a, b T) T {
	if a <= b {
		return b
	}
	return a
}

func Min[T Number](a, b T) T {
	if a <= b {
		return a
	}
	return b
}

func itImeDiff(later, earlier uint32) uint32 {
	return later - earlier
}

func bound[T Number](lower, middle, upper T) T {
	return min(max(lower, middle), upper)
}

var (
	ErrorMssInvalid      = errors.New("err mss is invalid")
	ErrorsFragLimit      = errors.New("frag over limit")
	ErrorsInvalidKcpHead = errors.New("invalid kcp head")
	ErrorsInvalidConv    = errors.New("invalid kcp conv")
)
