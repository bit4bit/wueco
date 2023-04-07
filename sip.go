package main

import (
	"log"
	"net/textproto"
	"bufio"
	"strconv"
	"fmt"
	"bytes"
	"io"
	"strings"
	"regexp"

	"github.com/gorilla/websocket"
)

type sipMessage struct {
	statusLine string
	header textproto.MIMEHeader
	content []byte
}

func (c sipMessage) To() string {
	return c.extractAddr("to")
}

func (c sipMessage) Contact() string {
	return c.extractAddr("contact")
}

func (c sipMessage) ContactFromTo(wsContact, host string) (string, string) {
	reContactAttrs := regexp.MustCompile(">(.+)$")
	reToName := regexp.MustCompile(":.+@")
	toAddr := c.header.Get("to")
	contactAttrs := strings.TrimLeft(reContactAttrs.FindString(wsContact), ">")
	toName := strings.Trim(reToName.FindString(toAddr), ":@")

	addr := fmt.Sprintf("sip:%s@%s;transport=tcp", toName, host)
	return addr, fmt.Sprintf("<%s>%s", addr, contactAttrs)
}

func (c sipMessage) extractAddr(field string) string {
	reAddr := regexp.MustCompile("<[^>]+")
	return strings.Trim(reAddr.FindString(c.header.Get(field)), "<>")
}

func (c sipMessage) IsMethod(method string) bool {
	return strings.Contains(c.statusLine, method)
}

func (c sipMessage) marshal() []byte {
	rspBuf := new(bytes.Buffer)
	rspBuf.WriteString(c.statusLine)
	rspBuf.WriteString("\r\n")
	c.header.Set("content-length", itoa(len(c.content)))
	for key, values := range c.header {
		fmt.Fprintf(rspBuf, "%s: %s\r\n", key, values[0])
	}
	rspBuf.WriteString("\r\n")
	rspBuf.Write(c.content)

	rawSIP := bytes.Replace(rspBuf.Bytes(), []byte("SIP/2.0/TCP"), []byte("SIP/2.0/WS"), 1)
	rawSIP = bytes.Replace(rawSIP, []byte("SIP/2.0/WS"), []byte("SIP/2.0/TCP"), 1)
	return rawSIP
}

func (c sipMessage) WriteMessage(conn *websocket.Conn) error {
	raw := c.marshal()
	log.Printf("SIP -> WEBRTC: %s\n", string(raw))
	return conn.WriteMessage(websocket.TextMessage, raw)
}

func (c sipMessage) Write(out io.Writer) (int, error)  {
	raw := c.marshal()
	log.Printf("WEBRTC -> SIP: %s\n", string(raw))
	return out.Write(raw)
}

func newSIPMessage(io *bufio.Reader) (*sipMessage, error) {
	reader := textproto.NewReader(io)
	statusLine, err := reader.ReadLine()
	if err != nil {
		return nil, err
	}
	header, err := reader.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	content_length, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		return nil, err
	}
	
	content := make([]byte, content_length)
	_, err = reader.R.Read(content)
	if err != nil {
		return nil, err
	}

	return &sipMessage{
		statusLine: statusLine,
		header: header,
		content: content,
	}, nil
}
