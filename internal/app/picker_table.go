package app

import (
	"fmt"
	"sort"
	"strings"
)

type pickerColumnID string

const (
	pickerColItem     pickerColumnID = "item"
	pickerColCmd      pickerColumnID = "cmd"
	pickerColCaptured pickerColumnID = "captured"
	pickerColWins     pickerColumnID = "wins"
	pickerColState    pickerColumnID = "state"
)

type pickerColumnSpec struct {
	ID         pickerColumnID
	Title      string
	MinWidth   int
	TargetWide int
	Priority   int
	Required   bool
	AlignRight bool
	Flex       bool
	Value      func(pickerRow) string
}

type pickerColumnLayout struct {
	spec  pickerColumnSpec
	width int
}

type pickerTableLayout struct {
	columns []pickerColumnLayout
}

// Column behavior lives in one place: tune Priority/MinWidth/TargetWide here.
var pickerColumnSpecs = []pickerColumnSpec{
	{
		ID:         pickerColItem,
		Title:      "ITEM",
		MinWidth:   16,
		TargetWide: 28,
		Priority:   0,
		Required:   true,
		Flex:       true,
		Value: func(r pickerRow) string {
			return r.item
		},
	},
	{
		ID:         pickerColCmd,
		Title:      "CMD",
		MinWidth:   12,
		TargetWide: 24,
		Priority:   1,
		Flex:       true,
		Value: func(r pickerRow) string {
			return r.cmd
		},
	},
	{
		ID:         pickerColCaptured,
		Title:      "CAPTURED",
		MinWidth:   19,
		TargetWide: 19,
		Priority:   2,
		Value: func(r pickerRow) string {
			return r.captured
		},
	},
	{
		ID:         pickerColWins,
		Title:      "WINS",
		MinWidth:   4,
		TargetWide: 4,
		Priority:   3,
		AlignRight: true,
		Value: func(r pickerRow) string {
			return r.wins
		},
	},
	{
		ID:         pickerColState,
		Title:      "STATE",
		MinWidth:   5,
		TargetWide: 5,
		Priority:   4,
		Value: func(r pickerRow) string {
			return r.state
		},
	},
}

func buildPickerTableLayout(totalWidth int) pickerTableLayout {
	if totalWidth <= 0 {
		totalWidth = 1
	}

	required := make([]pickerColumnSpec, 0, len(pickerColumnSpecs))
	optional := make([]pickerColumnSpec, 0, len(pickerColumnSpecs))
	for _, spec := range pickerColumnSpecs {
		if spec.Required {
			required = append(required, spec)
			continue
		}
		optional = append(optional, spec)
	}
	sort.Slice(optional, func(i, j int) bool { return optional[i].Priority < optional[j].Priority })

	active := append([]pickerColumnSpec{}, required...)
	for _, spec := range optional {
		candidate := append(active, spec)
		if minTableWidth(candidate) <= totalWidth {
			active = candidate
		}
	}

	columns := make([]pickerColumnLayout, 0, len(active))
	for _, spec := range active {
		columns = append(columns, pickerColumnLayout{spec: spec, width: spec.MinWidth})
	}
	extra := totalWidth - tableWidth(columns)
	if extra < 0 {
		shrinkColumnsToFit(columns, totalWidth)
		extra = totalWidth - tableWidth(columns)
	}
	for i := range columns {
		if extra <= 0 {
			break
		}
		need := columns[i].spec.TargetWide - columns[i].width
		if need <= 0 {
			continue
		}
		add := min(extra, need)
		columns[i].width += add
		extra -= add
	}

	if extra > 0 {
		for i := range columns {
			if columns[i].spec.Flex {
				columns[i].width += extra
				break
			}
		}
	}

	return pickerTableLayout{columns: columns}
}

func shrinkColumnsToFit(columns []pickerColumnLayout, totalWidth int) {
	if len(columns) == 0 {
		return
	}
	for {
		current := tableWidth(columns)
		if current <= totalWidth {
			return
		}

		target := widestShrinkableColumn(columns)
		if target < 0 {
			return
		}
		columns[target].width--
	}
}

func tableWidth(columns []pickerColumnLayout) int {
	if len(columns) == 0 {
		return 0
	}
	width := len(columns) - 1
	for _, col := range columns {
		width += col.width
	}
	return width
}

func widestShrinkableColumn(columns []pickerColumnLayout) int {
	best := -1
	bestWidth := 1
	for i := range columns {
		if columns[i].width > bestWidth {
			best = i
			bestWidth = columns[i].width
		}
	}
	return best
}

func minTableWidth(specs []pickerColumnSpec) int {
	if len(specs) == 0 {
		return 0
	}
	width := len(specs) - 1
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
	parts := make([]string, 0, len(l.columns))
	for _, col := range l.columns {
		parts = append(parts, alignAndTrim(valueFor(col.spec), col.width, col.spec.AlignRight))
	}
	return strings.Join(parts, " ")
}

func alignAndTrim(v string, width int, right bool) string {
	if width <= 0 {
		return ""
	}
	cell := trim(v, width)
	if right {
		return fmt.Sprintf("%*s", width, cell)
	}
	return fmt.Sprintf("%-*s", width, cell)
}
