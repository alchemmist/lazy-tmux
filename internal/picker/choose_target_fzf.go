//go:build lazy_fzf

package picker

import "fmt"

func ChooseTarget(_ []Session, _ []WindowSortKey, _ Actions) (Target, error) {
	return Target{}, fmt.Errorf("TUI picker disabled in fzf-only build")
}
