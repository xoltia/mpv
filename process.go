package mpv

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
)

type Process struct {
	Path string
	Args []string

	cmd    *exec.Cmd
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
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
	return client, nil
}

func (p *Process) Close() error {
	if p.cmd == nil {
		return nil
	}

	return p.cmd.Process.Kill()
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

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd
	return nil
}
