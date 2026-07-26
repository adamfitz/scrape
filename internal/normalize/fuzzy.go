package normalize

import "strings"

// Similarity returns a 0.0–1.0 score for how similar a and b are.
// Both inputs are normalised (FoldUnicode + lowercase) before scoring.
// Returns 0.0 for empty inputs.
func Similarity(a, b string) float64 {
	a = strings.ToLower(FoldUnicode(a))
	b = strings.ToLower(FoldUnicode(b))

	if a == "" || b == "" {
		return 0.0
	}
	if a == b {
		return 1.0
	}

	// Substring containment
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return 0.95
	}

	// Token Jaccard (word-level set similarity)
	tj := tokenJaccard(a, b)

	// Levenshtein (character-level edit distance)
	lv := levenshteinSimilarity(a, b)

	if tj > lv {
		return tj
	}
	return lv
}

// Match returns true if a and b are similar enough (>= threshold).
func Match(a, b string, threshold float64) bool {
	return Similarity(a, b) >= threshold
}

// BestMatch finds the best match for query in candidates.
// Returns (index, score, ok). ok is false if nothing meets threshold.
func BestMatch(query string, candidates []string, threshold float64) (int, float64, bool) {
	bestIdx := -1
	bestScore := 0.0
	for i, c := range candidates {
		score := Similarity(query, c)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 || bestScore < threshold {
		return -1, 0, false
	}
	return bestIdx, bestScore, true
}

// tokenJaccard computes Jaccard similarity between word token sets.
func tokenJaccard(a, b string) float64 {
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) == 0 && len(bTokens) == 0 {
		return 0.0
	}
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0.0
	}

	aSet := make(map[string]bool, len(aTokens))
	for _, t := range aTokens {
		aSet[t] = true
	}

	intersection := 0
	for _, t := range bTokens {
		if aSet[t] {
			intersection++
		}
	}

	union := len(aTokens) + len(bTokens) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// levenshtein computes the Levenshtein edit distance between two rune slices.
func levenshtein(a, b []rune) int {
	la := len(a)
	lb := len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row DP for memory efficiency.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// levenshteinSimilarity normalizes Levenshtein distance to a 0.0–1.0 similarity score.
func levenshteinSimilarity(a, b string) float64 {
	ra := []rune(a)
	rb := []rune(b)
	dist := levenshtein(ra, rb)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// min3 returns the smallest of three integers.
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
