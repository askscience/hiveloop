// relay: TCP forwarder used only for the load test (tailnet address -> test instance).
package main

import (
	"io"
	"log"
	"net"
	"os"
)

func main() {
	ln, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			d, err := net.Dial("tcp", os.Args[2])
			if err != nil {
				return
			}
			done := make(chan struct{}, 2)
			go func() { io.Copy(d, c); done <- struct{}{} }()
			go func() { io.Copy(c, d); done <- struct{}{} }()
			<-done // one side finished: close both, so nothing stays half-open
			d.Close()
		}(c)
	}
}
