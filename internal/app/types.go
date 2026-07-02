package app

import "github.com/alchemmist/lazy-tmux/internal/picker"

// PickerTarget aliases picker.Target (a session plus optional window index) so
// CLI code does not import picker directly.
type PickerTarget = picker.Target
