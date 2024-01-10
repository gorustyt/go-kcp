package main

import (
	"fmt"
	go_kcp "github.com/gorustyt/go-kcp"
	"strconv"
	"time"
)

func main() {
	go_kcp.SetLogLevel(go_kcp.LogLevelDebug)
	c, err := go_kcp.Dial("udp", "127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	go func() {
		var i int
		for {
			_, err := c.Write([]byte("client send to server" + strconv.Itoa(i)))
			if err != nil {
				panic(err)
			}
			time.Sleep(3 * time.Second)
			i++
		}
	}()
	for {
		var b [2048]byte
		n, err := c.Read(b[:])
		if err != nil {
			panic(err)
		}
		fmt.Printf("recv1:%v \n", string(b[:n]))
	}
}
