package sipproto

import (
	"testing"
	"bufio"
	"bytes"
	"net"
)


func TestReadMessage(t *testing.T) {
	pdu := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 3

abc
`
	
	reader := NewReader(bufio.NewReader(bytes.NewBufferString(pdu)))

	msg, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}
	
	if msg.StatusLine != "INVITE sip:bob@biloxi.com SIP/2.0" {
		t.Errorf("fails to get status line got %s\n", msg.StatusLine)
	}
	if msg.Content != "abc" {
		t.Errorf("fails to get content got %s\n", msg.Content)
	}
}

func TestReadContinuosContent(t *testing.T) {
	pdu := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 3

abcINVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 3

123
`
	
	reader := NewReader(bufio.NewReader(bytes.NewBufferString(pdu)))

	msg, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}

	if msg.Content != "abc" {
		t.Errorf("fails to get content got %s\n", msg.Content)
	}

	msg, err = reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}

	if msg.Content != "123" {
		t.Errorf("fails to get content got %s\n", msg.Content)
	}
}


func TestReadHeaderPartial(t *testing.T) {
	var parts [3]string
	parts[0] = `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip`
	parts[1] = `:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice`
	parts[2] = `@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 0

`
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Errorf("%s", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("%s", err)
		}
		for _, part := range parts {
			conn.Write([]byte(part))
		}
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Errorf("%s", err)
	}
	reader := NewReader(bufio.NewReader(conn))

	msg, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}

	if msg.Header["content-type"] != "application/sdp" {
		t.Errorf("fails to get header content-type")
	}
}
