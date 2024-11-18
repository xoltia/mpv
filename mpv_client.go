package mpv

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type (
	LoadFileMode string
	SeekFlag     string
)

const (
	LoadFileModeReplace    LoadFileMode = "replace"
	LoadFileModeAppend     LoadFileMode = "append"
	LoadFileModeAppendPlay LoadFileMode = "append-play"
)

const (
	SeekFlagRelative        SeekFlag = "relative"
	SeekFlagAbsolute        SeekFlag = "absolute"
	SeekFlagExact           SeekFlag = "exact"
	SeekFlagKeyframes       SeekFlag = "keyframes"
	SeekFlagRelativePercent SeekFlag = "relative-percent"
	SeekFlagAbsolutePercent SeekFlag = "absolute-percent"
)

type eventHandler struct {
	sync bool
	fn   func(map[string]any)
}

type mpvClient struct {
	ipc             *ipc
	eventHandlersMu sync.Mutex
	eventHandlers   []*eventHandler
	observerID      atomic.Int64
}

func (c *mpvClient) Close() error {
	return c.ipc.close()
}

func (c *mpvClient) Play() error {
	_, err := c.Command("set_property", "pause", false)
	return err
}

func (c *mpvClient) Pause() error {
	_, err := c.Command("set_property", "pause", true)
	return err
}

func (c *mpvClient) Seek(position float64, flags ...SeekFlag) error {
	var err error
	if len(flags) == 0 {
		_, err = c.Command("seek", position)
	} else {
		flag := strings.Builder{}
		for i, f := range flags {
			if i > 0 {
				flag.WriteRune('+')
			}
			flag.WriteString(string(f))
		}
		_, err = c.Command("seek", position, flag)
	}

	return err
}

func (c *mpvClient) LoadFile(file string, mode LoadFileMode) error {
	_, err := c.Command("loadfile", file, string(mode))
	return err
}

// Property setters

func (c *mpvClient) SetProperty(property string, value any) error {
	_, err := c.Command("set_property", property, value)
	return err
}

func (c *mpvClient) SetVolume(volume float64) error {
	return c.SetProperty("volume", volume)
}

func (c *mpvClient) SetMute(mute bool) error {
	return c.SetProperty("mute", mute)
}

func (c *mpvClient) SetLoop(loop bool) error {
	return c.SetProperty("loop", loop)
}

func (c *mpvClient) SetSpeed(speed float64) error {
	return c.SetProperty("speed", speed)
}

func (c *mpvClient) SetPosition(position float64) error {
	return c.SetProperty("time-pos", position)
}

// Property getters

func (c *mpvClient) GetProperty(property string) (any, error) {
	return c.Command("get_property", property)
}

func (c *mpvClient) GetPaused() (bool, error) {
	return c.GetPropertyBool("pause")
}

func (c *mpvClient) GetDuration() (float64, error) {
	return c.GetPropertyFloat("duration")
}

func (c *mpvClient) GetPosition() (float64, error) {
	return c.GetPropertyFloat("time-pos")
}

func (c *mpvClient) GetVolume() (float64, error) {
	return c.GetPropertyFloat("volume")
}

func (c *mpvClient) GetMute() (bool, error) {
	return c.GetPropertyBool("mute")
}

func (c *mpvClient) GetFilename() (string, error) {
	return c.GetPropertyString("filename")
}

func (c *mpvClient) GetSpeed() (float64, error) {
	return c.GetPropertyFloat("speed")
}

func (c *mpvClient) GetIdleActive() (bool, error) {
	return c.GetPropertyBool("idle-active")
}

func (c *mpvClient) GetLoop() (bool, error) {
	return c.GetPropertyBool("loop")
}

func (c *mpvClient) GetPropertyBool(property string) (b bool, err error) {
	value, err := c.GetProperty(property)
	if err != nil {
		return
	}
	b, ok := value.(bool)
	if !ok {
		err = fmt.Errorf("mpv: property is not a bool: %v", value)
		return
	}
	return
}

func (c *mpvClient) GetPropertyFloat(property string) (f float64, err error) {
	value, err := c.GetProperty(property)
	if err != nil {
		return
	}
	f, ok := value.(float64)
	if !ok {
		err = fmt.Errorf("mpv: property is not a float: %v", value)
		return
	}
	return
}

func (c *mpvClient) GetPropertyString(property string) (s string, err error) {
	value, err := c.GetProperty(property)
	if err != nil {
		return
	}
	s, ok := value.(string)
	if !ok {
		err = fmt.Errorf("mpv: property is not a string: %v", value)
		return
	}
	return
}

func (c *mpvClient) ObserveProperty(property string, fn func(any)) (rm func() error, err error) {
	observerID := c.observerID.Add(1)
	rmEventHandler := c.AddEventHandler(func(event map[string]any) {
		if event["event"] == "property-change" {
			id, ok := event["id"]
			if !ok {
				return
			}
			if int64(id.(float64)) != observerID {
				return
			}
			fn(event["data"])
		}
	})

	if _, err = c.CommandAsync("observe_property", observerID, property); err != nil {
		rmEventHandler()
		return nil, fmt.Errorf("failed to observe property: %w", err)
	}

	return func() error {
		rmEventHandler()
		if _, err := c.CommandAsync("unobserve_property", observerID); err != nil {
			return fmt.Errorf("failed to unobserve property: %w", err)
		}
		return nil
	}, nil
}

// Command sends a command to MPV. See https://mpv.io/manual/stable/#list-of-input-commands
// for a list of commands and their arguments.
func (c *mpvClient) Command(command string, args ...any) (any, error) {
	return c.command(false, command, args...)
}

// CommandAsync sends a command to MPV as an asynchronous command.
func (c *mpvClient) CommandAsync(command string, args ...any) (any, error) {
	return c.command(true, command, args...)
}

func (c *mpvClient) AddEventHandlerSync(fn func(map[string]any)) (rm func()) {
	c.eventHandlersMu.Lock()
	defer c.eventHandlersMu.Unlock()
	handler := &eventHandler{sync: true, fn: fn}
	c.eventHandlers = append(c.eventHandlers, handler)
	return c.removeEventHandler(handler)
}

func (c *mpvClient) AddEventHandler(fn func(map[string]any)) (rm func()) {
	c.eventHandlersMu.Lock()
	defer c.eventHandlersMu.Unlock()
	handler := &eventHandler{sync: false, fn: fn}
	c.eventHandlers = append(c.eventHandlers, handler)
	return c.removeEventHandler(handler)
}

func (c *mpvClient) removeEventHandler(handler *eventHandler) func() {
	return func() {
		c.eventHandlersMu.Lock()
		defer c.eventHandlersMu.Unlock()
		for i, h := range c.eventHandlers {
			if h == handler {
				c.eventHandlers = append(c.eventHandlers[:i], c.eventHandlers[i+1:]...)
				return
			}
		}
	}
}

func (c *mpvClient) acceptEvents() {
	for event := range c.ipc.events {
		c.eventHandlersMu.Lock()
		for _, handler := range c.eventHandlers {
			if handler.sync {
				handler.fn(event)
			} else {
				go handler.fn(event)
			}
		}
		c.eventHandlersMu.Unlock()
	}
}

func (c *mpvClient) command(async bool, command string, args ...any) (data any, err error) {
	args = append([]any{command}, args...)
	resp, err := c.ipc.sendCommand(async, args...)
	if err != nil {
		return nil, err
	}
	if !resp.isSuccess() {
		return nil, fmt.Errorf("mpv: command failed: %s", resp.Error)
	}
	return resp.Data, nil
}
