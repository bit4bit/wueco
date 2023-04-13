package sipproto

import (
	"bufio"
	"bytes"
	"net/textproto"
	"strings"
	"fmt"
	"strconv"
)

const (
	stateStatusLine = iota
	stateHeader
	stateContent
)

type Reader interface {
	ReadMessage() (*Message, error)
}

type sipRead struct {
	R *bufio.Reader
	state int
	buf bytes.Buffer
}

func NewReader(reader *bufio.Reader) *sipRead {
	return &sipRead{R: reader, state: stateStatusLine}
}

type Message struct {
	StatusLine string
	Header map[string]string
	Content string
}

func (c *sipRead) ReadMessage() (*Message, error) {
	var buf bytes.Buffer

	statusLine, isPrefix, err := c.R.ReadLine()
	// NOTE: si no hago esto al finalizar la funcion
	// statusLine contiene un valor diferente
	statusLineString := string(statusLine)

	if err != nil || isPrefix == true {
		return nil, fmt.Errorf("fails to read StatusLine: %w", err)
	}
	c.state = stateHeader


	// read header
	for c.state == stateHeader {
		line, _, err := c.R.ReadLine()
		if len(line) == 0 && buf.Len() > 0 && buf.Bytes()[buf.Len() - 1] == '\n' {
			buf.Write([]byte{'\n'})
			c.state = stateContent
			break
		}
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.Write([]byte{'\n'})
	}
	proto := textproto.NewReader(bufio.NewReader(&buf))
	proto_header, err := proto.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("fails to read header: %s\n", err)
	}

	header := make(map[string]string)
	for key, _ := range proto_header {
		header[strings.ToLower(key)] = proto_header.Get(key)
	}
	buf.Reset()

	// content
	var content strings.Builder
	content_length, _ := strconv.Atoi(header["content-length"])
	for c.state == stateContent && content_length > 0 {
		data := make([]byte, content_length)
		n, err := c.R.Read(data)
		if err != nil {
			return nil, fmt.Errorf("state content: %w", err)
		}
		content.Write(data)
		content_length -= n
	}
	c.state = stateStatusLine

	return &Message{Header: header, Content: content.String(), StatusLine: string(statusLineString)}, nil
}
