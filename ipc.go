package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

var ErrClosed = errors.New("ipc: closed")

type mpvCommand struct {
	Command   []any `json:"command"`
	Async     bool  `json:"async"`
	RequestID int64 `json:"request_id"`
}

type mpvResponse struct {
	Data      interface{} `json:"data"`
	Error     string      `json:"error"`
	RequestID int64       `json:"request_id"`
}

func (r mpvResponse) isSuccess() bool {
	return r.Error == "success"
}

type request struct {
	command mpvCommand
	resp    chan mpvResponse
	err     chan error
	ctx     context.Context
}

type ipc struct {
	conn    net.Conn
	scanner *bufio.Scanner

	requestID atomic.Int64
	outgoing  chan request
	events    chan map[string]any

	pendingRequestsMu sync.Mutex
	pendingRequests   map[int64]request

	closing   bool
	closingWg sync.WaitGroup
	closeCh   chan struct{}
	closeMu   sync.Mutex
}

func newIPC(socket net.Conn) *ipc {
	return &ipc{conn: socket}
}

func (i *ipc) init() {
	if i.conn == nil {
		panic("ipc: conn is nil")
	}
	i.outgoing = make(chan request)
	i.events = make(chan map[string]any)
	i.pendingRequests = make(map[int64]request)
	i.scanner = bufio.NewScanner(i.conn)
	i.closeCh = make(chan struct{})

	i.closingWg.Add(2)
	go i.writeLoop()
	go i.readLoop()
}

func (i *ipc) sendCommand(ctx context.Context, async bool, args ...any) (resp mpvResponse, err error) {
	if i.closing {
		err = ErrClosed
		return
	}

	id := i.requestID.Add(1) - 1

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req := request{
		command: mpvCommand{
			Command:   args,
			Async:     async,
			RequestID: id,
		},
		resp: make(chan mpvResponse, 1),
		err:  make(chan error, 1),
		ctx:  ctx,
	}

	select {
	case i.outgoing <- req:
	case <-ctx.Done():
		err = ctx.Err()
		return
	case <-i.closeCh:
		err = ErrClosed
		return
	}

	select {
	case resp = <-req.resp:
	case err = <-req.err:
	case <-ctx.Done():
		err = ctx.Err()
	case <-i.closeCh:
		err = ErrClosed
	}

	return
}

func (i *ipc) read() ([]byte, error) {
	if !i.scanner.Scan() {
		if err := i.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, ErrClosed
	}
	return i.scanner.Bytes(), nil
}

func (i *ipc) write(data []byte) error {
	_, err := i.conn.Write(data)
	return err
}

func (i *ipc) close() error {
	// Prevent double closing.
	// Fast path for when the connection is closed
	// and resources are being cleaned up.
	if i.closing {
		return nil
	}

	// Slow path for a close in progress that may or
	// may not succeed.
	i.closeMu.Lock()
	select {
	case <-i.closeCh:
		i.closeMu.Unlock()
		return nil
	default:
	}
	// Close the connection and signal the read loop to stop.
	err := i.conn.Close()
	if err != nil {
		i.closeMu.Unlock()
		return fmt.Errorf("ipc: failed to close connection: %w", err)
	}
	// Stop accepting new requests.
	i.closing = true
	// Signal the write loop to stop.
	close(i.closeCh)
	// Unlock the mutex, allows for subsequent calls to Close.
	i.closeMu.Unlock()

	// Wait for both the read and write loops to stop.
	i.closingWg.Wait()

	// Signal all pending requests that the IPC has been closed.
	i.pendingRequestsMu.Lock()
	for _, req := range i.pendingRequests {
		req.err <- ErrClosed
	}
	i.pendingRequestsMu.Unlock()

	// Finally, close the channels.
	close(i.events)
	close(i.outgoing)
	return nil
}

func (i *ipc) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return i.write(data)
}

func (i *ipc) writeLoop() {
	defer i.closingWg.Done()

	for {
		select {
		case <-i.closeCh:
			return
		case req := <-i.outgoing:
			//i.reqWg.Done()
			i.pendingRequestsMu.Lock()
			i.pendingRequests[req.command.RequestID] = req
			i.pendingRequestsMu.Unlock()

			if err := i.writeJSON(req.command); err != nil {
				select {
				case req.err <- err:
				case <-req.ctx.Done():
				}
				i.pendingRequestsMu.Lock()
				delete(i.pendingRequests, req.command.RequestID)
				i.pendingRequestsMu.Unlock()
				continue
			}
		}
	}
}

func (i *ipc) readLoop() {
	defer i.closingWg.Done()

	for {
		data, err := i.read()
		if err != nil {
			return
		}

		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		switch {
		case event["event"] != nil:
			select {
			case i.events <- event:
			case <-i.closeCh:
				return
			default:
				// Drop event if no one is listening
			}
		case event["error"] != nil:
			i.handleResponse(event)
		}
	}
}

func (i *ipc) handleResponse(event map[string]any) {
	i.pendingRequestsMu.Lock()
	defer i.pendingRequestsMu.Unlock()

	reqID := int64(event["request_id"].(float64))
	if req, ok := i.pendingRequests[reqID]; ok {
		response := mpvResponse{
			Error:     event["error"].(string),
			RequestID: reqID,
			Data:      event["data"],
		}

		select {
		case req.resp <- response:
		case <-req.ctx.Done():
		}

		delete(i.pendingRequests, reqID)
	}
}
