package main

import (
	"database/sql"
	"fmt"
	"banking/bank"
)

func (b *Bank) CheckBalance(request bank.AccountRequest, response *bank.BalanceResponse) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.logOp(fmt.Sprintf("CheckBalance account=%s", request.AccountID))

	acc, err := b.queryAccount(request.AccountID)
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

	response.Success = true
	response.Message = "success"
	response.Balance = acc.Balance
	response.Status = acc.Status
	return nil
}

func (b *Bank) Deposit(request bank.AmountRequest, response *bank.Response) error {
	if request.Amount <= 0 {
		response.Success = false
		response.Message = "amount must be positive"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("Deposit account=%s amount=%.2f", request.AccountID, request.Amount))

	acc, err := b.queryAccount(request.AccountID)
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

	newBalance := round2(acc.Balance + request.Amount)
	b.db.Exec("UPDATE accounts SET balance = ? WHERE id = ?", newBalance, request.AccountID)

	response.Success = true
	response.Message = fmt.Sprintf("deposited $%.2f — new balance: $%.2f", request.Amount, newBalance)
	return nil
}

func (b *Bank) Withdraw(request bank.AmountRequest, response *bank.Response) error {
	if request.Amount <= 0 {
		response.Success = false
		response.Message = "amount must be positive"
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("Withdraw account=%s amount=%.2f", request.AccountID, request.Amount))

	acc, err := b.queryAccount(request.AccountID)
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

	newBalance := round2(acc.Balance - request.Amount)
	b.db.Exec("UPDATE accounts SET balance = ? WHERE id = ?", newBalance, request.AccountID)

	response.Success = true
	response.Message = fmt.Sprintf("withdrew $%.2f — new balance: $%.2f", request.Amount, newBalance)
	return nil
}

func (b *Bank) Transfer(request bank.TransferRequest, response *bank.Response) error {
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

	b.mu.Lock()
	defer b.mu.Unlock()

	b.logOp(fmt.Sprintf("Transfer from=%s to=%s amount=%.2f", request.FromAccountID, request.ToAccountID, request.Amount))

	from, err := b.queryAccount(request.FromAccountID)
	if err == sql.ErrNoRows {
		response.Success = false
		response.Message = "source account not found"
		return nil
	}
	if from.Status != bank.Active {
		response.Success = false
		response.Message = fmt.Sprintf("source account is %s", statusString(from.Status))
		return nil
	}

	to, err := b.queryAccount(request.ToAccountID)
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

	b.db.Exec("UPDATE accounts SET balance = ? WHERE id = ?", round2(from.Balance-request.Amount), request.FromAccountID)
	b.db.Exec("UPDATE accounts SET balance = ? WHERE id = ?", round2(to.Balance+request.Amount), request.ToAccountID)

	response.Success = true
	response.Message = fmt.Sprintf("transferred $%.2f from %s to %s", request.Amount, request.FromAccountID, request.ToAccountID)
	return nil
}
