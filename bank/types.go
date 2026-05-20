package bank

type AccountStatus int

const (
	Active AccountStatus = iota
	Frozen
	Closed
)

type Account struct {
	ID       string
	Username string
	Balance  float64
	Status   AccountStatus
}

// Customer-facing request types

type AccountRequest struct {
	AccountID string
}

type AmountRequest struct {
	AccountID string
	Amount    float64
}

type TransferRequest struct {
	FromAccountID string
	ToAccountID   string
	Amount        float64
}

// Teller-facing request types

type TellerRequest struct {
	TellerID  string
	AccountID string
}

type BonusRequest struct {
	TellerID   string
	AccountID  string
	Percentage float64
}

type OpenAccountRequest struct {
	TellerID       string
	Username       string
	InitialBalance float64
}

type FeeRequest struct {
	TellerID  string
	AccountID string
	Fee       float64
}

// Response types

type Response struct {
	Success    bool
	Message    string
	LeaderAddr string
}

type BalanceResponse struct {
	Success    bool
	Message    string
	Balance    float64
	Status     AccountStatus
	LeaderAddr string
}

type OpenAccountResponse struct {
	Success    bool
	Message    string
	AccountID  string
	LeaderAddr string
}

// Raft types

type LogEntry struct {
	Index   int
	Term    int
	Command string
}

type AppendEntriesArgs struct {
	Term         int
	LeaderID     string
	LeaderAddr   string
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
	RequestID    int64
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type RequestVoteArgs struct {
	Term          int
	CandidateID   string
	CandidateAddr string
	LastLogIndex  int
	LastLogTerm   int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type GetLogArgs struct{}

type GetLogReply struct {
	Entries []LogEntry
}
