//go:build lazy_fzf

package picker

func RunRestoreAnimation(_ string, _ <-chan struct{}) (bool, error) {
	return false, nil
}
