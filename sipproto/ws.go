package sipproto

import (
	"bytes"
	"io"
	"fmt"
	"time"
	"errors"
)

var errTimeoutWSMessage = errors.New("timeout reading websocket message")

type WSReadMessage interface {
	ReadMessage() (messageType int, p []byte, err error)
}

type wsRead struct {
	r WSReadMessage
	buf bytes.Buffer
	retries int
	retry_timeout time.Duration
}


func (c *wsRead) Read(p []byte) (int, error) {
	for {
		n, data, err := c.r.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if (c.retries < 1) {
					return 0, errTimeoutWSMessage
				}
				c.retries -= 1
				time.Sleep(c.retry_timeout)
				continue
			}
			return n, fmt.Errorf("ReadMessage: %w", err)
		}
		nWrite, err := c.buf.Write(data)
		if err != nil {
			return nWrite, fmt.Errorf("buffer.Write: %w", err)
		}
		break
	}

	return c.buf.Read(p)
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

func NewReaderWS(reader WSReadMessage, opts ...ReaderWSOption) io.Reader {
	r := &wsRead{
		r: reader,
		retries: 15,
		retry_timeout: 10 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(r)
	}

	return r
}
