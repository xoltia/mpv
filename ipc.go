package mpv

import (
	"bufio"
	"encoding/json"
	"errors"
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
}

type ipc struct {
	conn            net.Conn
	scanner         *bufio.Scanner
	mu              sync.Mutex
	requestID       atomic.Int64
	outgoing        chan request
	events          chan map[string]any
	pendingRequests map[int64]request
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
	go i.writeLoop()
	go i.readLoop()
}

func (i *ipc) sendCommand(async bool, args ...any) (resp mpvResponse, err error) {
	if i.conn == nil {
		err = ErrClosed
		return
	}

	id := i.requestID.Add(1) - 1

	req := request{
		command: mpvCommand{
			Command:   args,
			Async:     async,
			RequestID: id,
		},
		resp: make(chan mpvResponse, 1),
		err:  make(chan error, 1),
	}

	i.outgoing <- req
	select {
	case resp = <-req.resp:
	case err = <-req.err:
	}

	return
}

func (i *ipc) read() ([]byte, error) {
	if !i.scanner.Scan() {
		return nil, i.scanner.Err()
	}
	return i.scanner.Bytes(), nil
}

func (i *ipc) write(data []byte) error {
	_, err := i.conn.Write(data)
	return err
}

func (i *ipc) close() error {
	close(i.outgoing)
	close(i.events)
	err := i.conn.Close()
	i.conn = nil

	i.mu.Lock()
	for _, req := range i.pendingRequests {
		req.err <- ErrClosed
	}
	i.mu.Unlock()
	return err
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
	for req := range i.outgoing {
		i.mu.Lock()
		i.pendingRequests[req.command.RequestID] = req
		i.mu.Unlock()

		if err := i.writeJSON(req.command); err != nil {
			req.err <- err
			i.mu.Lock()
			delete(i.pendingRequests, req.command.RequestID)
			i.mu.Unlock()
			continue
		}
	}
}

func (i *ipc) readLoop() {
	for {
		data, err := i.read()
		if err != nil {
			return
		}

		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		if event["error"] != nil {
			i.mu.Lock()
			reqID := int64(event["request_id"].(float64))
			if req, ok := i.pendingRequests[reqID]; ok {
				req.resp <- mpvResponse{
					Error:     event["error"].(string),
					RequestID: reqID,
					Data:      event["data"],
				}
				delete(i.pendingRequests, reqID)
			}
			i.mu.Unlock()
			continue
		}

		if event["event"] != nil {
			i.events <- event
			continue
		}
	}
}
