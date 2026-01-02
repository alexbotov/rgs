package rng

import (
	"math"
	"testing"
)

func TestGenerateBytes(t *testing.T) {
	s := New()

	t.Run("GeneratesCorrectLength", func(t *testing.T) {
		for _, size := range []int{1, 8, 16, 32, 64, 128, 256} {
			bytes, err := s.GenerateBytes(size)
			if err != nil {
				t.Fatalf("Failed to generate %d bytes: %v", size, err)
			}
			if len(bytes) != size {
				t.Errorf("Expected %d bytes, got %d", size, len(bytes))
			}
		}
	})

	t.Run("GeneratesUniqueValues", func(t *testing.T) {
		// Generate multiple samples and verify they're different
		samples := make([][]byte, 100)
		for i := 0; i < 100; i++ {
			bytes, err := s.GenerateBytes(16)
			if err != nil {
				t.Fatalf("Failed to generate bytes: %v", err)
			}
			samples[i] = bytes
		}

		// Check for duplicates (extremely unlikely with 128-bit values)
		seen := make(map[string]bool)
		for _, sample := range samples {
			key := string(sample)
			if seen[key] {
				t.Error("Duplicate value generated - extremely unlikely, possible RNG issue")
			}
			seen[key] = true
		}
	})
}

func TestGenerateInt(t *testing.T) {
	s := New()

	t.Run("GeneratesWithinRange", func(t *testing.T) {
		for _, max := range []int64{2, 10, 100, 1000, 10000} {
			for i := 0; i < 1000; i++ {
				n, err := s.GenerateInt(max)
				if err != nil {
					t.Fatalf("Failed to generate int: %v", err)
				}
				if n < 0 || n >= max {
					t.Errorf("Generated value %d out of range [0, %d)", n, max)
				}
			}
		}
	})

	t.Run("RejectsZeroOrNegative", func(t *testing.T) {
		_, err := s.GenerateInt(0)
		if err == nil {
			t.Error("Expected error for max=0")
		}

		_, err = s.GenerateInt(-1)
		if err == nil {
			t.Error("Expected error for max=-1")
		}
	})

	t.Run("UniformDistribution", func(t *testing.T) {
		// Test uniform distribution with chi-square
		const max = 10
		const samples = 100000
		counts := make([]int, max)

		for i := 0; i < samples; i++ {
			n, err := s.GenerateInt(max)
			if err != nil {
				t.Fatalf("Failed to generate int: %v", err)
			}
			counts[n]++
		}

		// Chi-square test
		expected := float64(samples) / float64(max)
		var chiSquare float64
		for _, count := range counts {
			diff := float64(count) - expected
			chiSquare += (diff * diff) / expected
		}

		// Critical value for 9 DOF at 99% confidence is ~21.67
		if chiSquare > 25 {
			t.Errorf("Chi-square test failed: %f (expected < 25)", chiSquare)
		}
	})
}

func TestGenerateIntRange(t *testing.T) {
	s := New()

	t.Run("GeneratesWithinRange", func(t *testing.T) {
		testCases := []struct {
			min, max int64
		}{
			{0, 10},
			{5, 15},
			{-10, 10},
			{100, 200},
		}

		for _, tc := range testCases {
			for i := 0; i < 100; i++ {
				n, err := s.GenerateIntRange(tc.min, tc.max)
				if err != nil {
					t.Fatalf("Failed to generate int range: %v", err)
				}
				if n < tc.min || n > tc.max {
					t.Errorf("Generated value %d out of range [%d, %d]", n, tc.min, tc.max)
				}
			}
		}
	})

	t.Run("RejectsInvalidRange", func(t *testing.T) {
		_, err := s.GenerateIntRange(10, 5)
		if err == nil {
			t.Error("Expected error for min > max")
		}
	})

	t.Run("SingleValueRange", func(t *testing.T) {
		n, err := s.GenerateIntRange(5, 5)
		if err != nil {
			t.Fatalf("Failed to generate single value range: %v", err)
		}
		if n != 5 {
			t.Errorf("Expected 5, got %d", n)
		}
	})
}

