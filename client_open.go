package mpv

import (
	"fmt"
	"time"
)

type ClientOptions struct {
	SocketPath  string
	DialTimeout time.Duration
}

func (o *ClientOptions) applyDefaults() {
	if o.SocketPath == "" {
		o.SocketPath = defaultSocketPath
	}
	if o.DialTimeout == 0 {
		o.DialTimeout = 5 * time.Second
	}
}

func OpenClient() (*Client, error) {
	return OpenClientWithOptions(ClientOptions{})
}

func OpenClientWithOptions(opts ClientOptions) (*Client, error) {
	opts.applyDefaults()

	ipc, err := openIPC(opts.SocketPath, opts.DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to open IPC: %w", err)
	}

	client := &Client{ipc: ipc}
	go client.acceptEvents()
	return client, nil
}
