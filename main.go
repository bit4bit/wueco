package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"bit4bit.in/wueco/rtpproxy"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var (
	host       = flag.String("host", "", "Host that websocket is available on")
	sipAddress = flag.String("sip", "", "SIP Server Host example: 1.2.3.5:5060")
)

func main() {
	flag.Parse()
	if *host == "" || *sipAddress == "" {
		log.Fatal("-host, -sip are required")
	}

	http.HandleFunc("/ws", websocketHandler)
	log.Fatal(http.ListenAndServe("localhost:8088", nil))
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("New websocket connection")

	contactWSToSIP := make(map[string]string)
	contactSIPToWS := make(map[string]string)

	rtpengine, err := rtpproxy.NewRTPProxy(*host)
	defer rtpengine.Close()

	if err != nil {
		log.Printf("[ERR] newRTPEngine: %s\n", err)
	}
	log.Printf("RTPENGINE LISTENING AT %s\n", rtpengine.Addr())

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer conn.Close()

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	peerConn, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Println(err)
		return
	}

	defer peerConn.Close()

	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Println(err)
			}
		case webrtc.PeerConnectionStateClosed:
			log.Println("PeerConnectionStateClosed")
		}

	})

	// TODO: construir desde fmtp
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "wueco")
	if err != nil {
		panic(err)
	}
	
	rtpSender, err := peerConn.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}
	ctxRTCP, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rtpengine.WriteRTCP(ctxRTCP, rtpSender)
	go rtpengine.ReadRTCP(ctxRTCP, peerConn)
	

	peerConn.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Println("OnTrack")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go rtpengine.Read(ctx, audioTrack)
		rtpengine.Write(ctx, track)
	})

	if _, err = peerConn.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal(err)
	}
	offer, err := peerConn.CreateOffer(nil)
	if err != nil {
		panic(err)
	}

	if err := peerConn.SetLocalDescription(offer); err != nil {
		panic(err)
	}

	sipConnRaw, err := net.Dial("tcp", *sipAddress)
	if err != nil {
		log.Println(err)
		return
	}
	sipConn := textproto.NewConn(sipConnRaw)
	defer sipConn.Close()

	// SIP -> WS
	go func() {
		for {
			sipMsg, err := newSIPMessage(sipConn.R)
			if err != nil {
				fmt.Printf("[ERR] newSIPMessage: %s\n", err)
				return
			}

			if wsContact, ok := contactSIPToWS[sipMsg.Contact()]; ok {
				sipMsg.header.Set("contact", wsContact)
			}

			content := sipMsg.content
			if strings.HasPrefix(sipMsg.statusLine, "INVITE") {
				if sipMsg.header.Get("content-disposition") == "session" {
					rtpengine.SetSIPSDP(string(sipMsg.content))

					//ofrecemos al navegador el sdp de wueco
					content = []byte(offer.SDP)
				}
			}
			sipMsg.content = content
			sipMsg.WriteMessage(conn)

		}
	}()

	// WS -> SIP
	for {
		// cada peticion sip se envia completamente en el message?
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		//log.Printf("ReadMessage: [%s]\n", string(raw))
		msgBuf := new(bytes.Buffer)
		msgBuf.Write(raw)
		sipMsg, err := newSIPMessage(bufio.NewReader(msgBuf))
		if err != nil {
			log.Printf("[ERR] newSipMessage: %s\n", err)
			return
		}
		content := sipMsg.content

		wsContact := sipMsg.header.Get("contact")
		sipAddr, sipContact := sipMsg.ContactFromTo(wsContact, sipConnRaw.LocalAddr().String())
		contactSIPToWS[sipAddr] = wsContact
		contactWSToSIP[wsContact] = sipContact

		if sipContact, ok := contactWSToSIP[wsContact]; ok {
			// enviamos el contact de wueco
			sipMsg.header.Set("contact", sipContact)
		}

		if strings.HasPrefix(sipMsg.statusLine, "SIP/2.0 200 OK") && sipMsg.header.Get("content-type") == "application/sdp" {

			if err := peerConn.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: string(content)}); err != nil {
				log.Printf("[ERR] setRemoteDescription: %s\n", err)
				return
			}
			log.Println("[INFO] setRemoteDescription OK\n")
			wuecoSDP := rtpengine.LocalSDP(string(content))
			content = []byte(wuecoSDP)
		}
		sipMsg.content = content

		if _, err := sipMsg.Write(sipConn.W); err != nil {
			log.Printf("[ERR] sipConn.Write: %w", err)
			return
		}

		sipConn.W.Flush()
	}
}

func itoa(s int) string {
	return strconv.Itoa(s)
}
