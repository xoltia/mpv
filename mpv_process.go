package mpv

import (
	"fmt"
	"io"
	"os/exec"
)

type MPVProcess struct {
	Path string
	Args []string

	cmd    *exec.Cmd
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func NewMPVProcess() *MPVProcess {
	return &MPVProcess{
		Path: "mpv",
		Args: []string{},
	}
}

func (p *MPVProcess) OpenClient() (*MPVClient, error) {
	if err := p.startProcess(); err != nil {
		return nil, err
	}

	client, err := OpenClient()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (p *MPVProcess) Close() error {
	if p.cmd == nil {
		return nil
	}

	return p.cmd.Process.Kill()
}

func (p *MPVProcess) startProcess() error {
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
