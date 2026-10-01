package tmux

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

func processArgv(pid int) ([]string, error) {
	data, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, fmt.Errorf("read process arguments: %w", err)
	}
	const headerSize = 4
	if len(data) < headerSize {
		return nil, errHookOwner
	}
	count := int(binary.LittleEndian.Uint32(data[:headerSize]))
	data = data[headerSize:]
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return nil, errHookOwner
	}
	data = data[end+1:]
	data = bytes.TrimLeft(data, "\x00")
	argv := make([]string, 0, count)
	for len(argv) < count {
		end = bytes.IndexByte(data, 0)
		if end < 0 {
			return nil, errHookOwner
		}
		argv = append(argv, string(data[:end]))
		data = data[end+1:]
	}

	return argv, nil
}
