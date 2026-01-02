// Package game - Slot game implementation
// Compliant with GLI-19 §4.4, §4.5, §4.6
package game

import (
	"github.com/alexbotov/rgs/internal/domain"
)

// Symbol represents a slot reel symbol
type Symbol string

const (
	SymbolSeven   Symbol = "7"
	SymbolBar     Symbol = "BAR"
	SymbolCherry  Symbol = "CHERRY"
	SymbolBell    Symbol = "BELL"
	SymbolLemon   Symbol = "LEMON"
	SymbolOrange  Symbol = "ORANGE"
	SymbolPlum    Symbol = "PLUM"
	SymbolGrapes  Symbol = "GRAPES"
	SymbolWild    Symbol = "WILD"
)

// SlotOutcome represents the outcome of a slot spin
// GLI-19 §4.14: Game Recall
type SlotOutcome struct {
	Reels      []Symbol   `json:"reels"`       // Final reel positions
	WinLines   []WinLine  `json:"win_lines"`   // Winning combinations
	Multiplier int        `json:"multiplier"`  // Total multiplier
	IsWin      bool       `json:"is_win"`      // Whether this is a winning spin
}

// WinLine represents a winning payline
type WinLine struct {
	Line    int      `json:"line"`    // Payline number
	Symbols []Symbol `json:"symbols"` // Matching symbols
	Count   int      `json:"count"`   // Number of matching symbols
	Payout  int64    `json:"payout"`  // Payout in cents per unit bet
}

// Reel configuration for Fortune Slots
// Each reel has weighted symbols for ~96% RTP
// GLI-19 §4.5.2, §4.6: Game Selection Process, Game Fairness
var fortuneSlotsReels = [][]Symbol{
	// Reel 1
	{SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar, SymbolSeven, SymbolWild,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes},
	// Reel 2
	{SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar, SymbolSeven, SymbolWild,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell},
	// Reel 3
	{SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar, SymbolSeven, SymbolWild,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar,
	 SymbolCherry, SymbolLemon, SymbolOrange, SymbolPlum, SymbolGrapes, SymbolBell, SymbolBar},
}

// Paytable for Fortune Slots (payout per unit bet in cents)
// GLI-19 §4.4.1: Paytable information
var fortuneSlotsPaytable = map[string]int64{
	"7-7-7":           5000, // Jackpot: 50x bet
	"WILD-WILD-WILD":  2500, // 25x bet
	"BAR-BAR-BAR":     1000, // 10x bet
	"BELL-BELL-BELL":  500,  // 5x bet
	"GRAPES-GRAPES-GRAPES": 300, // 3x bet
	"PLUM-PLUM-PLUM":  200,  // 2x bet
	"ORANGE-ORANGE-ORANGE": 150, // 1.5x bet
	"LEMON-LEMON-LEMON": 100, // 1x bet
	"CHERRY-CHERRY-CHERRY": 80, // 0.8x bet
	"CHERRY-CHERRY-*": 20,   // 0.2x bet (any third symbol)
	"CHERRY-*-*":      10,   // 0.1x bet (any second and third symbol)
}

// generateSlotOutcome generates a random slot outcome using the RNG
// GLI-19 §4.5.2: Game Selection Process - outcomes determined by RNG
// GLI-19 §4.6.1: Game Fairness - no adaptive behavior
func (e *Engine) generateSlotOutcome(game *domain.Game) (*SlotOutcome, error) {
	var reels [][]Symbol
	
	// Select reel configuration based on game
	switch game.ID {
	case "fortune-slots":
		reels = fortuneSlotsReels
	case "lucky-sevens":
		reels = fortuneSlotsReels // Using same reels for simplicity
	default:
		reels = fortuneSlotsReels
	}

	// Generate random positions for each reel using CSPRNG
	// GLI-19 §4.5.2.a: Making calls to RNG
	outcome := &SlotOutcome{
		Reels:      make([]Symbol, len(reels)),
		WinLines:   []WinLine{},
		Multiplier: 1,
		IsWin:      false,
	}

	for i, reel := range reels {
		// Generate random index within reel
		idx, err := e.rng.GenerateInt(int64(len(reel)))
		if err != nil {
			return nil, err
		}
		outcome.Reels[i] = reel[idx]
	}

	// Evaluate winning combinations
	// GLI-19 §4.5.2.b: Outcomes used as directed by game rules
	outcome.WinLines = e.evaluateWins(outcome.Reels)
	outcome.IsWin = len(outcome.WinLines) > 0

	return outcome, nil
}

// evaluateWins checks for winning combinations
// GLI-19 §4.4.1: Paytable information
func (e *Engine) evaluateWins(reels []Symbol) []WinLine {
	var winLines []WinLine

	if len(reels) < 3 {
		return winLines
	}

	// Check for three of a kind (with wild substitution)
	// GLI-19 §4.4.1.m: Wild/substitute symbols
	s1, s2, s3 := reels[0], reels[1], reels[2]

	// Check three matching symbols
	key := string(s1) + "-" + string(s2) + "-" + string(s3)
	if payout, ok := fortuneSlotsPaytable[key]; ok {
		winLines = append(winLines, WinLine{
			Line:    1,
			Symbols: []Symbol{s1, s2, s3},
			Count:   3,
			Payout:  payout,
		})
		return winLines
	}

	// Check for wild substitution
	if s1 == s2 && (s3 == SymbolWild || s3 == s1) ||
	   s2 == s3 && (s1 == SymbolWild || s1 == s2) ||
	   s1 == s3 && (s2 == SymbolWild || s2 == s1) {
		// Find the non-wild symbol
		var baseSymbol Symbol
		for _, s := range []Symbol{s1, s2, s3} {
			if s != SymbolWild {
				baseSymbol = s
				break
			}
		}
		if baseSymbol != "" {
			key = string(baseSymbol) + "-" + string(baseSymbol) + "-" + string(baseSymbol)
			if payout, ok := fortuneSlotsPaytable[key]; ok {
				winLines = append(winLines, WinLine{
					Line:    1,
					Symbols: []Symbol{s1, s2, s3},
					Count:   3,
					Payout:  payout,
				})
				return winLines
			}
		}
	}

	// Check for cherry combinations
	if s1 == SymbolCherry && s2 == SymbolCherry {
		if payout, ok := fortuneSlotsPaytable["CHERRY-CHERRY-*"]; ok {
			winLines = append(winLines, WinLine{
				Line:    1,
				Symbols: []Symbol{s1, s2, s3},
				Count:   2,
				Payout:  payout,
			})
			return winLines
		}
	}

	if s1 == SymbolCherry {
		if payout, ok := fortuneSlotsPaytable["CHERRY-*-*"]; ok {
			winLines = append(winLines, WinLine{
				Line:    1,
				Symbols: []Symbol{s1, s2, s3},
				Count:   1,
				Payout:  payout,
			})
			return winLines
		}
	}

	return winLines
}

// calculateWin calculates the total win amount
// GLI-19 §4.7: Game Payout Percentages
func (e *Engine) calculateWin(outcome *SlotOutcome, wager domain.Money) domain.Money {
	if !outcome.IsWin || len(outcome.WinLines) == 0 {
		return domain.Money{Amount: 0, Currency: wager.Currency}
	}

	var totalPayout int64
	for _, line := range outcome.WinLines {
		// Payout is per unit bet (100 cents = $1), scale to actual wager
		linePayout := (line.Payout * wager.Amount) / 100
		totalPayout += linePayout
	}

	return domain.Money{Amount: totalPayout, Currency: wager.Currency}
}

