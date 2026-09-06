//go:build lazy_fzf

package picker

func ChooseQuickSession([]QuickSession, string, []string, int, func() map[string]bool) (string, error) {
	return "", errTUIDisabled
}

func ChooseTargetWithLoader([]WindowSortKey, Actions, string) (Target, error) {
	return Target{}, errTUIDisabled
}

func tuiDisabled() bool {
	return true
}
