//go:build linux

package tmux

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseProcessArgvRejectsEmptyCommandLine(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{nil, {}, {0}, {0, 0}} {
		argv, err := parseProcessArgv(data)
		if !errors.Is(err, errHookOwner) || argv != nil {
			t.Fatalf("data=%q argv=%q err=%v", data, argv, err)
		}
	}
}

func TestParseProcessArgvPreservesArgumentBoundaries(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		[]byte("claude\x00--append-system-prompt\x00\x00path with spaces\x00"),
		[]byte("claude\x00--append-system-prompt\x00\x00path with spaces"),
	} {
		argv, err := parseProcessArgv(data)
		want := []string{"claude", "--append-system-prompt", "", "path with spaces"}
		if err != nil || !reflect.DeepEqual(argv, want) {
			t.Fatalf("argv=%q err=%v want=%q", argv, err, want)
		}
	}
}
