package manabase

import "math"

// Hypergeometric probability helpers used by the mana-base analyzer. All draws are
// without replacement, modeling cards seen from a shuffled library.
//
// Combinatorics are evaluated in log-space against a precomputed log-factorial
// table so that 100-card decks with double-digit draw counts do not overflow a
// float64. This is a direct port of DeckFlow.Core.Manabase.Hypergeometric.
const maxN = 512

var logFactorial = buildLogFactorial()

func buildLogFactorial() []float64 {
	table := make([]float64, maxN+1)
	table[0] = 0.0
	for i := 1; i <= maxN; i++ {
		table[i] = table[i-1] + math.Log(float64(i))
	}
	return table
}

// logChoose returns the natural log of the binomial coefficient C(n, k). Out-of-range
// inputs (n > maxN, k < 0, k > n, n < 0) yield -Inf, matching DeckFlow's behavior of
// returning double.NegativeInfinity for invalid combinations.
func logChoose(n, k int) float64 {
	if n > maxN {
		return math.Inf(-1)
	}
	if k < 0 || k > n || n < 0 {
		return math.Inf(-1)
	}
	return logFactorial[n] - logFactorial[k] - logFactorial[n-k]
}

// choose returns the binomial coefficient C(n, k) as a float64.
func choose(n, k int) float64 {
	log := logChoose(n, k)
	if math.IsInf(log, -1) {
		return 0.0
	}
	return math.Exp(log)
}

// exactly returns P(X = hits) when drawing `draws` cards from a population of
// `population` cards containing `successes` winners.
func exactly(population, successes, draws, hits int) float64 {
	if hits < 0 || hits > successes || draws-hits > population-successes || draws > population || draws < 0 {
		return 0.0
	}
	log := logChoose(successes, hits) +
		logChoose(population-successes, draws-hits) -
		logChoose(population, draws)
	return math.Exp(log)
}

// atLeast returns P(X >= atLeast) — the chance of drawing at least that many winners.
// Sums the shorter tail for numerical stability, matching DeckFlow.
func atLeast(population, successes, draws, atLeast int) float64 {
	if atLeast <= 0 {
		return 1.0
	}
	max := successes
	if draws < max {
		max = draws
	}
	if atLeast > max {
		return 0.0
	}
	sum := 0.0
	for hits := atLeast; hits <= max; hits++ {
		sum += exactly(population, successes, draws, hits)
	}
	return clamp1(sum)
}

func clamp1(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
