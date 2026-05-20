package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"strconv"
	"banking/bank"
)

func main() {
	address := flag.String("server", "localhost:8080", "bank server address")
	tester  := flag.String("tester", "localhost:9000", "tester server address")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	client, err := rpc.Dial("tcp", *address)
	if err != nil {
		log.Fatalf("connect %s: %v", *address, err)
	}
	defer client.Close()

	switch args[0] {
	case "check-balance":
		need(args, 2, "check-balance <account_id>")
		checkBalance(client, args[1])
	case "deposit":
		need(args, 3, "deposit <account_id> <amount>")
		deposit(client, args[1], parseAmount(args[2]))
	case "withdraw":
		need(args, 3, "withdraw <account_id> <amount>")
		withdraw(client, args[1], parseAmount(args[2]))
	case "transfer":
		need(args, 4, "transfer <from_id> <to_id> <amount>")
		transfer(client, args[1], args[2], parseAmount(args[3]))
	case "open-account":
		need(args, 4, "open-account <teller_id> <username> <initial_balance>")
		openAccount(client, args[1], args[2], parseAmount(args[3]))
	case "close-account":
		need(args, 3, "close-account <teller_id> <account_id>")
		closeAccount(client, args[1], args[2])
	case "freeze-account":
		need(args, 3, "freeze-account <teller_id> <account_id>")
		freezeAccount(client, args[1], args[2])
	case "unfreeze-account":
		need(args, 3, "unfreeze-account <teller_id> <account_id>")
		unfreezeAccount(client, args[1], args[2])
	case "apply-bonus":
		need(args, 4, "apply-bonus <teller_id> <account_id> <percentage>")
		pct, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			log.Fatalf("invalid percentage %q: %v", args[3], err)
		}
		applyBonus(client, args[1], args[2], pct)
	case "charge-fee":
		need(args, 4, "charge-fee <teller_id> <account_id> <fee>")
		chargeServiceFee(client, args[1], args[2], parseAmount(args[3]))
	case "compare-log":
		client.Close()
		compareLog(*tester)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
		os.Exit(1)
	}
}

// Tester

type CompareArgs struct{}
type CompareReply struct {
	Match   bool
	Message string
}

func compareLog(addr string) {
	c, err := rpc.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("connect tester %s: %v", addr, err)
	}
	defer c.Close()
	var reply CompareReply
	if err := c.Call("Tester.CompareLog", CompareArgs{}, &reply); err != nil {
		log.Fatalf("CompareLog: %v", err)
	}
	if reply.Match {
		fmt.Printf("ok: %s\n", reply.Message)
	} else {
		fmt.Printf("MISMATCH: %s\n", reply.Message)
	}
}

// Customer APIs

func checkBalance(c *rpc.Client, id string) {
	var resp bank.BalanceResponse
	call(c, "Bank.CheckBalance", bank.AccountRequest{AccountID: id}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
	if resp.Success {
		fmt.Printf("  balance: $%.2f   status: %s\n", resp.Balance, statusLabel(resp.Status))
	}
}

func deposit(c *rpc.Client, id string, amount float64) {
	var resp bank.Response
	call(c, "Bank.Deposit", bank.AmountRequest{AccountID: id, Amount: amount}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func withdraw(c *rpc.Client, id string, amount float64) {
	var resp bank.Response
	call(c, "Bank.Withdraw", bank.AmountRequest{AccountID: id, Amount: amount}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func transfer(c *rpc.Client, from, to string, amount float64) {
	var resp bank.Response
	call(c, "Bank.Transfer", bank.TransferRequest{FromAccountID: from, ToAccountID: to, Amount: amount}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

// Teller APIs

func openAccount(c *rpc.Client, tellerID, username string, initial float64) {
	var resp bank.OpenAccountResponse
	call(c, "Bank.OpenAccount", bank.OpenAccountRequest{TellerID: tellerID, Username: username, InitialBalance: initial}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
	if resp.Success {
		fmt.Printf("  account id: %s\n", resp.AccountID)
	}
}

func closeAccount(c *rpc.Client, tellerID, accountID string) {
	var resp bank.Response
	call(c, "Bank.CloseAccount", bank.TellerRequest{TellerID: tellerID, AccountID: accountID}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func freezeAccount(c *rpc.Client, tellerID, accountID string) {
	var resp bank.Response
	call(c, "Bank.FreezeAccount", bank.TellerRequest{TellerID: tellerID, AccountID: accountID}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func unfreezeAccount(c *rpc.Client, tellerID, accountID string) {
	var resp bank.Response
	call(c, "Bank.UnfreezeAccount", bank.TellerRequest{TellerID: tellerID, AccountID: accountID}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func applyBonus(c *rpc.Client, tellerID, accountID string, pct float64) {
	var resp bank.Response
	call(c, "Bank.ApplyBonus", bank.BonusRequest{TellerID: tellerID, AccountID: accountID, Percentage: pct}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

func chargeServiceFee(c *rpc.Client, tellerID, accountID string, fee float64) {
	var resp bank.Response
	call(c, "Bank.ChargeServiceFee", bank.FeeRequest{TellerID: tellerID, AccountID: accountID, Fee: fee}, &resp)
	printResult(resp.Success, resp.Message, resp.LeaderAddr)
}

// Helpers

func call(c *rpc.Client, method string, args, reply any) {
	if err := c.Call(method, args, reply); err != nil {
		log.Fatalf("rpc %s: %v", method, err)
	}
}

func parseAmount(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatalf("invalid amount %q: %v", s, err)
	}
	return v
}

func statusLabel(s bank.AccountStatus) string {
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

func printResult(success bool, message, leaderAddr string) {
	if success {
		fmt.Printf("ok: %s\n", message)
	} else {
		fmt.Printf("error: %s\n", message)
		if leaderAddr != "" {
			fmt.Printf("  redirect to leader: %s\n", leaderAddr)
		}
	}
}

func need(args []string, n int, usage string) {
	if len(args) != n {
		fmt.Fprintf(os.Stderr, "usage: client %s\n", usage)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`usage: client [flags] <command> [args]

flags:
  -server string   bank server address (default "localhost:8080")

customer commands:
  check-balance  <account_id>
  deposit        <account_id>  <amount>
  withdraw       <account_id>  <amount>
  transfer       <from_id>     <to_id>  <amount>

teller commands  (teller IDs: teller1, teller2, teller3):
  open-account     <teller_id>  <username>    <initial_balance>
  close-account    <teller_id>  <account_id>
  freeze-account   <teller_id>  <account_id>
  unfreeze-account <teller_id>  <account_id>
  apply-bonus      <teller_id>  <account_id>  <percentage>   (negative = interest)
  charge-fee       <teller_id>  <account_id>  <fee>
`)
}
