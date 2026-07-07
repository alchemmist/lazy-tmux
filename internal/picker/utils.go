//go:build !lazy_fzf

package picker

import "github.com/charmbracelet/x/ansi"

const truncationEllipsis = "..."

func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

func clampWidth(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	if displayWidth(text) <= maxWidth {
		return text
	}

	return ansi.Truncate(text, maxWidth, "")
}

func truncateString(input string, maxWidth int) string {
	if maxWidth < 0 {
		maxWidth = 0
	}

	if displayWidth(input) <= maxWidth {
		return input
	}

	if maxWidth <= len(truncationEllipsis) {
		return ansi.Truncate(input, maxWidth, "")
	}

	return ansi.Truncate(input, maxWidth, truncationEllipsis)
}
