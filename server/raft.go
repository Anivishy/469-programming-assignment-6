package main

import (
	"fmt"
	"net/rpc"
	"strings"
	"sync"
	"time"

	"banking/bank"
)

// Leader: client request replication

func (b *Bank) replicateAndCommit(cmd string) (string, error) {
	b.mu.Lock()

	if b.state != "leader" {
		la := b.leaderAddr
		b.mu.Unlock()
		return "", fmt.Errorf("not the leader, contact %s", la)
	}

	reqID := b.nextRequestID.Add(1)
	b.raftLogger.Printf("LEADER SERVER %s RECEIVES REQUEST %d FROM CLIENT", b.serverID, reqID)

	prevIndex, prevTerm := b.lastLogInfo()
	entry := bank.LogEntry{
		Index:   prevIndex + 1,
		Term:    b.currentTerm,
		Command: cmd,
	}
	b.raftLog = append(b.raftLog, entry)
	b.db.Exec("INSERT INTO raft_log (idx, term, cmd) VALUES (?, ?, ?)",
		entry.Index, entry.Term, entry.Command)

	b.raftLogger.Printf("LEADER SERVER %s ADDS REQUEST %d TO LOG ENTRY %d",
		b.serverID, reqID, entry.Index)

	fullLog := make([]bank.LogEntry, len(b.raftLog))
	copy(fullLog, b.raftLog)
	peers := b.peers
	term := b.currentTerm
	commitIndex := b.commitIndex
	b.mu.Unlock()

	args := bank.AppendEntriesArgs{
		Term:         term,
		LeaderID:     b.serverID,
		LeaderAddr:   b.myAddr,
		PrevLogIndex: prevIndex,
		PrevLogTerm:  prevTerm,
		Entries:      []bank.LogEntry{entry},
		LeaderCommit: commitIndex,
		RequestID:    reqID,
	}
	catchUpArgs := bank.AppendEntriesArgs{
		Term:         term,
		LeaderID:     b.serverID,
		LeaderAddr:   b.myAddr,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      fullLog,
		LeaderCommit: commitIndex,
		RequestID:    reqID,
	}

	acks := 1
	var wg sync.WaitGroup
	var mu sync.Mutex
	stepDown := false

	for _, peer := range peers {
		peerID := b.addrToPeerID(peer)
		wg.Add(1)
		go func(addr, pid string) {
			defer wg.Done()
			b.raftLogger.Printf("LEADER SENDS REQUEST %d TO SERVER %s", reqID, pid)
			var reply bank.AppendEntriesReply
			c, err := rpc.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer c.Close()
			if err := c.Call("Bank.AppendEntries", args, &reply); err != nil {
				return
			}
			if !reply.Success && reply.Term <= term {
				c.Call("Bank.AppendEntries", catchUpArgs, &reply) //nolint:errcheck
			}
			mu.Lock()
			defer mu.Unlock()
			if reply.Term > term {
				stepDown = true
				return
			}
			if reply.Success {
				b.raftLogger.Printf("LEADER RECEIVES ACK FROM SERVER %s FOR REQUEST %d", pid, reqID)
				acks++
			}
		}(peer, peerID)
	}
	wg.Wait()

	if stepDown {
		b.mu.Lock()
		if b.currentTerm < args.Term {
			b.becomeFollower(args.Term)
		}
		b.mu.Unlock()
		return "", fmt.Errorf("lost leadership during replication")
	}

	total := len(peers) + 1
	majority := total/2 + 1
	if acks < majority {
		return "", fmt.Errorf("failed to reach majority: got %d/%d ACKs", acks, total)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.commitIndex = entry.Index
	b.lastApplied = entry.Index
	return b.applyEntry(entry.Command)
}

// Follower: receive log entries from leader

func (b *Bank) AppendEntries(args bank.AppendEntriesArgs, reply *bank.AppendEntriesReply) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	reply.Term = b.currentTerm

	if args.Term < b.currentTerm {
		reply.Success = false
		return nil
	}

	if args.Term > b.currentTerm {
		b.becomeFollower(args.Term)
	} else if b.state == "candidate" {
		b.state = "follower"
		b.votedFor = ""
	}

	b.leaderID = args.LeaderID
	b.leaderAddr = args.LeaderAddr
	b.resetElectionTimerLocked()

	if args.RequestID != 0 {
		b.raftLogger.Printf("FOLLOWER %s RECEIVES REQUEST %d", b.serverID, args.RequestID)
	}

	if args.PrevLogIndex > 0 {
		if args.PrevLogIndex > len(b.raftLog) {
			reply.Success = false
			return nil
		}
		if b.raftLog[args.PrevLogIndex-1].Term != args.PrevLogTerm {
			reply.Success = false
			return nil
		}
	}

	for _, e := range args.Entries {
		if e.Index <= len(b.raftLog) {
			b.raftLog[e.Index-1] = e
			b.db.Exec("INSERT OR REPLACE INTO raft_log (idx, term, cmd) VALUES (?, ?, ?)",
				e.Index, e.Term, e.Command)
		} else {
			b.raftLog = append(b.raftLog, e)
			b.db.Exec("INSERT INTO raft_log (idx, term, cmd) VALUES (?, ?, ?)",
				e.Index, e.Term, e.Command)
		}
	}

	if args.LeaderCommit > b.commitIndex {
		newCommit := args.LeaderCommit
		if len(b.raftLog) > 0 && b.raftLog[len(b.raftLog)-1].Index < newCommit {
			newCommit = b.raftLog[len(b.raftLog)-1].Index
		}
		for b.lastApplied < newCommit {
			b.lastApplied++
			b.applyEntry(b.raftLog[b.lastApplied-1].Command) 
		}
		b.commitIndex = newCommit
	}

	reply.Success = true
	return nil
}

