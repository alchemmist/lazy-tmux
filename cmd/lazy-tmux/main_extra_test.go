package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, action func()) string {
	t.Helper()

	old := os.Stdout

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdout = write
	defer func() { os.Stdout = old }()
	defer write.Close()

	defer func() {
		if rec := recover(); rec != nil {
			panic(rec)
		}
	}()

	action()
	write.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, read); err != nil {
		t.Fatalf("copy: %v", err)
	}

	read.Close()

	return buf.String()
}

func TestMainUsesExitFunc(t *testing.T) {
	origExit := exitFunc
	exitCode := -1
	exitFunc = func(code int) {
		exitCode = code
	}

	defer func() { exitFunc = origExit }()

	origArgs := os.Args
	os.Args = []string{"lazy-tmux", "help"}

	defer func() { os.Args = origArgs }()
	main()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestFatalErrNotFound(t *testing.T) {
	origOut := fatalOutput
	origExit := exitFunc

	var buf bytes.Buffer
	fatalOutput = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		fatalOutput = origOut
		exitFunc = origExit
	}()
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit code 1, got %v", r)
		}
	}()
	fatalErr(os.ErrNotExist)
}

func TestFatalErrGeneric(t *testing.T) {
	origOut := fatalOutput
	origExit := exitFunc

	var buf bytes.Buffer
	fatalOutput = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		fatalOutput = origOut
		exitFunc = origExit
	}()
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit code 1, got %v", r)
		}
	}()
	fatalErr(errors.New("boom"))

	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("expected error in output, got %q", buf.String())
	}
}

func TestUsageAndSetupPrint(t *testing.T) {
	usageOutput := captureStdout(t, usage)
	if !strings.Contains(usageOutput, "lazy-tmux - tmux session snapshots with lazy restore") {
		t.Fatalf("unexpected usage output: %s", usageOutput)
	}

	setupOutput := captureStdout(t, setupConfig)
	if !strings.Contains(setupOutput, "lazy-tmux daemon") {
		t.Fatalf("unexpected setup output: %s", setupOutput)
	}
}
