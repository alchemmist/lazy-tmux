//go:build !lazy_fzf

package picker

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

type pickerColumnID string

// pickerColumnGap is the blank gutter rendered between adjacent columns so they
// stay visually separated even when each is at its minimum width.
const pickerColumnGap = 2

type pickerColumnSpec struct {
	ID         pickerColumnID
	Title      string
	MinWidth   int
	Priority   int
	Required   bool
	Grow       bool // absorbs leftover width; non-growing columns stay at MinWidth
	Value      func(pickerRow) string
	TrimPrefix string
}

type pickerColumnLayout struct {
	spec  pickerColumnSpec
	width int
}

type pickerTableLayout struct {
	columns []pickerColumnLayout
}

var pickerColumnSpecs = []pickerColumnSpec{
	{
		ID:       "item",
		Title:    "Session / Window",
		MinWidth: 10,
		Priority: 0,
		Required: true,
		Grow:     true,
		Value: func(r pickerRow) string {
			return r.item
		},
	},
	{
		ID:       "cmd",
		Title:    "Cmd",
		MinWidth: 14,
		Priority: 1,
		Grow:     true,
		Value: func(r pickerRow) string {
			return r.cmd
		},
	},
	{
		ID:       "captured",
		Title:    "Captured",
		MinWidth: 10,
		Priority: 2,
		Value: func(r pickerRow) string {
			return r.captured
		},
	},
	{
		ID:       "wins",
		Title:    "Wins",
		MinWidth: 4,
		Priority: 3,
		Value: func(r pickerRow) string {
			return r.wins
		},
	},
	{
		ID:       "state",
		Title:    "State",
		MinWidth: 5,
		Priority: 4,
		Value: func(r pickerRow) string {
			return r.state
		},
	},
}

func buildPickerTableLayout(totalWidth int) pickerTableLayout {
	required := make([]pickerColumnSpec, 0, len(pickerColumnSpecs))
	optional := make([]pickerColumnSpec, 0, len(pickerColumnSpecs))

	for _, spec := range pickerColumnSpecs {
		if spec.Required {
			required = append(required, spec)
		} else {
			optional = append(optional, spec)
		}
	}

	sort.Slice(optional, func(i, j int) bool {
		if optional[i].Priority == optional[j].Priority {
			return optional[i].ID < optional[j].ID
		}

		return optional[i].Priority < optional[j].Priority
	})

	active := append([]pickerColumnSpec{}, required...)

	activeSet := make(map[pickerColumnID]struct{}, len(pickerColumnSpecs))
	for _, spec := range required {
		activeSet[spec.ID] = struct{}{}
	}

	for i := range optional {
		candidate := append(active, optional[i])
		if minTableWidth(candidate) <= totalWidth {
			active = candidate
			activeSet[optional[i].ID] = struct{}{}
		} else {
			break
		}
	}

	columns := make([]pickerColumnLayout, 0, len(active))

	for _, spec := range pickerColumnSpecs {
		if _, ok := activeSet[spec.ID]; !ok {
			continue
		}

		columns = append(columns, pickerColumnLayout{spec: spec, width: spec.MinWidth})
	}

	extra := totalWidth - minTableWidth(active)
	if extra < 0 {
		shrinkColumnsToFit(columns, totalWidth)
		return pickerTableLayout{columns: columns}
	}

	if extra > 0 {
		growColumns(columns, extra)
	}

	return pickerTableLayout{columns: columns}
}

// growColumns hands the leftover width to the growable columns only (the name
// and command), keeping the narrow meta columns at their minimum width. If no
// column is growable it falls back to spreading the width across all of them.
func growColumns(columns []pickerColumnLayout, extra int) {
	growers := make([]int, 0, len(columns))

	for i := range columns {
		if columns[i].spec.Grow {
			growers = append(growers, i)
		}
	}

	if len(growers) == 0 {
		for i := range columns {
			growers = append(growers, i)
		}
	}

	share := extra / len(growers)
	for _, i := range growers {
		columns[i].width += share
	}

	for k := 0; k < extra%len(growers); k++ {
		columns[growers[k]].width++
	}
}

