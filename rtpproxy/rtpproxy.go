package rtpproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"

	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

type RTPProxy struct {
	server  net.PacketConn
	sipAddr net.Addr
	port    int
	host    string
}

func NewRTPProxy(host string) (*RTPProxy, error) {
	srv, err := net.ListenPacket("udp", fmt.Sprintf("%s:0", host))
	if err != nil {
		return nil, err
	}

	return &RTPProxy{server: srv, sipAddr: nil, host: host}, nil
}

func (c *RTPProxy) Addr() string {
	return c.server.LocalAddr().String()
}

func (c *RTPProxy) Port() int {
	_, port, _ := net.SplitHostPort(c.Addr())
	iport, _ := strconv.Atoi(port)
	return iport
}

func (c *RTPProxy) SetSIPSDP(sdpBody string) {
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

func (c *RTPProxy) LocalSDP(sdpBody string) string {
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
			"fmtp":   true,
			"ptime":  true,
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

func (c *RTPProxy) Close() {
	c.server.Close()
}

func (c *RTPProxy) Write(ctx context.Context, in *webrtc.TrackRemote) {
	log.Printf("RTPPROXY TO SIP\n")

	rtpBuf := make([]byte, 1600)
	rtpPacket := &rtp.Packet{}
	for {
		select {
		case <-ctx.Done():
		default:
			n, _, err := in.Read(rtpBuf)
			if err != nil {
				return
			}
			if err = rtpPacket.Unmarshal(rtpBuf[:n]); err != nil {
				panic(err)
			}
			rtpPacket.PayloadType = 111
			if n, err = rtpPacket.MarshalTo(rtpBuf); err != nil {
				panic(err)
			}

			if c.sipAddr != nil {
				if _, writeErr := c.server.WriteTo(rtpBuf[:n], c.sipAddr); writeErr != nil {
					var opError *net.OpError
					if errors.As(writeErr, &opError) && opError.Err.Error() == "write: connection refused" {
						continue
					}
					return
				}
			}
		}
	}
}

func (c *RTPProxy) Read(ctx context.Context, out io.Writer) {
	log.Printf("RTPPROXY FROM SIP\n")

	rtpBuf := make([]byte, 1600)
	rtpPacket := &rtp.Packet{}
	for {
		select {
		case <-ctx.Done():
			break
		default:
			n, _, err := c.server.ReadFrom(rtpBuf)
			if err != nil {
				log.Printf("RTPPROXY READING ERROR: %s\n", err)
				return
			}
			if err = rtpPacket.Unmarshal(rtpBuf[:n]); err != nil {
				panic(err)
			}
			if n, err = rtpPacket.MarshalTo(rtpBuf); err != nil {
				panic(err)
			}

			if _, err := out.Write(rtpBuf[:n]); err != nil {
				log.Printf("RTPPROXY WRITING ERROR: %s\n", err)
				return
			}
		}
	}
}
