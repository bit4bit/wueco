package sipproto

import (
	"io"
	"time"
	"errors"
)

type WSReadMessage interface {
	ReadMessage() (messageType int, p []byte, err error)
}

type wsRead struct {
	wsR WSReadMessage
	r io.Reader
	w io.WriteCloser
	retries int
	retry_timeout time.Duration
}

func (c *wsRead) Run() {
	for {
		_, data, err := c.wsR.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if (c.retries < 1) {
					c.w.Close()
					panic("timeout reading websocket message")
				}
				c.retries -= 1
				time.Sleep(c.retry_timeout)
				continue
			}
			c.w.Close()
			return
		}
		c.w.Write(data)
	}
}

func (c *wsRead) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

type ReaderWSOption func(*wsRead)

func WithRetries(val int) ReaderWSOption {
	return func(c *wsRead) {
		c.retries = val
	}
}

func WithRetryTimeout(val time.Duration) ReaderWSOption {
	return func(c *wsRead) {
		c.retry_timeout = val
	}
}

func NewReaderWS(reader WSReadMessage, opts ...ReaderWSOption) *wsRead {
	r, w := io.Pipe()
	
	wsR := &wsRead{
		wsR: reader,
		r: r,
		w: w,
		retries: 15,
		retry_timeout: 10 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(wsR)
	}

	return wsR
}
