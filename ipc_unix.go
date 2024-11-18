//go:build unix

package mpv

import (
	"net"
	"time"
)

var defaultSocketPath = "/tmp/mpvsocket"

func openIPC(socketPath string, timeout time.Duration) (*ipc, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, err
	}

	ipc := newIPC(conn)
	ipc.init()
	return ipc, nil
}
