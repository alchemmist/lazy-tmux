//go:build !lazy_fzf

package picker

func truncateString(input string, maxRunes int) string {
	if maxRunes < 0 {
		maxRunes = 0
	}

	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}

	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}

	return string(runes[:maxRunes-3]) + "..."
}
