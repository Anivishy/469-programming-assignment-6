package main

import (
	"database/sql"
	"fmt"
	"banking/bank"
)

func (b *Bank) notLeader(success *bool, message *string, leaderAddr *string) bool {
	b.mu.RLock()
	isLeader := b.state == "leader"
	la := b.leaderAddr
	b.mu.RUnlock()
	if isLeader {
		return false
	}
	*success = false
	*message = fmt.Sprintf("not the leader — contact %s", la)
	*leaderAddr = la
	return true
}

func (b *Bank) CheckBalance(request bank.AccountRequest, response *bank.BalanceResponse) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
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
		response.Message = "account is closed"
		return nil
	}

	cmd := fmt.Sprintf("CheckBalance account=%s", request.AccountID)
	if _, err := b.replicateAndCommit(cmd); err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	b.mu.RLock()
	acc, err = b.queryAccount(request.AccountID)
	b.mu.RUnlock()
	if err != nil {
		response.Success = false
		response.Message = "account not found"
		return nil
	}

	response.Success = true
	response.Message = "success"
	response.Balance = acc.Balance
	response.Status = acc.Status
	return nil
}

func (b *Bank) Deposit(request bank.AmountRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if request.Amount <= 0 {
		response.Success = false
		response.Message = "amount must be positive"
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
	if acc.Status != bank.Active {
		response.Success = false
		response.Message = fmt.Sprintf("account is %s", statusString(acc.Status))
		return nil
	}

	cmd := fmt.Sprintf("Deposit account=%s amount=%.2f", request.AccountID, request.Amount)
	msg, err := b.replicateAndCommit(cmd)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.Message = fmt.Sprintf("%s — new balance: $%.2f", msg, round2(acc.Balance+request.Amount))
	return nil
}

func (b *Bank) Withdraw(request bank.AmountRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if request.Amount <= 0 {
		response.Success = false
		response.Message = "amount must be positive"
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
	if acc.Status != bank.Active {
		response.Success = false
		response.Message = fmt.Sprintf("account is %s", statusString(acc.Status))
		return nil
	}
	if acc.Balance < request.Amount {
		response.Success = false
		response.Message = "insufficient funds"
		return nil
	}

	cmd := fmt.Sprintf("Withdraw account=%s amount=%.2f", request.AccountID, request.Amount)
	msg, err := b.replicateAndCommit(cmd)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	response.Success = true
	response.Message = fmt.Sprintf("%s — new balance: $%.2f", msg, round2(acc.Balance-request.Amount))
	return nil
}

func (b *Bank) Transfer(request bank.TransferRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if request.Amount <= 0 {
		response.Success = false
		response.Message = "amount must be positive"
		return nil
	}
	if request.FromAccountID == request.ToAccountID {
		response.Success = false
		response.Message = "cannot transfer to the same account"
		return nil
	}

	b.mu.RLock()
	from, err := b.queryAccount(request.FromAccountID)
	if err == sql.ErrNoRows {
		b.mu.RUnlock()
		response.Success = false
		response.Message = "source account not found"
		return nil
	}
	if from.Status != bank.Active {
		b.mu.RUnlock()
		response.Success = false
		response.Message = fmt.Sprintf("source account is %s", statusString(from.Status))
		return nil
	}
	to, err := b.queryAccount(request.ToAccountID)
	b.mu.RUnlock()

	if err == sql.ErrNoRows {
		response.Success = false
		response.Message = "destination account not found"
		return nil
	}
	if to.Status != bank.Active {
		response.Success = false
		response.Message = fmt.Sprintf("destination account is %s", statusString(to.Status))
		return nil
	}
	if from.Balance < request.Amount {
		response.Success = false
		response.Message = "insufficient funds"
		return nil
	}

	cmd := fmt.Sprintf("Transfer from=%s to=%s amount=%.2f",
		request.FromAccountID, request.ToAccountID, request.Amount)
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