func shrinkColumnsToFit(columns []pickerColumnLayout, totalWidth int) {
	if len(columns) == 0 {
		return
	}

	for tableWidth(columns) > totalWidth {
		idx := widestShrinkableColumn(columns)
		if idx < 0 {
			return
		}

		columns[idx].width--
	}
}

func tableWidth(columns []pickerColumnLayout) int {
	width := 0
	for _, col := range columns {
		width += col.width
	}

	return width + gapWidth(len(columns))
}

// gapWidth is the total width taken by the gutters between n columns.
func gapWidth(columns int) int {
	if columns <= 1 {
		return 0
	}

	return pickerColumnGap * (columns - 1)
}

func widestShrinkableColumn(columns []pickerColumnLayout) int {
	best := -1
	bestWidth := 0

	for idx, col := range columns {
		if col.width <= 1 {
			continue
		}

		if col.width > bestWidth {
			bestWidth = col.width
			best = idx
		}
	}

	return best
}

func minTableWidth(specs []pickerColumnSpec) int {
	width := 0
	for _, spec := range specs {
		width += spec.MinWidth
	}

	return width + gapWidth(len(specs))
}

func (l pickerTableLayout) header() string {
	return l.render(func(spec pickerColumnSpec) string { return spec.Title })
}

func (l pickerTableLayout) row(row pickerRow) string {
	return l.render(func(spec pickerColumnSpec) string { return spec.Value(row) })
}

// styledHeader renders the column titles in the faint header style.
func (l pickerTableLayout) styledHeader(theme pickerTheme) string {
	return l.renderWith(func(spec pickerColumnSpec) (string, lipgloss.Style) {
		return spec.Title, theme.headerCell
	})
}

// styledRow renders a row with the name column bright (bold for session
// headers) and the meta columns dimmed.
func (l pickerTableLayout) styledRow(row pickerRow, theme pickerTheme) string {
	return l.renderWith(func(spec pickerColumnSpec) (string, lipgloss.Style) {
		if spec.ID == "item" {
			if row.selectable {
				return spec.Value(row), theme.name
			}

			return spec.Value(row), theme.session
		}

		return spec.Value(row), theme.meta
	})
}

func (l pickerTableLayout) renderWith(
	cell func(spec pickerColumnSpec) (string, lipgloss.Style),
) string {
	var out strings.Builder

	for idx, col := range l.columns {
		val, style := cell(col.spec)
		if col.spec.TrimPrefix != "" {
			val = strings.TrimPrefix(val, col.spec.TrimPrefix)
		}

		val = truncateString(val, col.width)

		pad := col.width - displayWidth(val)
		if idx != len(l.columns)-1 && pad > 0 {
			val += strings.Repeat(" ", pad)
		}

		out.WriteString(style.Render(val))

		if idx != len(l.columns)-1 {
			out.WriteString(strings.Repeat(" ", pickerColumnGap))
		}
	}

	return out.String()
}

func (l pickerTableLayout) render(valueFor func(spec pickerColumnSpec) string) string {
	var out strings.Builder

	for idx, col := range l.columns {
		val := valueFor(col.spec)
		if col.spec.TrimPrefix != "" {
			val = strings.TrimPrefix(val, col.spec.TrimPrefix)
		}

		val = truncateString(val, col.width)
		if idx == len(l.columns)-1 {
			out.WriteString(val)
			continue
		}

		out.WriteString(val)

		pad := col.width - displayWidth(val)
		if pad > 0 {
			out.WriteString(strings.Repeat(" ", pad))
		}

		out.WriteString(strings.Repeat(" ", pickerColumnGap))
	}

	return out.String()
}
