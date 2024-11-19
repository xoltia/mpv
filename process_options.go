package mpv

import (
	"io"
	"runtime"
	"time"
)

type ProcessOptions struct {
	Path   string
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Maximum number of connection retries before giving up.
	// Retry delay doubles with each attempt.
	ConnMaxRetries int
	ConnRetryDelay time.Duration

	ClientOptions ClientOptions
}

func (o *ProcessOptions) applyDefaults() {
	if o.Path == "" {
		switch runtime.GOOS {
		case "windows":
			o.Path = "mpv.exe"
		default:
			o.Path = "mpv"
		}
	}
	if o.ConnMaxRetries == 0 {
		o.ConnMaxRetries = 5
	}
	if o.ConnRetryDelay == 0 {
		o.ConnRetryDelay = 100 * time.Millisecond
	}
	o.ClientOptions.applyDefaults()
}
