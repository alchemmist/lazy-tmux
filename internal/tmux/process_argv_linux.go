package tmux

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

func processArgv(pid int) ([]string, error) {
	root, err := os.OpenRoot("/proc")
	if err != nil {
		return nil, fmt.Errorf("open process directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, fmt.Errorf("read process arguments: %w", err)
	}
	fields := bytes.Split(bytes.TrimSuffix(data, []byte{0}), []byte{0})
	argv := make([]string, len(fields))
	for idx, field := range fields {
		argv[idx] = string(field)
	}
	return argv, nil
}