func TestGenerateFloat(t *testing.T) {
	s := New()

	t.Run("GeneratesWithinRange", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			f, err := s.GenerateFloat()
			if err != nil {
				t.Fatalf("Failed to generate float: %v", err)
			}
			if f < 0.0 || f >= 1.0 {
				t.Errorf("Generated value %f out of range [0.0, 1.0)", f)
			}
		}
	})

	t.Run("HasGoodPrecision", func(t *testing.T) {
		// Check that we get fine-grained values, not just coarse buckets
		seen := make(map[float64]bool)
		for i := 0; i < 1000; i++ {
			f, _ := s.GenerateFloat()
			seen[f] = true
		}

		// Should have many unique values
		if len(seen) < 990 {
			t.Errorf("Expected near-unique values, got %d unique out of 1000", len(seen))
		}
	})
}

func TestShuffle(t *testing.T) {
	s := New()

	t.Run("PreservesElements", func(t *testing.T) {
		original := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		shuffled := make([]int, len(original))
		copy(shuffled, original)

		if err := s.Shuffle(shuffled); err != nil {
			t.Fatalf("Failed to shuffle: %v", err)
		}

		// Check all elements present
		seen := make(map[int]bool)
		for _, v := range shuffled {
			if seen[v] {
				t.Error("Duplicate element after shuffle")
			}
			seen[v] = true
		}

		for _, v := range original {
			if !seen[v] {
				t.Errorf("Element %d missing after shuffle", v)
			}
		}
	})

	t.Run("ActuallySHuffles", func(t *testing.T) {
		// Run shuffle many times and check it's not always the same
		original := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		differentCount := 0

		for i := 0; i < 100; i++ {
			shuffled := make([]int, len(original))
			copy(shuffled, original)
			s.Shuffle(shuffled)

			// Check if different from original
			different := false
			for j := range original {
				if original[j] != shuffled[j] {
					different = true
					break
				}
			}
			if different {
				differentCount++
			}
		}

		// Should be different most of the time (probability of same order is 1/10!)
		if differentCount < 99 {
			t.Errorf("Shuffle produced identical order too often: %d/100 were different", differentCount)
		}
	})
}

func TestSelectWeighted(t *testing.T) {
	s := New()

	t.Run("SelectsWithinBounds", func(t *testing.T) {
		weights := []float64{1.0, 2.0, 3.0, 4.0}
		for i := 0; i < 1000; i++ {
			idx, err := s.SelectWeighted(weights)
			if err != nil {
				t.Fatalf("Failed weighted selection: %v", err)
			}
			if idx < 0 || idx >= len(weights) {
				t.Errorf("Selected index %d out of bounds", idx)
			}
		}
	})

	t.Run("RespectsWeights", func(t *testing.T) {
		// Heavy weight on first element
		weights := []float64{9.0, 1.0}
		counts := make([]int, 2)

		for i := 0; i < 10000; i++ {
			idx, _ := s.SelectWeighted(weights)
			counts[idx]++
		}

		// First element should be selected ~90% of the time
		ratio := float64(counts[0]) / float64(counts[0]+counts[1])
		if ratio < 0.85 || ratio > 0.95 {
			t.Errorf("Weight distribution off: expected ~0.9, got %f", ratio)
		}
	})

	t.Run("HandlesZeroWeight", func(t *testing.T) {
		weights := []float64{0.0, 1.0, 0.0}
		for i := 0; i < 100; i++ {
			idx, err := s.SelectWeighted(weights)
			if err != nil {
				t.Fatalf("Failed with zero weight: %v", err)
			}
			if idx != 1 {
				t.Errorf("Should only select index 1, got %d", idx)
			}
		}
	})

	t.Run("RejectsEmptyWeights", func(t *testing.T) {
		_, err := s.SelectWeighted([]float64{})
		if err == nil {
			t.Error("Expected error for empty weights")
		}
	})

	t.Run("RejectsNegativeWeight", func(t *testing.T) {
		_, err := s.SelectWeighted([]float64{1.0, -1.0, 1.0})
		if err == nil {
			t.Error("Expected error for negative weight")
		}
	})
}

