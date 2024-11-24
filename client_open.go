package mpv

import (
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"sync/atomic"
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

// DefaultSocketPath returns the default socket path used by OpenClient.
func DefaultSocketPath() string {
	return defaultSocketPath
}

var inc = atomic.Int32{}

// IncrementingSocketPath returns a new socket path based on the default socket path
// with an incrementing number appended to it.
func IncrementingSocketPath() string {
	return fmt.Sprintf("%s-%d", defaultSocketPath, inc.Add(1))
}

// RandomSocketPath returns a new socket path based on the default socket path
// with a random 16-character hexadecimal string appended to it. May produce
// collisions.
func RandomSocketPath() string {
	b := [8]byte{}
	n := rand.Uint64()
	for i := 0; i < 8; i++ {
		b[i] = byte(n)
		n >>= 8
	}
	randomString := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s", defaultSocketPath, randomString)
}

// IncrementingPIDSocketPath returns a new socket path based on the default socket path
// with an incrementing number and the process ID appended to it.
func IncrementingPIDSocketPath() string {
	return fmt.Sprintf("%s-%d-%d", defaultSocketPath, os.Getpid(), inc.Add(1))
}
