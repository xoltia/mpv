package mpv

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	ErrProcessClosed = errors.New("process closed")
)

// Process represents an mpv process and provides a convenient way to manage
// multiple clients that communicate with the same process.
type Process struct {
	opts       ProcessOptions
	mu         sync.Mutex
	cmd        *exec.Cmd
	clients    []*Client
	closed     bool
	closeErr   error
	closedCond *sync.Cond
}

func NewProcess() *Process {
	return NewProcessWithOptions(ProcessOptions{})
}

func NewProcessWithOptions(opts ProcessOptions) *Process {
	opts.applyDefaults()

	p := &Process{
		opts:   opts,
		closed: false,
	}
	p.closedCond = sync.NewCond(&p.mu)
	return p
}

// OpenClient starts a new mpv process if one has not already been started and
// returns a new Client instance to communicate with it. The client is only
// valid for as long as the process is running.
func (p *Process) OpenClient() (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.startProcess(); err != nil {
		return nil, err
	}

	attempts := 0
tryConnect:
	client, err := OpenClientWithOptions(p.opts.ClientOptions)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) && attempts < p.opts.ConnMaxRetries {
			attempts++
			time.Sleep(p.opts.ConnRetryDelay * time.Duration(attempts<<1))
			goto tryConnect
		}
		return nil, err
	}

	p.clients = append(p.clients, client)

	return client, nil
}

func (p *Process) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	return p.close()
}

func (p *Process) Wait() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for !p.closed {
		p.closedCond.Wait()
	}

	return p.closeErr
}

func (p *Process) close() error {
	errs := []error{}
	for _, c := range p.clients {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	p.clients = nil

	if p.cmd != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			errs = append(errs, err)
		}
		p.cmd = nil
	}

	p.closed = true
	return errors.Join(errs...)
}

func (p *Process) startProcess() error {
	if p.closed {
		if p.closeErr != nil {
			return fmt.Errorf("%w: %v", ErrProcessClosed, p.closeErr)
		} else {
			return ErrProcessClosed
		}
	}

	if p.cmd != nil {
		return nil
	}

	defaultArgs := []string{
		fmt.Sprintf("--input-ipc-server=%s", p.opts.ClientOptions.SocketPath),
		"--idle",
	}
	args := append(defaultArgs, p.opts.Args...)
	cmd := exec.Command(p.opts.Path, args...)
	cmd.Stdin = p.opts.Stdin
	cmd.Stdout = p.opts.Stdout
	cmd.Stderr = p.opts.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd

	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()
		err2 := p.close()
		if err2 != nil {
			err = errors.Join(err, err2)
		}
		p.closeErr = err
		p.cmd = nil
		p.closedCond.Broadcast()
	}()

	return nil
}
