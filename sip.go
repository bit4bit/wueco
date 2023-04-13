package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/textproto"
	"regexp"
	"strings"
	"errors"

	"bit4bit.in/wueco/sipproto"
	"github.com/gorilla/websocket"
)

var (
	errNeedMoreData = errors.New("sip: need more data")
)

type sipMessage struct {
	statusLine string
	header     textproto.MIMEHeader
	content    string
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

func (c sipMessage) IsStatus(status string) bool {
	return strings.Contains(c.statusLine, status)
}

func (c sipMessage) IsMethod(method string) bool {
	return strings.Contains(c.statusLine, method)
}

func (c sipMessage) marshal() []byte {
	// se prueba usando como referencia
	// SetContent(content string)
	// strings.TrimSpace
	// bytes.Buffer
	// strings.Builder
	// bug go real length != len(content)
	var fixContent strings.Builder
	for _, b := range []byte(c.content) {
		if b != 0 {
			fixContent.Write([]byte{b})
		}
	}
	content := fixContent.String()


	rspBuf := new(bytes.Buffer)
	rspBuf.WriteString(c.statusLine)
	rspBuf.WriteString("\r\n")
	c.header.Set("content-length", fmt.Sprintf("%d", len(content)))
	for key, _ := range c.header {
		fmt.Fprintf(rspBuf, "%s: %s\r\n", key, c.header.Get(key))
	}
	rspBuf.WriteString("\r\n")
	rspBuf.WriteString(content)

	rawSIP := bytes.Replace(rspBuf.Bytes(), []byte("SIP/2.0/TCP"), []byte("SIP/2.0/WS"), 1)
	rawSIP = bytes.Replace(rawSIP, []byte("SIP/2.0/WS"), []byte("SIP/2.0/TCP"), 1)
	return rawSIP
}

func (c sipMessage) WriteMessage(conn *websocket.Conn) error {
	raw := c.marshal()
	log.Printf("SIP -> WEBRTC: %s\n", string(raw))
	return conn.WriteMessage(websocket.TextMessage, raw)
}

func (c sipMessage) Write(out io.Writer) (int, error) {
	raw := c.marshal()
	log.Printf("WEBRTC -> SIP: %s\n", string(raw))
	return out.Write(raw)
}

func newSIPMessage(msg *sipproto.Message) (*sipMessage, error) {
	header := textproto.MIMEHeader{}

	for key, val := range msg.Header {
		header.Set(key, val)
	}
	return &sipMessage{
		statusLine: msg.StatusLine,
		header:     header,
		content:    msg.Content,
	}, nil
}
