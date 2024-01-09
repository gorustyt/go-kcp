package main

import (
	"fmt"
	go_kcp "github.com/gorustyt/go-kcp"
)

func main() {
	l, err := go_kcp.Listen("udp", ":8080")
	if err != nil {
		panic(err)
	}
	for {
		c, err := l.Accept()
		if err != nil {
			panic(err)
		}
		_, err = c.Write([]byte("handShake"))
		if err != nil {
			panic(err)
		}
		var b [4096]byte
		n, err := c.Read(b[:])
		if err != nil {
			panic(err)
		}
		fmt.Printf("server recv1:%v ", string(b[:n]))
		_, err = c.Write([]byte("server send to client"))
		if err != nil {
			panic(err)
		}
	}

}
