package picker

import (
	"os"
	"testing"
)

func TestIsTerminalReturnsFalseForPipe(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = read.Close() }()
	defer func() { _ = write.Close() }()

	if isTerminal(read) {
		t.Fatal("expected pipe to not be a terminal")
	}

	if isTerminal(write) {
		t.Fatal("expected pipe to not be a terminal")
	}
}
