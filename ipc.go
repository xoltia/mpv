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
	closing         bool
	closingWg       sync.WaitGroup
	reqWg           sync.WaitGroup
	closeCh         chan struct{}
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

func (i *ipc) sendCommand(async bool, args ...any) (resp mpvResponse, err error) {
	if i.closing {
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

	// Just in case close was called between the closing check
	// and the request being added to the outgoing channel.
	i.reqWg.Add(1)
	i.outgoing <- req
	select {
	case resp = <-req.resp:
	case err = <-req.err:
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
	// Close the connection and signal the write loop to stop.
	err := i.conn.Close()
	if err != nil {
		return err
	}
	// Signal the read loop to stop.
	close(i.closeCh)
	// Stop accepting new requests.
	i.closing = true
	// Wait for both the read and write loops to stop.
	i.closingWg.Wait()

	// Signal all pending requests that the IPC has been closed.
	i.mu.Lock()
	for _, req := range i.pendingRequests {
		req.err <- ErrClosed
	}
	i.mu.Unlock()

	// Handle any requests that were initiated after the IPC was closed.
	go func() {
		for req := range i.outgoing {
			req.err <- ErrClosed
			i.reqWg.Done()
		}
	}()

	// Wait for requests to finish.
	i.reqWg.Wait()

	// Finally, close the channels.
	close(i.events)
	close(i.outgoing)

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
	defer i.closingWg.Done()

	for {
		select {
		case <-i.closeCh:
			return
		case req := <-i.outgoing:
			i.reqWg.Done()
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
