package main

import (
	"os"
	"os/signal"

	"github.com/xoltia/mpv"
)

func main() {
	m := mpv.NewProcessWithOptions(mpv.ProcessOptions{
		Args: []string{"--force-window"},
	})
	defer m.Close()

	c, err := m.OpenClient()
	if err != nil {
		panic(err)
	}
	defer c.Close()

	err = c.LoadFile("https://www.youtube.com/watch?v=6BfKzQzBe7M", mpv.LoadFileModeReplace)
	if err != nil {
		panic(err)
	}

	err = c.Play()
	if err != nil {
		panic(err)
	}

	exitChan := make(chan os.Signal, 1)
	_, err = c.ObserveProperty("idle-active", func(v any) {
		if v.(bool) {
			close(exitChan)
		}
	})
	if err != nil {
		panic(err)
	}

	go func() {
		m.Wait()
		close(exitChan)
	}()

	signal.Notify(exitChan, os.Interrupt)
	<-exitChan
}