// Leader: heartbeat loop

func (b *Bank) sendHeartbeats() {
	b.mu.RLock()
	if b.state != "leader" {
		b.mu.RUnlock()
		return
	}
	term := b.currentTerm
	commitIndex := b.commitIndex
	prevIndex, prevTerm := b.lastLogInfo()
	peers := make([]Peer, len(b.allPeers))
	copy(peers, b.allPeers)
	fullLog := make([]bank.LogEntry, len(b.raftLog))
	copy(fullLog, b.raftLog)
	b.mu.RUnlock()

	args := bank.AppendEntriesArgs{
		Term:         term,
		LeaderID:     b.serverID,
		LeaderAddr:   b.myAddr,
		PrevLogIndex: prevIndex,
		PrevLogTerm:  prevTerm,
		LeaderCommit: commitIndex,
		RequestID:    0,
	}
	catchUpArgs := bank.AppendEntriesArgs{
		Term:         term,
		LeaderID:     b.serverID,
		LeaderAddr:   b.myAddr,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      fullLog,
		LeaderCommit: commitIndex,
		RequestID:    0,
	}

	for _, p := range peers {
		go func(peer Peer) {
			b.raftLogger.Printf("LEADER SENDS REQUEST 0 TO SERVER %s", peer.ID)
			var reply bank.AppendEntriesReply
			c, err := rpc.Dial("tcp", peer.Addr)
			if err != nil {
				return
			}
			defer c.Close()
			c.Call("Bank.AppendEntries", args, &reply) //nolint:errcheck
			if !reply.Success && reply.Term <= term && len(fullLog) > 0 {
				c.Call("Bank.AppendEntries", catchUpArgs, &reply) //nolint:errcheck
			}
			if reply.Term > term {
				b.mu.Lock()
				if reply.Term > b.currentTerm {
					b.becomeFollower(reply.Term)
				}
				b.mu.Unlock()
			}
		}(p)
	}
}

func (b *Bank) runHeartbeatLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		b.sendHeartbeats()
	}
}

// Leader election

func (b *Bank) startElection() {
	b.mu.Lock()
	if b.state == "leader" {
		b.mu.Unlock()
		return
	}

	b.state = "candidate"
	b.currentTerm++
	b.votedFor = b.serverID
	term := b.currentTerm
	lastIdx, lastTerm := b.lastLogInfo()
	peers := make([]Peer, len(b.allPeers))
	copy(peers, b.allPeers)
	b.mu.Unlock()

	b.raftLogger.Printf("CANDIDATE SERVER %s SENDING A VOTING REQUEST", b.serverID)

	votes := 1
	var mu sync.Mutex
	var wg sync.WaitGroup
	stepDown := false
	stepDownTerm := 0

	for _, p := range peers {
		wg.Add(1)
		go func(peer Peer) {
			defer wg.Done()
			args := bank.RequestVoteArgs{
				Term:          term,
				CandidateID:   b.serverID,
				CandidateAddr: b.myAddr,
				LastLogIndex:  lastIdx,
				LastLogTerm:   lastTerm,
			}
			var reply bank.RequestVoteReply
			c, err := rpc.Dial("tcp", peer.Addr)
			if err != nil {
				return
			}
			defer c.Close()
			if err := c.Call("Bank.RequestVote", args, &reply); err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if reply.Term > term {
				stepDown = true
				stepDownTerm = reply.Term
				return
			}
			if reply.VoteGranted {
				votes++
			}
		}(p)
	}
	wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	if stepDown {
		b.becomeFollower(stepDownTerm)
		return
	}

	if b.state != "candidate" || b.currentTerm != term {
		return
	}

	total := len(peers) + 1
	majority := total/2 + 1

	if votes >= majority {
		b.raftLogger.Printf("CANDIDATE SERVER %s WINS THE ELECTION FOR TERM %d", b.serverID, term)
		b.becomeLeader()
	} else {
		b.raftLogger.Printf("CANDIDATE SERVER %s LOSES THE ELECTION FOR TERM %d", b.serverID, term)
		b.raftLogger.Printf("ELECTION TIE, TIMEOUT BEGINS")
		b.state = "follower"
		b.votedFor = ""
		b.resetElectionTimerLocked()
	}
}

