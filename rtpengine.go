package main

import (
	"log"
	"net"
	"strconv"
	"io"
	"fmt"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

type rtpEngine struct {
	server net.PacketConn
	sipAddr net.Addr
	port int
	host string
}

func newRTPEngine(host string) (*rtpEngine, error) {
	srv, err := net.ListenPacket("udp", fmt.Sprintf("%s:0", host))
	if err != nil {
		return nil, err
	}

	return &rtpEngine{server: srv, sipAddr: nil, host: host}, nil
}

func (c *rtpEngine) Addr() string {
	return c.server.LocalAddr().String()
}

func (c *rtpEngine) Port() int {
	_, port, _ := net.SplitHostPort(c.Addr())
	iport, _ := strconv.Atoi(port)
	return iport
}

func (c *rtpEngine) SetSIPSDP(sdpBody string) {
	parsed := &sdp.SessionDescription{}
	if err := parsed.Unmarshal([]byte(sdpBody)); err != nil {
		log.Fatal(err)
	}
	if len(parsed.MediaDescriptions) == 0 {
		return
	}

	address := fmt.Sprintf("%s:%d",
		parsed.ConnectionInformation.Address.Address,
		parsed.MediaDescriptions[0].MediaName.Port.Value)
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		panic(err)
	}
	c.sipAddr = addr
}

func (c *rtpEngine) LocalSDP(sdpBody string) string {
	//https://pkg.go.dev/github.com/pion/sdp/v3#SessionDescription
	parsed := &sdp.SessionDescription{}
	if err := parsed.Unmarshal([]byte(sdpBody)); err != nil {
		log.Fatal(err)
	}

	// TODO apuntamos a ip del servidor 
	parsed.MediaDescriptions[0].ConnectionInformation.Address.Address = c.host
	parsed.MediaDescriptions[0].MediaName.Port.Value = c.Port()
	parsed.MediaDescriptions[0].MediaName.Protos = []string{"RTP/AVP"}
	parsed.Attributes = make([]sdp.Attribute, 0)

	attributes := make([]sdp.Attribute, 0)
	for _, remoteAttribute := range parsed.MediaDescriptions[0].Attributes {
		valids := map[string]bool{
			"rtpmap": true,
			"fmtp": true,
			"ptime": true,
		}
		if _, ok := valids[remoteAttribute.Key]; ok {
			attributes = append(attributes, remoteAttribute)
		}
	}
	parsed.MediaDescriptions[0].Attributes = attributes

	out, err := parsed.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	return string(out)
}

func (c *rtpEngine) Close() {
	c.server.Close()
}

func (c *rtpEngine) Write(in *webrtc.TrackRemote) {
	log.Printf("RTPENGINE TO SIP\n")
	for {
		rtpBuf := make([]byte, 1500)
		if _, _, err := in.Read(rtpBuf); err != nil {
			return
		}
		if c.sipAddr != nil {
			c.server.WriteTo(rtpBuf, c.sipAddr)
		}
	}
}

func (c *rtpEngine) Read(out io.Writer) {
	log.Printf("RTPENGINE FROM SIP\n")
	for {
		buf := make([]byte, 1024)
		_, _, err := c.server.ReadFrom(buf)
		if err != nil {
			log.Printf("RTPENGINE READING ERROR: %s\n", err)
			return
		}
		if _, err := out.Write(buf); err != nil {
			log.Printf("RTPENGINE WRITING ERROR: %s\n", err)
			return
		}
	}
}
