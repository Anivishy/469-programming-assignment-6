package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"banking/bank"
	_ "modernc.org/sqlite"
)

type Bank struct {
	mu sync.RWMutex
	db *sql.DB
	tellers map[string]bool
	nextID atomic.Int64
}

func newBank() *Bank {
	db, err := sql.Open("sqlite", "bank.db")
	if err != nil {
		log.Fatal("open db:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS accounts (
		id      TEXT    PRIMARY KEY,
		username TEXT   NOT NULL,
		balance  REAL   NOT NULL DEFAULT 0,
		status   INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		log.Fatal("create accounts table:", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS log (
		id  INTEGER  PRIMARY KEY AUTOINCREMENT,
		ts  DATETIME DEFAULT CURRENT_TIMESTAMP,
		op  TEXT     NOT NULL
	)`)
	if err != nil {
		log.Fatal("create log table:", err)
	}

	b := &Bank{
		db: db,
		tellers: map[string]bool{
			"teller1": true,
			"teller2": true,
			"teller3": true,
		},
	}

	var maxN int64
	db.QueryRow("SELECT COALESCE(MAX(CAST(SUBSTR(id,4) AS INTEGER)),0) FROM accounts").Scan(&maxN)
	b.nextID.Store(maxN)

	return b
}

func (b *Bank) isTeller(id string) bool {
	return b.tellers[id]
}

func (b *Bank) generateID() string {
	return fmt.Sprintf("ACC%08d", b.nextID.Add(1))
}

func (b *Bank) logOp(op string) {
	b.db.Exec("INSERT INTO log (op) VALUES (?)", op)
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

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
