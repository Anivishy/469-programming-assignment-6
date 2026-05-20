package main

import (
	"database/sql"
	"fmt"
	"banking/bank"
)

func (b *Bank) ApplyBonus(request bank.BonusRequest, response *bank.Response) error {
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
	if acc.Status != bank.Active {
		response.Success = false
		response.Message = fmt.Sprintf("account is %s", statusString(acc.Status))
		return nil
	}

	cmd := fmt.Sprintf("ApplyBonus teller=%s account=%s pct=%.2f",
		request.TellerID, request.AccountID, request.Percentage)
	if _, err := b.replicateAndCommit(cmd); err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	label := "bonus"
	if request.Percentage < 0 {
		label = "interest"
	}
	delta := round2(acc.Balance * request.Percentage / 100.0)
	newBalance := round2(acc.Balance + delta)
	if newBalance < 0 {
		newBalance = 0
	}
	response.Success = true
	response.Message = fmt.Sprintf("applied %.2f%% %s ($%.2f) — new balance: $%.2f",
		request.Percentage, label, delta, newBalance)
	return nil
}

func (b *Bank) ChargeServiceFee(request bank.FeeRequest, response *bank.Response) error {
	if b.notLeader(&response.Success, &response.Message, &response.LeaderAddr) {
		return nil
	}
	if !b.isTeller(request.TellerID) {
		response.Success = false
		response.Message = "unauthorized: not a registered teller"
		return nil
	}
	if request.Fee <= 0 {
		response.Success = false
		response.Message = "fee must be a positive value"
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

	cmd := fmt.Sprintf("ChargeServiceFee teller=%s account=%s fee=%.2f",
		request.TellerID, request.AccountID, request.Fee)
	if _, err := b.replicateAndCommit(cmd); err != nil {
		response.Success = false
		response.Message = err.Error()
		return nil
	}

	newBalance := round2(acc.Balance - request.Fee)
	if newBalance < 0 {
		newBalance = 0
	}
	response.Success = true
	response.Message = fmt.Sprintf("charged service fee $%.2f — new balance: $%.2f",
		request.Fee, newBalance)
	return nil
}
