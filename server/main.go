package main

import (
	"log"
	"net"
	"net/rpc"
)

func main() {
	srv := newBank()
	if err := rpc.Register(srv); err != nil {
		log.Fatal("register error:", err)
	}

	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("listen error:", err)
	}

	log.Println("bank server listening on :8080")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
