//go:build lazy_fzf

package picker

func ChooseQuickSession([]QuickSession, string) (string, error) {
	return "", errTUIDisabled
}

func ChooseTargetWithLoader([]WindowSortKey, Actions, string) (Target, error) {
	return Target{}, errTUIDisabled
}

func tuiDisabled() bool {
	return true
}
