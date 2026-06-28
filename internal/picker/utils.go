//go:build !lazy_fzf

package picker

import "github.com/charmbracelet/x/ansi"

// displayWidth returns the rendered cell width of s, counting full-width glyphs
// (CJK, emoji) as 2 and ignoring ANSI escape sequences. This is the single
// width helper the picker's layout and padding math relies on so selected and
// unselected rows stay aligned for the same data.
func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

// clampWidth truncates s (ANSI-aware) so its rendered width does not exceed
// maxWidth. Styling escape sequences are preserved.
func clampWidth(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	if displayWidth(text) <= maxWidth {
		return text
	}

	return ansi.Truncate(text, maxWidth, "")
}

// truncateString trims input to maxWidth rendered cells, appending an ellipsis
// when there is room for one.
func truncateString(input string, maxWidth int) string {
	if maxWidth < 0 {
		maxWidth = 0
	}

	if displayWidth(input) <= maxWidth {
		return input
	}

	if maxWidth <= 3 {
		return ansi.Truncate(input, maxWidth, "")
	}

	return ansi.Truncate(input, maxWidth, "...")
}
