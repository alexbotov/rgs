// Package rng provides a cryptographically strong Random Number Generator
// Compliant with GLI-19 Chapter 3: RNG Requirements
package rng

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
	"time"
)

// Service provides cryptographically strong random number generation
// GLI-19 §3.2: General RNG Requirements
// GLI-19 §3.3: RNG Strength and Monitoring
type Service struct {
	entropy io.Reader
	mu      sync.Mutex

	// Statistics for monitoring
	lastHealthCheck time.Time
	samplesGenerated int64
}

// New creates a new RNG service using crypto/rand
func New() *Service {
	return &Service{
		entropy:         rand.Reader,
		lastHealthCheck: time.Now(),
	}
}

// GenerateBytes returns n cryptographically random bytes
// GLI-19 §3.3.1: RNG Strength for Outcome Determination
func (s *Service) GenerateBytes(n int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := make([]byte, n)
	if _, err := io.ReadFull(s.entropy, buf); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	s.samplesGenerated++
	return buf, nil
}

// GenerateInt returns a random integer in range [0, max)
// Uses rejection sampling to eliminate modulo bias (GLI-19 §3.2.3)
func (s *Service) GenerateInt(max int64) (int64, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate threshold for rejection sampling to eliminate bias
	// We need to reject values >= threshold to ensure uniform distribution
	threshold := uint64(1<<63-1) - (uint64(1<<63-1) % uint64(max))

	for {
		buf := make([]byte, 8)
		if _, err := io.ReadFull(s.entropy, buf); err != nil {
			return 0, fmt.Errorf("failed to generate random int: %w", err)
		}

		n := binary.BigEndian.Uint64(buf) >> 1 // Use 63 bits for positive range

		if n < threshold {
			s.samplesGenerated++
			return int64(n % uint64(max)), nil
		}
		// Reject and retry to avoid modulo bias
	}
}

// GenerateIntRange returns a random integer in range [min, max]
func (s *Service) GenerateIntRange(min, max int64) (int64, error) {
	if min > max {
		return 0, fmt.Errorf("min cannot be greater than max")
	}

	rangeSize := max - min + 1
	n, err := s.GenerateInt(rangeSize)
	if err != nil {
		return 0, err
	}

	return min + n, nil
}

// GenerateFloat returns a random float in range [0.0, 1.0)
func (s *Service) GenerateFloat() (float64, error) {
	n, err := s.GenerateInt(1 << 53) // 53 bits of precision
	if err != nil {
		return 0, err
	}
	return float64(n) / float64(1<<53), nil
}

// Shuffle performs a Fisher-Yates shuffle on a slice of integers
// GLI-19 §3.2.1: Source Code Review for shuffling algorithms
func (s *Service) Shuffle(slice []int) error {
	for i := len(slice) - 1; i > 0; i-- {
		j, err := s.GenerateInt(int64(i + 1))
		if err != nil {
			return err
		}
		slice[i], slice[int(j)] = slice[int(j)], slice[i]
	}
	return nil
}

// SelectWeighted selects an index based on weighted probabilities
// GLI-19 §3.2.3: Distribution - non-uniform distribution support
func (s *Service) SelectWeighted(weights []float64) (int, error) {
	if len(weights) == 0 {
		return 0, fmt.Errorf("weights cannot be empty")
	}

	// Calculate total weight
	var total float64
	for _, w := range weights {
		if w < 0 {
			return 0, fmt.Errorf("weights cannot be negative")
		}
		total += w
	}

	if total <= 0 {
		return 0, fmt.Errorf("total weight must be positive")
	}

	// Generate random value
	r, err := s.GenerateFloat()
	if err != nil {
		return 0, err
	}

	// Scale to total weight
	target := r * total

	// Find the selected index
	var cumulative float64
	for i, w := range weights {
		cumulative += w
		if target < cumulative {
			return i, nil
		}
	}

	// Should not reach here, but return last index as fallback
	return len(weights) - 1, nil
}

// HealthCheck verifies RNG is functioning correctly
// GLI-19 §3.3.3: Dynamic Output Monitoring
func (s *Service) HealthCheck() (*HealthResult, error) {
	s.mu.Lock()
	s.lastHealthCheck = time.Now()
	s.mu.Unlock()

	// Generate test samples
	const sampleSize = 1000
	samples := make([]int64, sampleSize)

	for i := 0; i < sampleSize; i++ {
		n, err := s.GenerateInt(100)
		if err != nil {
			return &HealthResult{
				Healthy:   false,
				Timestamp: time.Now(),
				Error:     err.Error(),
			}, err
		}
		samples[i] = n
	}

	// Run basic chi-square test
	chiSquare, passed := s.chiSquareTest(samples, 100)

	return &HealthResult{
		Healthy:          passed,
		Timestamp:        time.Now(),
		SamplesGenerated: s.samplesGenerated,
		ChiSquare:        chiSquare,
		ChiSquarePassed:  passed,
	}, nil
}

// chiSquareTest performs a basic chi-square test for uniformity
// GLI-19 §3.2.2: Statistical Analysis
func (s *Service) chiSquareTest(samples []int64, bins int) (float64, bool) {
	// Count occurrences in each bin
	counts := make([]int, bins)
	for _, sample := range samples {
		counts[int(sample)%bins]++
	}

	// Calculate expected count per bin
	expected := float64(len(samples)) / float64(bins)

	// Calculate chi-square statistic
	var chiSquare float64
	for _, count := range counts {
		diff := float64(count) - expected
		chiSquare += (diff * diff) / expected
	}

	// Critical value for 99 bins (bins-1 degrees of freedom) at 99% confidence
	// For 99 DOF, critical value is approximately 134.6
	criticalValue := 134.6
	if bins != 100 {
		// Approximate critical value for other bin counts
		criticalValue = float64(bins-1) + 2.576*math.Sqrt(2.0*float64(bins-1))
	}

	return chiSquare, chiSquare < criticalValue
}

// HealthResult contains RNG health check results
type HealthResult struct {
	Healthy          bool      `json:"healthy"`
	Timestamp        time.Time `json:"timestamp"`
	SamplesGenerated int64     `json:"samples_generated"`
	ChiSquare        float64   `json:"chi_square"`
	ChiSquarePassed  bool      `json:"chi_square_passed"`
	Error            string    `json:"error,omitempty"`
}

