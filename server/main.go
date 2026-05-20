package main

import (
	"flag"
	"log"
	"net"
	"net/rpc"
	"strings"
)

func main() {
	id        := flag.String("id",     "S1",             "server ID used in log messages (e.g. S1)")
	port      := flag.String("port",   "8080",           "TCP port to listen on")
	addr      := flag.String("addr",   "localhost:8080", "external address clients use to reach this server")
	isLeader  := flag.Bool("leader",   false,            "become leader immediately without running an election")
	peersFlag := flag.String("peers",  "",               "other servers: S2=host:port,S3=host:port,...")
	flag.Parse()

	var allPeers []Peer
	if *peersFlag != "" {
		for _, token := range strings.Split(*peersFlag, ",") {
			kv := strings.SplitN(token, "=", 2)
			if len(kv) != 2 {
				log.Fatalf("bad -peers entry %q, want ID=host:port", token)
			}
			allPeers = append(allPeers, Peer{ID: kv[0], Addr: kv[1]})
		}
	}

	srv := newBank(*id, *addr, allPeers)

	if *isLeader {
		srv.mu.Lock()
		srv.currentTerm = 1
		srv.becomeLeader()
		srv.mu.Unlock()
		log.Printf("SERVER %s starts as initial leader for term 1", *id)
	}

	go srv.runHeartbeatLoop()

	if err := rpc.Register(srv); err != nil {
		log.Fatal("rpc register:", err)
	}

	ln, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatal("listen:", err)
	}
	log.Printf("bank server %s listening on :%s (addr=%s)", *id, *port, *addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
