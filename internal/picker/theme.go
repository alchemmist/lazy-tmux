//go:build !lazy_fzf

package picker

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	colAccent  = "#d7875f"
	colText    = "#d0d0d0"
	colMeta    = "#9e9e9e"
	colFaint   = "#585858"
	colBorder  = "#444444"
	colSelBg   = "#303030"
	colSelText = "#f0f0f0"
	colError   = "#d75f5f"
	colCount   = "#9e9e9e"

	colDelete    = "#d75f5f"
	colRename    = "#5f87af"
	colNew       = "#87af87"
	colSleepWake = "#8787af"
)

const (
	themeDark  = "dark"
	themeLight = "light"

	lightText    = "#000000"
	lightMeta    = "#666666"
	lightFaint   = "#888888"
	lightBorder  = "#000000"
	lightSelBg   = "#e6e6e6"
	lightSelText = "#000000"
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
	mark       lipgloss.Style
	statusErr  lipgloss.Style
	helpKey    lipgloss.Style
	helpText   lipgloss.Style

	statusWorking          lipgloss.Style
	statusAwaitingDecision lipgloss.Style
	statusAwaitingInput    lipgloss.Style
	statusIdle             lipgloss.Style
	statusError            lipgloss.Style
	selBg                  string
	selText                string
}

const (
	glyphWorking          = "●"
	glyphAwaitingDecision = "●"
	glyphAwaitingInput    = "●"
	glyphIdle             = "○"
	glyphError            = "×"
)

//nolint:gochecknoglobals // static spinner frame table, never mutated
var workingSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

func statusGlyphFrame(status WindowStatus, frame int) string {
	if status == StatusWorking {
		return workingSpinnerFrames[frame%len(workingSpinnerFrames)]
	}

	return statusGlyph(status)
}

func statusGlyph(status WindowStatus) string {
	switch status {
	case StatusWorking:
		return glyphWorking
	case StatusAwaitingDecision:
		return glyphAwaitingDecision
	case StatusAwaitingInput:
		return glyphAwaitingInput
	case StatusIdle:
		return glyphIdle
	case StatusError:
		return glyphError
	default:
		return ""
	}
}

func (t pickerTheme) statusStyle(status WindowStatus) lipgloss.Style {
	switch status {
	case StatusWorking:
		return t.statusWorking
	case StatusAwaitingDecision:
		return t.statusAwaitingDecision
	case StatusAwaitingInput:
		return t.statusAwaitingInput
	case StatusIdle:
		return t.statusIdle
	case StatusError:
		return t.statusError
	default:
		return lipgloss.NewStyle()
	}
}

func (t pickerTheme) statusStyleOn(status WindowStatus, selected bool) lipgloss.Style {
	style := t.statusStyle(status)
	if selected {
		style = style.Background(lipgloss.Color(t.selBg))
	}

	return style
}

func (t pickerTheme) markStyle(selected bool) lipgloss.Style {
	if selected {
		return t.mark.Background(lipgloss.Color(t.selBg))
	}

	return t.mark
}

func accentForMode(mode actionMode) string {
	if cmd, ok := commandForMode(mode); ok {
		return cmd.accent
	}

	return colAccent
}

func newPickerThemeFor(name, accent string) pickerTheme {
	border := colBorder
	text, meta, faint, selBG, selText, errColor := colText, colMeta, colFaint, colSelBg, colSelText, colError
	statusNew, statusAccent := colNew, colAccent
	if name == themeLight {
		border, text, meta, faint = lightBorder, lightText, lightMeta, lightFaint
		selBG, selText = lightSelBg, lightSelText
	}
	if accent != colAccent {
		border = accent
	}

	return pickerTheme{
		border:     lipgloss.NewStyle().Foreground(lipgloss.Color(border)),
		title:      lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true),
		count:      lipgloss.NewStyle().Foreground(lipgloss.Color(meta)),
		prompt:     lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		headerCell: lipgloss.NewStyle().Foreground(lipgloss.Color(faint)),
		name:       lipgloss.NewStyle().Foreground(lipgloss.Color(text)),
		session:    lipgloss.NewStyle().Foreground(lipgloss.Color(text)).Bold(true),
		meta:       lipgloss.NewStyle().Foreground(lipgloss.Color(meta)),
		faint:      lipgloss.NewStyle().Foreground(lipgloss.Color(faint)),
		stripe:     lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		selBar: lipgloss.NewStyle().
			Background(lipgloss.Color(selBG)).
			Foreground(lipgloss.Color(selText)),
		mark:      lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		statusErr: lipgloss.NewStyle().Foreground(lipgloss.Color(errColor)),
		helpKey:   lipgloss.NewStyle().Foreground(lipgloss.Color(accent)),
		helpText:  lipgloss.NewStyle().Foreground(lipgloss.Color(faint)),

		statusWorking:          lipgloss.NewStyle().Foreground(lipgloss.Color(statusNew)),
		statusAwaitingDecision: lipgloss.NewStyle().Foreground(lipgloss.Color(statusAccent)),
		statusAwaitingInput:    lipgloss.NewStyle().Foreground(lipgloss.Color(faint)),
		statusIdle:             lipgloss.NewStyle().Foreground(lipgloss.Color(faint)),
		statusError:            lipgloss.NewStyle().Foreground(lipgloss.Color(errColor)),
		selBg:                  selBG,
		selText:                selText,
	}
}

func (t pickerTheme) frameTop(title, right string, width int) string {
	return t.frameBar('╭', '╮', t.title.Render(title), right, width)
}

func (t pickerTheme) frameBottom(hints string, width int) string {
	return t.frameBar('╰', '╯', hints, "", width)
}

const (
	frameChromeWidth   = 4
	barChromeTwoLabels = 8
	barChromeOneLabel  = 6
)

func (t pickerTheme) frameBar(left, right rune, leftLabel, rightLabel string, width int) string {
	if rightLabel != "" {
		budget := max(0, width-barChromeTwoLabels)
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

	budget := max(0, width-barChromeOneLabel)
	leftLabel = clampWidth(leftLabel, budget)

	var buf strings.Builder

	fill := max(0, budget-displayWidth(leftLabel))
	buf.WriteString(t.border.Render(string(left) + "─ "))
	buf.WriteString(leftLabel)
	buf.WriteString(t.border.Render(" " + strings.Repeat("─", fill) + "─" + string(right)))

	return buf.String()
}

func (t pickerTheme) frameLine(content string, width int) string {
	inner := max(0, width-frameChromeWidth)
	content = clampWidth(content, inner)
	pad := max(0, inner-displayWidth(content))

	side := t.border.Render("│")

	return side + " " + content + strings.Repeat(" ", pad) + " " + side
}
