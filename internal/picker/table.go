//go:build !lazy_fzf

package picker

import (
	"sort"
	"strings"
)

type pickerColumnID string

type pickerColumnSpec struct {
	ID         pickerColumnID
	Title      string
	MinWidth   int
	Priority   int
	Required   bool
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
		Value: func(r pickerRow) string {
			return r.item
		},
	},
	{
		ID:       "cmd",
		Title:    "Cmd",
		MinWidth: 12,
		Priority: 1,
		Value: func(r pickerRow) string {
			return r.cmd
		},
	},
	{
		ID:         "captured",
		Title:      "Captured",
		MinWidth:   16,
		Priority:   2,
		TrimPrefix: "202",
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
		for i := range columns {
			columns[i].width += extra / len(columns)
		}
		for i := 0; i < extra%len(columns); i++ {
			columns[i].width++
		}
	}

	return pickerTableLayout{columns: columns}
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
	return width
}

func widestShrinkableColumn(columns []pickerColumnLayout) int {
	best := -1
	bestWidth := 0
	for i, col := range columns {
		if col.width <= 1 {
			continue
		}
		if col.width > bestWidth {
			bestWidth = col.width
			best = i
		}
	}
	return best
}

func minTableWidth(specs []pickerColumnSpec) int {
	width := 0
	for _, spec := range specs {
		width += spec.MinWidth
	}
	return width
}

func (l pickerTableLayout) header() string {
	return l.render(func(spec pickerColumnSpec) string { return spec.Title })
}

func (l pickerTableLayout) row(row pickerRow) string {
	return l.render(func(spec pickerColumnSpec) string { return spec.Value(row) })
}

func (l pickerTableLayout) render(valueFor func(spec pickerColumnSpec) string) string {
	var out strings.Builder
	for i, col := range l.columns {
		val := valueFor(col.spec)
		if col.spec.TrimPrefix != "" {
			val = strings.TrimPrefix(val, col.spec.TrimPrefix)
		}
		val = trim(val, col.width)
		if i == len(l.columns)-1 {
			out.WriteString(val)
			continue
		}
		out.WriteString(val)
		pad := col.width - len([]rune(val))
		if pad > 0 {
			out.WriteString(strings.Repeat(" ", pad))
		}
	}
	return out.String()
}
