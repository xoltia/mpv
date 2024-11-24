package mpv

import (
	"context"
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

// Request represents an asynchronous request to MPV. When waiting for a response,
// you should always check the error channel for any errors that may have occurred.
// A canceled request will not return an error, a request will only be canceled if
// Cancel is called on the request or the parent context is canceled.
type Request struct {
	ID       int64
	Response <-chan Response
	Error    <-chan error
	Cancel   context.CancelFunc
}

type eventHandler struct {
	sync bool
	fn   func(map[string]any)
}

type Client struct {
	ipc             *ipc
	eventHandlersMu sync.Mutex
	eventHandlers   []*eventHandler
	observerID      atomic.Int64
}

func (c *Client) Close() error {
	return c.ipc.close()
}

func (c *Client) Play(ctx context.Context) error {
	return c.SetProperty(ctx, "pause", false)
}

func (c *Client) Pause(ctx context.Context) error {
	return c.SetProperty(ctx, "pause", true)
}

func (c *Client) Seek(ctx context.Context, position float64, flags ...SeekFlag) error {
	var err error
	if len(flags) == 0 {
		_, err = c.Command(ctx, "seek", position)
	} else {
		flag := strings.Builder{}
		for i, f := range flags {
			if i > 0 {
				flag.WriteRune('+')
			}
			flag.WriteString(string(f))
		}
		_, err = c.Command(ctx, "seek", position, flag)
	}

	return err
}

func (c *Client) LoadFile(ctx context.Context, file string, mode LoadFileMode) error {
	_, err := c.Command(ctx, "loadfile", file, string(mode))
	return err
}

// Property setters

func (c *Client) SetProperty(ctx context.Context, property string, value any) error {
	_, err := c.Command(ctx, "set_property", property, value)
	return err
}

func (c *Client) SetVolume(ctx context.Context, volume float64) error {
	return c.SetProperty(ctx, "volume", volume)
}

func (c *Client) SetMute(ctx context.Context, mute bool) error {
	return c.SetProperty(ctx, "mute", mute)
}

func (c *Client) SetLoop(ctx context.Context, loop bool) error {
	return c.SetProperty(ctx, "loop", loop)
}

func (c *Client) SetSpeed(ctx context.Context, speed float64) error {
	return c.SetProperty(ctx, "speed", speed)
}

func (c *Client) SetPosition(ctx context.Context, position float64) error {
	return c.SetProperty(ctx, "time-pos", position)
}

// Property getters

func (c *Client) GetProperty(ctx context.Context, property string) (any, error) {
	return c.Command(ctx, "get_property", property)
}

func (c *Client) GetPaused(ctx context.Context) (bool, error) {
	return c.GetPropertyBool(ctx, "pause")
}

func (c *Client) GetDuration(ctx context.Context) (float64, error) {
	return c.GetPropertyFloat(ctx, "duration")
}

func (c *Client) GetPosition(ctx context.Context) (float64, error) {
	return c.GetPropertyFloat(ctx, "time-pos")
}

func (c *Client) GetVolume(ctx context.Context) (float64, error) {
	return c.GetPropertyFloat(ctx, "volume")
}

func (c *Client) GetMute(ctx context.Context) (bool, error) {
	return c.GetPropertyBool(ctx, "mute")
}

func (c *Client) GetFilename(ctx context.Context) (string, error) {
	return c.GetPropertyString(ctx, "filename")
}

func (c *Client) GetSpeed(ctx context.Context) (float64, error) {
	return c.GetPropertyFloat(ctx, "speed")
}

func (c *Client) GetIdleActive(ctx context.Context) (bool, error) {
	return c.GetPropertyBool(ctx, "idle-active")
}

func (c *Client) GetLoop(ctx context.Context) (bool, error) {
	return c.GetPropertyBool(ctx, "loop")
}

func (c *Client) GetPropertyBool(ctx context.Context, property string) (b bool, err error) {
	value, err := c.GetProperty(ctx, property)
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

func (c *Client) GetPropertyFloat(ctx context.Context, property string) (f float64, err error) {
	value, err := c.GetProperty(ctx, property)
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

func (c *Client) GetPropertyString(ctx context.Context, property string) (s string, err error) {
	value, err := c.GetProperty(ctx, property)
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

// ObserveProperty observes a property and calls the provided function when it changes.
func (c *Client) ObserveProperty(ctx context.Context, property string, fn func(any)) (rm func() error, err error) {
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

	if _, err = c.Command(ctx, "observe_property", observerID, property); err != nil {
		rmEventHandler()
		return nil, fmt.Errorf("failed to observe property: %w", err)
	}

	return func() error {
		rmEventHandler()
		if _, err := c.Command(ctx, "unobserve_property", observerID); err != nil {
			return fmt.Errorf("failed to unobserve property: %w", err)
		}
		return nil
	}, nil
}

// Command sends a command to MPV. See https://mpv.io/manual/stable/#list-of-input-commands
// for a list of commands and their arguments.
func (c *Client) Command(ctx context.Context, command string, args ...any) (any, error) {
	return c.command(ctx, false, command, args...)
}

// CommandAsync sends a command to MPV as an asynchronous command.
// Returns an AsyncRequest that can be used to wait for the response or
// cancel the request.
func (c *Client) CommandAsync(ctx context.Context, command string, args ...any) (preq Request, err error) {
	ctx, cancel := context.WithCancel(ctx)
	req, err := c.ipc.startRequest(ctx, true, append([]any{command}, args...)...)
	if err != nil {
		cancel()
		return
	}
	preq.ID = req.command.RequestID
	preq.Response = req.resp
	preq.Error = req.err
	preq.Cancel = cancel
	return
}

// AddEventHandlerSync adds a synchronous event handler to the client.
// This handler will block the event loop until it returns.
func (c *Client) AddEventHandlerSync(fn func(map[string]any)) (rm func()) {
	c.eventHandlersMu.Lock()
	defer c.eventHandlersMu.Unlock()
	handler := &eventHandler{sync: true, fn: fn}
	c.eventHandlers = append(c.eventHandlers, handler)
	return c.removeEventHandler(handler)
}

// AddEventHandler adds an event handler to the client. This handler will be
// called in a new goroutine when an event is received.
func (c *Client) AddEventHandler(fn func(map[string]any)) (rm func()) {
	c.eventHandlersMu.Lock()
	defer c.eventHandlersMu.Unlock()
	handler := &eventHandler{sync: false, fn: fn}
	c.eventHandlers = append(c.eventHandlers, handler)
	return c.removeEventHandler(handler)
}

func (c *Client) removeEventHandler(handler *eventHandler) func() {
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

func (c *Client) acceptEvents() {
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

func (c *Client) command(ctx context.Context, async bool, command string, args ...any) (data any, err error) {
	args = append([]any{command}, args...)
	resp, err := c.ipc.sendCommandSync(ctx, async, args...)
	if err != nil {
		return nil, err
	}
	if !resp.Success() {
		return nil, fmt.Errorf("mpv: command failed: %s", resp.Error)
	}
	return resp.Data, nil
}
