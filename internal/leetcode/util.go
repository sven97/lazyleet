package leetcode

import (
	"strconv"
	"strings"
)

// parseFrontendID extracts the leading integer from a LeetCode frontend id.
// Most are plain numbers ("1"); some contest problems are like "LCP 01" or
// "面试题 01.01" — those yield 0.
func parseFrontendID(s string) int {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	// take a leading run of digits if present
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 {
		if n, err := strconv.Atoi(s[:i]); err == nil {
			return n
		}
	}
	return 0
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// titleCaseDifficulty normalises "EASY"/"easy"/"Easy" to "Easy".
func titleCaseDifficulty(d string) string {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case "easy":
		return "Easy"
	case "medium":
		return "Medium"
	case "hard":
		return "Hard"
	default:
		return d
	}
}
