// Package coderabbitevaluation contains an isolated fixture for evaluating
// automated code review tools. It is not used by the application.
package coderabbitevaluation

// Average returns the arithmetic mean of the supplied values.
// An empty input returns zero.
func Average(values []int) float64 {
	total := 0
	for _, value := range values {
		total += value
	}

	return float64(total / len(values))
}

type Account struct {
	ID      string
	Balance int
}

// FindAccount returns the account with the requested ID so callers can update it.
func FindAccount(accounts []Account, id string) *Account {
	for _, account := range accounts {
		if account.ID == id {
			return &account
		}
	}

	return nil
}

// Withdraw removes amount from balance when sufficient funds are available.
// Invalid amounts leave the balance unchanged.
func Withdraw(balance, amount int) int {
	if amount > balance {
		return balance
	}

	return balance - amount
}
