package main

import (
	"io"
	"bufio"
	"errors"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"

	"bit4bit.in/wueco/rtpproxy"
	"bit4bit.in/wueco/sipproto"
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
		log.Fatal(err)
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
	ctxRTCP, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxyRTCP(ctxRTCP, rtpengine, peerConn, audioTrack)

	peerConn.OnTrack(func(track *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		log.Println("OnTrack")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go rtpengine.WriteRTCP(ctx, r)
		go rtpengine.Read(ctx, audioTrack)
		rtpengine.Write(ctx, track)
	})

	if _, err = peerConn.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal(err)
	}

	offer := &webrtc.SessionDescription{}

	sipConnRaw, err := net.Dial("tcp", *sipAddress)
	if err != nil {
		log.Println(err)
		return
	}
	defer sipConnRaw.Close()


	sipReader := sipproto.NewReader(bufio.NewReader(sipConnRaw))


	// SIP -> WS
	go func() {
		for {
			protoMsg, err := sipReader.ReadMessage()
			if err != nil {
				if errors.Is(err, io.EOF) {
					continue
				}
				fmt.Printf("[ERR] SIP -> WS newSIPMessage: %s\n", err)
				return
			}
			sipMsg, _ := newSIPMessage(protoMsg)
			if wsContact, ok := contactSIPToWS[sipMsg.Contact()]; ok {
				sipMsg.header.Set("contact", wsContact)
			}

			if err := proxyRTPSIPToWS(peerConn, sipMsg, rtpengine, offer); err != nil {
				log.Printf("[ERR] proxyRTPSIPToWS: %s\n", err)
				return
			}

			sipMsg.WriteMessage(conn)
		}
	}()


	
	// WS -> SIP
	wsR := sipproto.NewReaderWS(conn)
	go wsR.Run()
	wsReader := sipproto.NewReader(bufio.NewReader(wsR))
	for {
		protoMsg, err := wsReader.ReadMessage()
		if err != nil {
			if errors.Is(err, errNeedMoreData) {
				continue
			}
			if errors.Is(err, io.EOF) {
				continue
			}
			log.Printf("[ERR] WS - SIP newSipMessage: %s\n", err)
			return
		}
		sipMsg, _ := newSIPMessage(protoMsg)
		wsContact := sipMsg.header.Get("contact")
		sipAddr, sipContact := sipMsg.ContactFromTo(wsContact, sipConnRaw.LocalAddr().String())
		contactSIPToWS[sipAddr] = wsContact
		contactWSToSIP[wsContact] = sipContact
		
		if sipContact, ok := contactWSToSIP[wsContact]; ok {
			// enviamos el contact de wueco
			sipMsg.header.Set("contact", sipContact)
		}
		
		
		if err := proxyRTPWSToSIP(peerConn, sipMsg, rtpengine, offer); err != nil {
			log.Printf("[ERR] proxyRTPWSToSIP: %s", err)
			return
		}
		
		if _, err := sipMsg.Write(sipConnRaw); err != nil {
			log.Printf("[ERR] sipConn.Write: %w", err)
			return
		}

	}
}


func proxyRTPWSToSIP(peerConn *webrtc.PeerConnection, sipMsg *sipMessage, rtpengine *rtpproxy.RTPProxy, offer *webrtc.SessionDescription) error {
	content := string(sipMsg.content)
	if sipMsg.IsMethod("INVITE") && sipMsg.header.Get("content-type") == "application/sdp" {
		if sipMsg.header.Get("proxy-authorization") == "" {
			if err := peerConn.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: content}); err != nil {
				panic(err)
				return err
			}
			
			loffer, err := peerConn.CreateAnswer(nil)
			if err != nil {
				panic(err)
				return err
			}
			if err := peerConn.SetLocalDescription(loffer); err != nil {
				panic(err)
				return err
			}
			*offer = loffer
		} else {
			content = string(sipMsg.content)
		}


		sipMsg.content = rtpengine.LocalSDP(content)
	} else if sipMsg.IsStatus("200") && sipMsg.header.Get("content-type") == "application/sdp" {
		content := string(sipMsg.content)
		
		if err := peerConn.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: content}); err != nil {
			return err
		}
		wuecoSDP := rtpengine.LocalSDP(content)
		sipMsg.content =wuecoSDP
	}

	return nil
}

func proxyRTPSIPToWS(peerConn *webrtc.PeerConnection, sipMsg *sipMessage, rtpengine *rtpproxy.RTPProxy, offer *webrtc.SessionDescription) error {
	if sipMsg.IsMethod("INVITE") && sipMsg.header.Get("content-disposition") == "session" {
		loffer, err := peerConn.CreateOffer(nil)
		if err != nil {
			return err
		}
		if err := peerConn.SetLocalDescription(loffer); err != nil {
			return err
		}
		*offer = loffer

		rtpengine.SetSIPSDP(string(sipMsg.content))
		
		//ofrecemos al navegador el sdp de wueco
		sipMsg.content = (*offer).SDP
	} else if sipMsg.IsStatus("200") && sipMsg.header.Get("content-type") == "application/sdp" {
		rtpengine.SetSIPSDP(string(sipMsg.content))
		sipMsg.content = (*offer).SDP
	}

	return nil

}

func proxyRTCP(ctx context.Context, rtpengine *rtpproxy.RTPProxy, pc *webrtc.PeerConnection, track *webrtc.TrackLocalStaticRTP) {
	if track == nil {
		panic("expected track")
		return
	}
	_, err := pc.AddTrack(track)
	if err != nil {
		panic(err)
	}

	go rtpengine.ReadRTCP(ctx, pc)
}

func itoa(s int) string {
	return strconv.Itoa(s)
}
