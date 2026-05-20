package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strings"
	"banking/bank"
)

type CompareArgs struct{}
type CompareReply struct {
	Match   bool
	Message string
}

type Tester struct {
	servers []string
}

func (t *Tester) CompareLog(_ CompareArgs, reply *CompareReply) error {
	logs := make([][]bank.LogEntry, len(t.servers))
	for i, addr := range t.servers {
		c, err := rpc.Dial("tcp", addr)
		if err != nil {
			reply.Match = false
			reply.Message = fmt.Sprintf("could not connect to %s: %v", addr, err)
			return nil
		}
		var resp bank.GetLogReply
		if err := c.Call("Bank.GetLog", bank.GetLogArgs{}, &resp); err != nil {
			c.Close()
			reply.Match = false
			reply.Message = fmt.Sprintf("GetLog failed for %s: %v", addr, err)
			return nil
		}
		c.Close()
		logs[i] = resp.Entries
	}

	ref := logs[0]
	for i := 1; i < len(logs); i++ {
		if len(logs[i]) != len(ref) {
			reply.Match = false
			reply.Message = fmt.Sprintf("server %d has %d entries, server 1 has %d",
				i+1, len(logs[i]), len(ref))
			return nil
		}
		for j, e := range ref {
			o := logs[i][j]
			if e.Index != o.Index || e.Term != o.Term || e.Command != o.Command {
				reply.Match = false
				reply.Message = fmt.Sprintf("mismatch at entry %d between server 1 and server %d", j+1, i+1)
				return nil
			}
		}
	}

	reply.Match = true
	reply.Message = fmt.Sprintf("all %d servers have identical logs (%d entries)", len(t.servers), len(ref))
	return nil
}

func main() {
	serversEnv := os.Getenv("SERVERS")
	if serversEnv == "" {
		serversEnv = "server1:8080,server2:8080,server3:8080"
	}
	servers := strings.Split(serversEnv, ",")

	tester := &Tester{servers: servers}
	if err := rpc.Register(tester); err != nil {
		log.Fatal("register:", err)
	}

	ln, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatal("listen:", err)
	}
	log.Println("tester listening on :9000, watching:", servers)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
