package mpv

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Process represents an mpv process and provides a convenient way to manage
// multiple clients that communicate with the same process.
type Process struct {
	Path string
	Args []string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	mu      sync.Mutex
	cmd     *exec.Cmd
	clients []*Client
}

func NewProcess() *Process {
	return &Process{
		Path: "mpv",
		Args: []string{},
	}
}

// OpenClient starts a new mpv process and returns a new Client instance.
// The client is only valid for as long as the process is running.
func (p *Process) OpenClient() (*Client, error) {
	if err := p.startProcess(); err != nil {
		return nil, err
	}

	client, err := OpenClient()
	if err != nil {
		return nil, err
	}

	p.clients = append(p.clients, client)
	return client, nil
}

func (p *Process) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil {
		return nil
	}

	for _, c := range p.clients {
		c.Close()
	}
	p.clients = nil

	err := p.cmd.Process.Kill()
	p.cmd = nil
	return err
}

func (p *Process) Wait() error {
	if p.cmd == nil {
		return errors.New("process is not started")
	}

	return p.cmd.Wait()
}

func (p *Process) startProcess() error {
	if p.cmd != nil {
		return nil
	}

	defaultArgs := []string{
		fmt.Sprintf("--input-ipc-server=%s", defaultSocketPath),
		"--idle",
	}
	args := append(defaultArgs, p.Args...)
	cmd := exec.Command(p.Path, args...)
	cmd.Stdin = p.Stdin
	cmd.Stdout = p.Stdout
	cmd.Stderr = p.Stderr
	go func() {
		cmd.Wait()
		p.Close()
	}()

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd
	return nil
}
