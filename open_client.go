package mpv

import (
	"fmt"
	"time"
)

type openClientOptions struct {
	socketPath  string
	dialTimeout time.Duration
}

func (o *openClientOptions) applyDefaults() {
	if o.socketPath == "" {
		o.socketPath = defaultSocketPath
	}
	if o.dialTimeout == 0 {
		o.dialTimeout = 5 * time.Second
	}
}

type OpenClientOption func(*openClientOptions)

func WithSocketPath(socketPath string) OpenClientOption {
	return func(o *openClientOptions) {
		o.socketPath = socketPath
	}
}

func WithDialTimeout(timeout time.Duration) OpenClientOption {
	return func(o *openClientOptions) {
		o.dialTimeout = timeout
	}
}

func OpenClient(options ...OpenClientOption) (*MPVClient, error) {
	var opts openClientOptions
	for _, o := range options {
		o(&opts)
	}
	opts.applyDefaults()

	ipc, err := openIPC(opts.socketPath, opts.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to open IPC: %w", err)
	}

	client := &MPVClient{ipc: ipc}
	go client.acceptEvents()
	return client, nil
}
