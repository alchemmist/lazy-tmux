//go:build lazy_fzf

package picker

func RunRestoreAnimation(_ string, _ <-chan struct{}) (bool, error) {
	return false, nil
}

func RunRestoreAnimationWithTheme(sessionName string, done <-chan struct{}, _ string) (bool, error) {
	return RunRestoreAnimation(sessionName, done)
}
