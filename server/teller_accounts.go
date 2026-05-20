package main

import (
	"database/sql"
	"fmt"
	"banking/bank"
)

func (b *Bank) OpenAccount(request bank.OpenAccountRequest, response *bank.OpenAccountResponse) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
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

	id := b.generateID()
	cmd := fmt.Sprintf("OpenAccount teller=%s username=%s id=%s balance=%.2f",
		request.TellerID, request.Username, id, request.InitialBalance)

	if _, err := b.replicateAndCommit(cmd); err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.AccountID = id
	response.Message = fmt.Sprintf("opened account for %s with balance $%.2f",
		request.Username, request.InitialBalance)
	return nil
}

func (b *Bank) CloseAccount(request bank.TellerRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.RLock()
	acc, err := b.queryAccount(request.AccountID)
	b.mu.RUnlock()

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

	cmd := fmt.Sprintf("CloseAccount teller=%s account=%s", request.TellerID, request.AccountID)
	msg, err := b.replicateAndCommit(cmd)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.Message = msg
	return nil
}

func (b *Bank) FreezeAccount(request bank.TellerRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.RLock()
	acc, err := b.queryAccount(request.AccountID)
	b.mu.RUnlock()

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

	cmd := fmt.Sprintf("FreezeAccount teller=%s account=%s", request.TellerID, request.AccountID)
	msg, err := b.replicateAndCommit(cmd)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.Message = msg
	return nil
}

func (b *Bank) UnfreezeAccount(request bank.TellerRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}

	b.mu.RLock()
	acc, err := b.queryAccount(request.AccountID)
	b.mu.RUnlock()

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

	cmd := fmt.Sprintf("UnfreezeAccount teller=%s account=%s", request.TellerID, request.AccountID)
	msg, err := b.replicateAndCommit(cmd)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.Message = msg
	return nil
}
