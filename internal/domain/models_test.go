package domain

import (
	"testing"
)

func TestMoney(t *testing.T) {
	t.Run("NewMoney", func(t *testing.T) {
		m := NewMoney(100.50, "USD")
		if m.Amount != 10050 {
			t.Errorf("Expected 10050 cents, got %d", m.Amount)
		}
		if m.Currency != "USD" {
			t.Errorf("Expected USD, got %s", m.Currency)
		}
	})

	t.Run("Float64", func(t *testing.T) {
		m := Money{Amount: 10050, Currency: "USD"}
		f := m.Float64()
		if f != 100.50 {
			t.Errorf("Expected 100.50, got %f", f)
		}
	})

	t.Run("Add", func(t *testing.T) {
		m1 := Money{Amount: 1000, Currency: "USD"}
		m2 := Money{Amount: 500, Currency: "USD"}
		result := m1.Add(m2)
		if result.Amount != 1500 {
			t.Errorf("Expected 1500, got %d", result.Amount)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		m1 := Money{Amount: 1000, Currency: "USD"}
		m2 := Money{Amount: 300, Currency: "USD"}
		result := m1.Sub(m2)
		if result.Amount != 700 {
			t.Errorf("Expected 700, got %d", result.Amount)
		}
	})

	t.Run("SubNegative", func(t *testing.T) {
		m1 := Money{Amount: 100, Currency: "USD"}
		m2 := Money{Amount: 300, Currency: "USD"}
		result := m1.Sub(m2)
		if result.Amount != -200 {
			t.Errorf("Expected -200, got %d", result.Amount)
		}
	})
}

func TestPlayerStatus(t *testing.T) {
	statuses := []PlayerStatus{
		PlayerStatusPending,
		PlayerStatusActive,
		PlayerStatusSuspended,
		PlayerStatusExcluded,
		PlayerStatusClosed,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("Player status should not be empty")
		}
	}
}

func TestTransactionType(t *testing.T) {
	types := []TransactionType{
		TxTypeDeposit,
		TxTypeWithdrawal,
		TxTypeWager,
		TxTypeWin,
		TxTypeBonus,
		TxTypeAdjustment,
		TxTypeRefund,
		TxTypeJackpot,
	}

	for _, txType := range types {
		if txType == "" {
			t.Error("Transaction type should not be empty")
		}
	}
}

func TestGameSessionStatus(t *testing.T) {
	statuses := []GameSessionStatus{
		GameSessionActive,
		GameSessionCompleted,
		GameSessionInterrupted,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("Game session status should not be empty")
		}
	}
}

func TestGameCycleStatus(t *testing.T) {
	statuses := []GameCycleStatus{
		CycleStatusPending,
		CycleStatusInProgress,
		CycleStatusCompleted,
		CycleStatusVoided,
		CycleStatusInterrupted,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("Game cycle status should not be empty")
		}
	}
}

func TestEventSeverity(t *testing.T) {
	severities := []EventSeverity{
		SeverityInfo,
		SeverityWarning,
		SeverityError,
		SeverityCritical,
	}

	for _, sev := range severities {
		if sev == "" {
			t.Error("Event severity should not be empty")
		}
	}
}