func (b *Bank) RequestVote(args bank.RequestVoteArgs, reply *bank.RequestVoteReply) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	reply.Term = b.currentTerm

	if args.Term > b.currentTerm {
		b.becomeFollower(args.Term)
	}

	if args.Term < b.currentTerm {
		b.raftLogger.Printf("SERVER %s DENIES VOTE FOR SERVER %s", b.serverID, args.CandidateID)
		reply.VoteGranted = false
		return nil
	}

	alreadyVoted := b.votedFor != "" && b.votedFor != args.CandidateID
	lastIdx, lastTerm := b.lastLogInfo()
	candidateUpToDate := args.LastLogTerm > lastTerm ||
		(args.LastLogTerm == lastTerm && args.LastLogIndex >= lastIdx)

	if !alreadyVoted && candidateUpToDate {
		b.votedFor = args.CandidateID
		b.leaderAddr = args.CandidateAddr
		b.resetElectionTimerLocked()
		b.raftLogger.Printf("SERVER %s VOTES FOR SERVER %s", b.serverID, args.CandidateID)
		reply.VoteGranted = true
	} else {
		b.raftLogger.Printf("SERVER %s DENIES VOTE FOR SERVER %s", b.serverID, args.CandidateID)
		reply.VoteGranted = false
	}
	return nil
}

// Tester support

func (b *Bank) GetLog(_ bank.GetLogArgs, reply *bank.GetLogReply) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	reply.Entries = make([]bank.LogEntry, len(b.raftLog))
	copy(reply.Entries, b.raftLog)
	return nil
}

// Internal helpers

func (b *Bank) addrToPeerID(addr string) string {
	for _, p := range b.allPeers {
		if p.Addr == addr {
			return p.ID
		}
	}
	return addr
}

func (b *Bank) applyEntry(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	switch parts[0] {
	case "OpenAccount":
		m := parseKV(parts[1:])
		id := m["id"]
		username := m["username"]
		var balance float64
		fmt.Sscanf(m["balance"], "%f", &balance)
		b.db.Exec(
			"INSERT OR IGNORE INTO accounts (id, username, balance, status) VALUES (?, ?, ?, ?)",
			id, username, balance, bank.Active,
		)
		return fmt.Sprintf("opened account for %s with balance $%.2f", username, balance), nil

	case "CloseAccount":
		m := parseKV(parts[1:])
		b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Closed, m["account"])
		return fmt.Sprintf("account %s closed", m["account"]), nil

	case "FreezeAccount":
		m := parseKV(parts[1:])
		b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Frozen, m["account"])
		return fmt.Sprintf("account %s frozen", m["account"]), nil

	case "UnfreezeAccount":
		m := parseKV(parts[1:])
		b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Active, m["account"])
		return fmt.Sprintf("account %s unfrozen", m["account"]), nil

	case "Deposit":
		m := parseKV(parts[1:])
		var amount float64
		fmt.Sscanf(m["amount"], "%f", &amount)
		b.db.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount, m["account"])
		return fmt.Sprintf("deposited $%.2f", amount), nil

	case "Withdraw":
		m := parseKV(parts[1:])
		var amount float64
		fmt.Sscanf(m["amount"], "%f", &amount)
		b.db.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, m["account"])
		return fmt.Sprintf("withdrew $%.2f", amount), nil

	case "Transfer":
		m := parseKV(parts[1:])
		var amount float64
		fmt.Sscanf(m["amount"], "%f", &amount)
		b.db.Exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, m["from"])
		b.db.Exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount, m["to"])
		return fmt.Sprintf("transferred $%.2f from %s to %s", amount, m["from"], m["to"]), nil

	case "ApplyBonus":
		m := parseKV(parts[1:])
		var pct float64
		fmt.Sscanf(m["pct"], "%f", &pct)
		b.db.Exec("UPDATE accounts SET balance = ROUND(balance * (1 + ? / 100.0), 2) WHERE id = ?",
			pct, m["account"])
		return fmt.Sprintf("applied %.2f%% bonus", pct), nil

	case "ChargeServiceFee":
		m := parseKV(parts[1:])
		var fee float64
		fmt.Sscanf(m["fee"], "%f", &fee)
		b.db.Exec("UPDATE accounts SET balance = MAX(0, ROUND(balance - ?, 2)) WHERE id = ?",
			fee, m["account"])
		return fmt.Sprintf("charged service fee $%.2f", fee), nil

	case "CheckBalance":
		return "balance checked", nil
	}

	return "", fmt.Errorf("unknown command: %s", parts[0])
}

func parseKV(pairs []string) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if i := strings.IndexByte(p, '='); i >= 0 {
			m[p[:i]] = p[i+1:]
		}
	}
	return m
}
