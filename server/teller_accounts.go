package main

import (
	"database/sql"
	"fmt"
	"banking/bank"
)

func (b *Bank) OpenAccount(request bank.OpenAccountRequest, response *bank.OpenAccountResponse) error {
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}
	if request.Username == "" {
		response.Success = false
		response.Message = "username is required"
		return nil
	}
	if request.InitialBalance < 0 {
		response.Success = false
		response.Message = "initial balance cannot be negative"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.generateID()
	b.logOp(fmt.Sprintf("OpenAccount teller=%s username=%s id=%s balance=%.2f", request.TellerID, request.Username, id, request.InitialBalance))

	_, err := b.db.Exec(
		"INSERT INTO accounts (id, username, balance, status) VALUES (?, ?, ?, ?)",
		id, request.Username, request.InitialBalance, bank.Active,
	)
	if err != nil {
		response.Success = false
		response.Message = "database error"
		return nil
	}

	response.Success = true
	response.AccountID = id
	response.Message = fmt.Sprintf("opened account for %s with balance $%.2f", request.Username, request.InitialBalance)
	return nil
}

func (b *Bank) CloseAccount(request bank.TellerRequest, response *bank.Response) error {
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("CloseAccount teller=%s account=%s", request.TellerID, request.AccountID))

	acc, err := b.queryAccount(request.AccountID)
	if err == sql.ErrNoRows {
		response.Success = false
		response.Message = "account not found"
		return nil
	}
	if acc.Status == bank.Closed {
		response.Success = false
		response.Message = "account is already closed"
		return nil
	}

	b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Closed, request.AccountID)

	response.Success = true
	response.Message = fmt.Sprintf("account %s closed", request.AccountID)
	return nil
}

func (b *Bank) FreezeAccount(request bank.TellerRequest, response *bank.Response) error {
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("FreezeAccount teller=%s account=%s", request.TellerID, request.AccountID))

	acc, err := b.queryAccount(request.AccountID)
	if err == sql.ErrNoRows {
		response.Success = false
		response.Message = "account not found"
		return nil
	}
	if acc.Status == bank.Closed {
		response.Success = false
		response.Message = "cannot freeze a closed account"
		return nil
	}
	if acc.Status == bank.Frozen {
		response.Success = false
		response.Message = "account is already frozen"
		return nil
	}

	b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Frozen, request.AccountID)

	response.Success = true
	response.Message = fmt.Sprintf("account %s frozen", request.AccountID)
	return nil
}

func (b *Bank) UnfreezeAccount(request bank.TellerRequest, response *bank.Response) error {
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("UnfreezeAccount teller=%s account=%s", request.TellerID, request.AccountID))

	acc, err := b.queryAccount(request.AccountID)
	if err == sql.ErrNoRows {
		response.Success = false
		response.Message = "account not found"
		return nil
	}
	if acc.Status == bank.Closed {
		response.Success = false
		response.Message = "cannot unfreeze a closed account"
		return nil
	}
	if acc.Status != bank.Frozen {
		response.Success = false
		response.Message = "account is not frozen"
		return nil
	}

	b.db.Exec("UPDATE accounts SET status = ? WHERE id = ?", bank.Active, request.AccountID)

	response.Success = true
	response.Message = fmt.Sprintf("account %s unfrozen", request.AccountID)
	return nil
}
