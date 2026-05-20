package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"banking/bank"
	_ "modernc.org/sqlite"
)

const noEntry = 0

type Peer struct {
	ID   string
	Addr string
}

type Bank struct {
	mu      sync.RWMutex
	db      *sql.DB
	tellers map[string]bool
	nextID  atomic.Int64

	serverID string
	myAddr   string

	currentTerm int
	votedFor    string

	state      string
	leaderID   string
	leaderAddr string
	allPeers   []Peer
	peers      []string

	raftLog     []bank.LogEntry
	commitIndex int
	lastApplied int

	electionTimer *time.Timer
	nextRequestID atomic.Int64
	raftLogger    *log.Logger
}

func newBank(serverID, myAddr string, allPeers []Peer) *Bank {
	db, err := sql.Open("sqlite", fmt.Sprintf("bank-%s.db", serverID))
	if err != nil {
		log.Fatal("open db:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS accounts (
		id       TEXT    PRIMARY KEY,
		username TEXT    NOT NULL,
		balance  REAL    NOT NULL DEFAULT 0,
		status   INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		log.Fatal("create accounts table:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS raft_log (
		idx  INTEGER PRIMARY KEY,
		term INTEGER NOT NULL,
		cmd  TEXT    NOT NULL
	)`)
	if err != nil {
		log.Fatal("create raft_log table:", err)
	}

	logFile, err := os.OpenFile(
		fmt.Sprintf("server-%s.log", serverID),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644,
	)
	if err != nil {
		log.Fatal("open log file:", err)
	}

	b := &Bank{
		db:         db,
		serverID:   serverID,
		myAddr:     myAddr,
		state:      "follower",
		allPeers:   allPeers,
		tellers:    map[string]bool{"teller1": true, "teller2": true, "teller3": true},
		raftLogger: log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags),
	}

	rows, err := db.Query("SELECT idx, term, cmd FROM raft_log ORDER BY idx")
	if err != nil {
		log.Fatal("load raft_log:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e bank.LogEntry
		rows.Scan(&e.Index, &e.Term, &e.Command)
		b.raftLog = append(b.raftLog, e)
	}
	if len(b.raftLog) > 0 {
		last := b.raftLog[len(b.raftLog)-1]
		b.commitIndex = last.Index
		b.lastApplied = last.Index
		b.currentTerm = last.Term
	}

	var maxN int64
	db.QueryRow("SELECT COALESCE(MAX(CAST(SUBSTR(id,4) AS INTEGER)),0) FROM accounts").Scan(&maxN)
	b.nextID.Store(maxN)

	b.mu.Lock()
	b.resetElectionTimerLocked()
	b.mu.Unlock()

	return b
}

// State-machine helpers

func (b *Bank) resetElectionTimerLocked() {
	if b.electionTimer != nil {
		b.electionTimer.Stop()
	}
	d := time.Duration(3000+rand.Intn(3000)) * time.Millisecond
	b.electionTimer = time.AfterFunc(d, func() { b.startElection() })
}

func (b *Bank) becomeFollower(term int) {
	b.state = "follower"
	b.currentTerm = term
	b.votedFor = ""
	b.resetElectionTimerLocked()
}

func (b *Bank) becomeLeader() {
	b.state = "leader"
	b.leaderID = b.serverID
	b.leaderAddr = b.myAddr
	addrs := make([]string, len(b.allPeers))
	for i, p := range b.allPeers {
		addrs[i] = p.Addr
	}
	b.peers = addrs
	if b.electionTimer != nil {
		b.electionTimer.Stop()
	}
	for b.lastApplied < b.commitIndex {
		b.lastApplied++
		b.applyEntry(b.raftLog[b.lastApplied-1].Command) //nolint:errcheck
	}
}

func (b *Bank) lastLogInfo() (index, term int) {
	if len(b.raftLog) == 0 {
		return 0, 0
	}
	last := b.raftLog[len(b.raftLog)-1]
	return last.Index, last.Term
}

// Misc helpers

func (b *Bank) isTeller(id string) bool { return b.tellers[id] }

func (b *Bank) generateID() string {
	return fmt.Sprintf("ACC%08d", b.nextID.Add(1))
}

func (b *Bank) queryAccount(id string) (*bank.Account, error) {
	var acc bank.Account
	err := b.db.QueryRow(
		"SELECT id, username, balance, status FROM accounts WHERE id = ?", id,
	).Scan(&acc.ID, &acc.Username, &acc.Balance, &acc.Status)
	return &acc, err
}

func statusString(s bank.AccountStatus) string {
	switch s {
	case bank.Active:
		return "active"
	case bank.Frozen:
		return "frozen"
	case bank.Closed:
		return "closed"
	}
	return "unknown"
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
