package manabase

import "testing"

func TestChooseKnownValues(t *testing.T) {
	// Hand-computed binomials as sanity anchors.
	expect := func(n, k int, want float64) {
		got := choose(n, k)
		if abs(got-want) > 1e-9 {
			t.Errorf("choose(%d,%d) = %v, want %v", n, k, got, want)
		}
	}
	expect(5, 2, 10)
	expect(10, 3, 120)
	expect(100, 0, 1)
	expect(100, 100, 1)
	expect(0, 0, 1)
}

func TestAtLeastMonteIndependent(t *testing.T) {
	// P(X >= 2) with 10 successes in 60 cards drawn 7 times: a classic quick sanity
	// value is between 0 and 1 and monotonic as atLeast rises.
	p1 := atLeast(60, 10, 7, 1)
	p2 := atLeast(60, 10, 7, 2)
	p3 := atLeast(60, 10, 7, 3)
	if p1 <= 0 || p1 > 1 || p2 <= 0 || p2 > 1 || p3 < 0 || p3 > 1 {
		t.Fatalf("probabilities out of range: %v %v %v", p1, p2, p3)
	}
	if p2 > p1 || p3 > p2 {
		t.Fatalf("not monotonic decreasing: %v %v %v", p1, p2, p3)
	}
}

func TestExactlySumsToAtMostOne(t *testing.T) {
	// The exact probability mass over all hit counts must sum to 1 (within fp error)
	// for a valid draw.
	total := 0.0
	for hits := 0; hits <= 7; hits++ {
		total += exactly(60, 10, 7, hits)
	}
	if abs(total-1.0) > 1e-6 {
		t.Fatalf("exact probabilities sum to %v, want ~1.0", total)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
