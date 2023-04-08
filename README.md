# webrtc-to-sip (WIP)

Experimental WEBRTC to SIP (Lab) 

it's working with some issues and only supports OPUS

- [X] REGISTER
- [X] INVITE SIP -> WEBRTC
- [ ] INVITE WEBRTC -> SIP
- [X] AUDIO SIP -> WEBRTC
- [X] AUDIO WEBRTC -> SIP
- [ ] AUDIO WEBRTC -> SIP HIGH QUALITY
- [ ] SUPPORT CODEC PCMU
- [ ] SUPPORT CODEC PCMA
- [ ] SUPPORT CODEC G722

~~~
$ go run -host <ip listening> -sip <freeswitch ip>
~~~


# Resources

- https://github.com/pion/webrtc/blob/master/examples/rtp-forwarder/main.go
- https://github.com/pion/example-webrtc-applications
