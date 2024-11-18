//go:build windows

package mpv

import (
	"time"

	"github.com/natefinch/npipe"
)

var defaultSocketPath = `\\.\pipe\mpvsocket`

func openIPC(socketPath string, timeout time.Duration) (*ipc, error) {
	conn, err := npipe.DialTimeout(socketPath, timeout)
	if err != nil {
		return nil, err
	}

	ipc := newIPC(conn)
	ipc.init()
	return ipc, nil
}
