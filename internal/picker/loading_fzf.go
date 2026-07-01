//go:build lazy_fzf

package picker

// RunRestoreAnimation is a no-op in the fzf-only build (no TUI to draw into);
// the caller waits on done itself.
func RunRestoreAnimation(_ string, _ <-chan struct{}) error {
	return nil
}
