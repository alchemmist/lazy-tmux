//go:build !lazy_fzf

package picker

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The picker palette is aligned with the "moss-dark" terminal theme: a muted,
// desaturated set of pastels on near-black. Amber is the browse-mode accent
// (title, cursor, selected stripe, key hints); red is reserved for errors. This
// keeps the picker feeling native in the terminal and in tune with the
// monochrome lazy-tmux brand. (Per-mode accents are tracked in #163.)
const (
	colAccent  = "#d7875f" // amber/orange (moss-dark "yellow") — browse-mode accent
	colText    = "#d0d0d0" // primary text (session / window names)
	colMeta    = "#9e9e9e" // dimmed meta columns (cmd, captured, …)
	colFaint   = "#585858" // tree branches, header labels, separators
	colBorder  = "#444444" // frame
	colSelBg   = "#303030" // selected-row background bar
	colSelText = "#f0f0f0" // selected-row text
	colError   = "#d75f5f" // error status
	colCount   = "#9e9e9e" // header counts
)

type pickerTheme struct {
	border     lipgloss.Style
	title      lipgloss.Style
	count      lipgloss.Style
	prompt     lipgloss.Style
	headerCell lipgloss.Style
	name       lipgloss.Style
	session    lipgloss.Style
	meta       lipgloss.Style
	faint      lipgloss.Style
	stripe     lipgloss.Style
	selBar     lipgloss.Style
	statusErr  lipgloss.Style
	helpKey    lipgloss.Style
	helpText   lipgloss.Style
}

func newPickerTheme() pickerTheme {
	return pickerTheme{
		border:     lipgloss.NewStyle().Foreground(lipgloss.Color(colBorder)),
		title:      lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true),
		count:      lipgloss.NewStyle().Foreground(lipgloss.Color(colCount)),
		prompt:     lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)),
		headerCell: lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)),
		name:       lipgloss.NewStyle().Foreground(lipgloss.Color(colText)),
		session:    lipgloss.NewStyle().Foreground(lipgloss.Color(colText)).Bold(true),
		meta:       lipgloss.NewStyle().Foreground(lipgloss.Color(colMeta)),
		faint:      lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)),
		stripe:     lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)),
		selBar: lipgloss.NewStyle().
			Background(lipgloss.Color(colSelBg)).
			Foreground(lipgloss.Color(colSelText)),
		statusErr: lipgloss.NewStyle().Foreground(lipgloss.Color(colError)),
		helpKey:   lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)),
		helpText:  lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)),
	}
}

// frameTop draws the top border with an embedded title on the left and an
// optional count on the right, e.g. "╭─ title ─── right ─╮".
func (t pickerTheme) frameTop(title, right string, width int) string {
	return t.frameBar('╭', '╮', t.title.Render(title), right, width)
}

// frameBottom draws the bottom border with embedded key hints on the left.
func (t pickerTheme) frameBottom(hints string, width int) string {
	return t.frameBar('╰', '╯', hints, "", width)
}

// frameBar builds one horizontal border line with pre-styled left/right labels
// and a dashed fill sized so the whole line is exactly width cells wide.
//
//	with right:  ╭─ left ──────── right ─╮   (head 3 + Lₗ + 1 + fill + 1 + Lᵣ + 3)
//	without:     ╰─ left ───────────────╯   (head 3 + Lₗ + 1 + fill + 2)
func (t pickerTheme) frameBar(left, right rune, leftLabel, rightLabel string, width int) string {
	// Clamp the labels so they can never push the line past width on a narrow
	// terminal (only the dashed fill would otherwise absorb the overflow, and a
	// fill of zero still leaves an over-wide line that breaks the frame math).
	if rightLabel != "" {
		budget := max(0, width-8) // head 3 + 2 inner spaces + tail 3
		rightLabel = clampWidth(rightLabel, budget)
		leftLabel = clampWidth(leftLabel, budget-displayWidth(rightLabel))

		var buf strings.Builder

		fill := max(0, budget-displayWidth(leftLabel)-displayWidth(rightLabel))
		buf.WriteString(t.border.Render(string(left) + "─ "))
		buf.WriteString(leftLabel)
		buf.WriteString(t.border.Render(" " + strings.Repeat("─", fill) + " "))
		buf.WriteString(rightLabel)
		buf.WriteString(t.border.Render(" ─" + string(right)))

		return buf.String()
	}

	budget := max(0, width-6) // head 3 + 1 inner space + tail 2
	leftLabel = clampWidth(leftLabel, budget)

	var buf strings.Builder

	fill := max(0, budget-displayWidth(leftLabel))
	buf.WriteString(t.border.Render(string(left) + "─ "))
	buf.WriteString(leftLabel)
	buf.WriteString(t.border.Render(" " + strings.Repeat("─", fill) + "─" + string(right)))

	return buf.String()
}

// frameLine wraps one inner content line with the side borders and a single
// space gutter on each side, padding the content to the available width.
func (t pickerTheme) frameLine(content string, width int) string {
	inner := max(0, width-4) // 2 borders + 2 gutter spaces
	content = clampWidth(content, inner)
	pad := max(0, inner-displayWidth(content))

	side := t.border.Render("│")

	return side + " " + content + strings.Repeat(" ", pad) + " " + side
}
