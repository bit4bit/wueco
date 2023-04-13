package sipproto

import (
	"bufio"
	"net/textproto"
	"strings"
	"fmt"
)

type Reader interface {
	ReadMessage() (*Message, error)
}

type sipRead struct {
	R *bufio.Reader
}

func NewReader(reader *bufio.Reader) *sipRead {
	return &sipRead{R: reader}
}

type Message struct {
	Header map[string]string
}

func (c *sipRead) ReadMessage() (*Message, error) {
	proto := textproto.NewReader(c.R)
	proto_header, err := proto.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("fails to read header: %s\n", err)
	}

	header := make(map[string]string)
	for key, _ := range proto_header {
		header[strings.ToLower(key)] = proto_header.Get(key)
	}
	
	return &Message{Header: header}, nil
}
