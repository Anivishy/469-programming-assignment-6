package bank

//all acocunt related data fields
type AccountStatus int

//account status
const (
	Active AccountStatus = iota
	Frozen
	Closed
)

//all data for a particular account
type Account struct {
	ID string
	Username string
	Balance float64
	Status AccountStatus
}

//messages for customer facing account APIs
type AccountRequest struct {
	AccountID string
}

type AmountRequest struct {
	AccountID string
	Amount float64
}

type TransferRequest struct {
	FromAccountID string
	ToAccountID string
	Amount float64 // dollars, must be > 0
}

//messages for teller facing account APIs
type TellerRequest struct {
	TellerID string
	AccountID string
}

type BonusRequest struct {
	TellerID string
	AccountID string
	Percentage float64
}

type OpenAccountRequest struct {
	TellerID string
	Username string
	InitialBalance float64
}

type FeeRequest struct {
	TellerID string
	AccountID string
	Fee float64
}

//Request response types
type Response struct {
	Success bool
	Message string
}

type BalanceResponse struct {
	Success bool
	Message string
	Balance float64
	Status AccountStatus
}

type OpenAccountResponse struct {
	Success bool
	Message string
	AccountID string
}
