package sipproto


import (
	"testing"
	"bufio"
	"bytes"
)

type wsFakeConn struct {
	buf bytes.Buffer
}

func (c *wsFakeConn) ReadMessage() (messageType int, p []byte, err error) {
	p = make([]byte, 512)
	messageType = 1
	_, err = c.buf.Read(p)
	return
}

func (c *wsFakeConn) WriteString(p string) {
	if _, err := c.buf.WriteString(p); err != nil {
		panic(err)
	}
}

func newWSFakeConn() *wsFakeConn {
	return &wsFakeConn{}
}

func TestWebsocketReader(t *testing.T) {
	pdu := `INVITE sip:bob@biloxi.com SIP/2.0
Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds
Max-Forwards: 70
To: Bob <sip:bob@biloxi.com>
From: Alice <sip:alice@atlanta.com>;tag=1928301774
Call-ID: a84b4c76e66710@pc33.atlanta.com
CSeq: 314159 INVITE
Contact: <sip:alice@pc33.atlanta.com>
Content-Type: application/sdp
Content-Length: 0

`
	wsconn := newWSFakeConn()
	wsconn.WriteString(pdu)

	wsReader := NewReaderWS(wsconn)
	go wsReader.Run()
	reader := NewReader(bufio.NewReader(wsReader))

	msg, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}

	if msg.Header["content-type"] != "application/sdp" {
		t.Errorf("fails to get header content-type")
	}
}

func TestWebsocketReadHeaderPartial(t *testing.T) {
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
	wsconn := newWSFakeConn()
	go func(){
		for _, part := range parts {
			wsconn.WriteString(part)
		}
	}()
	wsReader := NewReaderWS(wsconn)
	go wsReader.Run()
	reader := NewReader(bufio.NewReader(wsReader))

	msg, err := reader.ReadMessage()
	if err != nil {
		t.Fatalf("%s", err)
	}

	if msg.Header["content-type"] != "application/sdp" {
		t.Errorf("fails to get header content-type")
	}
}