func TestHealthCheck(t *testing.T) {
	s := New()

	result, err := s.HealthCheck()
	if err != nil {
		t.Fatalf("Health check error: %v", err)
	}

	if !result.Healthy {
		t.Error("RNG reported unhealthy")
	}

	if !result.ChiSquarePassed {
		t.Errorf("Chi-square test failed with value %f", result.ChiSquare)
	}

	// Chi-square should be reasonable (not too high or too low)
	// For 99 DOF, values between 50-150 are typical
	if result.ChiSquare < 20 || result.ChiSquare > 200 {
		t.Logf("Warning: Chi-square value %f is unusual (expected 50-150 range)", result.ChiSquare)
	}
}

func TestChiSquareTest(t *testing.T) {
	s := New()

	t.Run("PassesForUniformData", func(t *testing.T) {
		// Generate uniform data
		samples := make([]int64, 10000)
		for i := 0; i < len(samples); i++ {
			samples[i], _ = s.GenerateInt(100)
		}

		chiSquare, passed := s.chiSquareTest(samples, 100)
		if !passed {
			t.Errorf("Chi-square test failed for uniform RNG data: %f", chiSquare)
		}
	})

	t.Run("FailsForBiasedData", func(t *testing.T) {
		// Create heavily biased data
		samples := make([]int64, 10000)
		for i := 0; i < len(samples); i++ {
			samples[i] = 0 // All same value
		}

		_, passed := s.chiSquareTest(samples, 100)
		if passed {
			t.Error("Chi-square test should fail for heavily biased data")
		}
	})
}

// Benchmark tests
func BenchmarkGenerateInt(b *testing.B) {
	s := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GenerateInt(1000)
	}
}

func BenchmarkGenerateFloat(b *testing.B) {
	s := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.GenerateFloat()
	}
}

func BenchmarkShuffle(b *testing.B) {
	s := New()
	slice := make([]int, 52) // Deck of cards
	for i := range slice {
		slice[i] = i
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Shuffle(slice)
	}
}

func BenchmarkSelectWeighted(b *testing.B) {
	s := New()
	weights := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SelectWeighted(weights)
	}
}

// Statistical tests to verify RNG quality
func TestStatisticalQuality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping statistical tests in short mode")
	}

	s := New()

	t.Run("MeanAndVariance", func(t *testing.T) {
		const samples = 100000
		const max = 100
		var sum, sumSq float64

		for i := 0; i < samples; i++ {
			n, _ := s.GenerateInt(max)
			sum += float64(n)
			sumSq += float64(n * n)
		}

		mean := sum / float64(samples)
		variance := (sumSq / float64(samples)) - (mean * mean)

		// Expected mean for uniform [0, 100) is 49.5
		expectedMean := float64(max-1) / 2.0
		if math.Abs(mean-expectedMean) > 0.5 {
			t.Errorf("Mean deviation too large: got %f, expected ~%f", mean, expectedMean)
		}

		// Expected variance for uniform [0, 100) is (100^2 - 1) / 12 ≈ 833.25
		expectedVariance := float64(max*max-1) / 12.0
		if math.Abs(variance-expectedVariance) > 20 {
			t.Errorf("Variance deviation too large: got %f, expected ~%f", variance, expectedVariance)
		}
	})

	t.Run("SerialCorrelation", func(t *testing.T) {
		const samples = 100000
		values := make([]float64, samples)

		for i := 0; i < samples; i++ {
			values[i], _ = s.GenerateFloat()
		}

		// Calculate lag-1 correlation
		var sumXY, sumX, sumY, sumX2, sumY2 float64
		n := float64(samples - 1)

		for i := 0; i < samples-1; i++ {
			x, y := values[i], values[i+1]
			sumXY += x * y
			sumX += x
			sumY += y
			sumX2 += x * x
			sumY2 += y * y
		}

		correlation := (n*sumXY - sumX*sumY) /
			(math.Sqrt(n*sumX2-sumX*sumX) * math.Sqrt(n*sumY2-sumY*sumY))

		// Correlation should be very close to 0 for independent values
		if math.Abs(correlation) > 0.01 {
			t.Errorf("Serial correlation too high: %f (expected near 0)", correlation)
		}
	})
}

