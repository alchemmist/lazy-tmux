package app

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestNextWindowIndex(t *testing.T) {
	tests := []struct {
		windows []snapshot.Window
		want    int
	}{
		{
			windows: []snapshot.Window{
				{Index: 0},
				{Index: 1},
				{Index: 2},
			},
			want: 3,
		},
		{
			windows: []snapshot.Window{
				{Index: 0},
				{Index: 5},
				{Index: 2},
			},
			want: 6,
		},
		{
			windows: []snapshot.Window{},
			want:    0,
		},
		{
			windows: []snapshot.Window{
				{Index: 10},
			},
			want: 11,
		},
	}

	for _, tt := range tests {
		got := nextWindowIndex(tt.windows)
		if got != tt.want {
			t.Fatalf("nextWindowIndex(%v) = %d, want %d", tt.windows, got, tt.want)
		}
	}
}

func TestIsShellCommandName(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{cmd: "bash", want: true},
		{cmd: "-bash", want: true},
		{cmd: "/bin/bash", want: true},
		{cmd: "/bin/bash -l", want: true},
		{cmd: "zsh", want: true},
		{cmd: "fish", want: true},
		{cmd: "sh", want: true},
		{cmd: "ksh", want: true},
		{cmd: "nvim", want: false},
		{cmd: "vim", want: false},
		{cmd: "docker", want: false},
		{cmd: "", want: false},
		{cmd: "   ", want: false},
	}

	for _, tt := range tests {
		if got := isShellCommandName(tt.cmd); got != tt.want {
			t.Fatalf("isShellCommandName(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}
