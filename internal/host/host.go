// Package host runs the WebRTC side of a PunchPage host: it answers browser
// offers received through the signaling channel, tracks per-peer sessions,
// and attaches a tunnel bridge to each incoming data channel.
package host

import (
	"context"
	"log"
	"net/url"

	"github.com/pion/webrtc/v4"

	"github.com/thewh1teagle/punchpage/internal/signaling"
	"github.com/thewh1teagle/punchpage/internal/tunnel"
	"github.com/thewh1teagle/punchpage/internal/wire"
)

// peerSession holds one browser peer's connection and any ICE candidates
// that arrived before its offer.
type peerSession struct {
	pc        *webrtc.PeerConnection
	remoteSet bool
	queued    []webrtc.ICECandidateInit
}

// Run answers signaling messages until ctx is canceled, creating a WebRTC
// peer connection per browser and bridging its data channel to the local
// origin at base. If iface is non-empty, ICE is restricted to that network
// interface. Run always returns ctx.Err() on shutdown.
func Run(ctx context.Context, signaler *signaling.Signaler, iface string, base *url.URL) error {
	settings := webrtc.SettingEngine{}
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	if iface != "" {
		settings.SetInterfaceFilter(func(name string) bool { return name == iface })
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	configuration := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.cloudflare.com:3478", "stun:stun.l.google.com:19302"}}}}
	sessions := make(map[string]*peerSession)
	defer func() {
		for _, session := range sessions {
			if session.pc != nil {
				_ = session.pc.Close()
			}
		}
	}()
	for {
		var incoming signaling.Received
		select {
		case incoming = <-signaler.Messages:
		case <-ctx.Done():
			return ctx.Err()
		}
		message, peer := incoming.Signal, incoming.Peer
		session := sessions[peer]
		switch message.Type {
		case "offer":
			if message.SDP == nil {
				continue
			}
			queued := []webrtc.ICECandidateInit(nil)
			if session != nil {
				queued = session.queued
				if session.pc != nil {
					_ = session.pc.Close()
				}
			}
			pc, createErr := newPeerConnection(api, configuration, signaler, peer, base)
			if createErr != nil {
				log.Printf("peer=%s create: %v", peer, createErr)
				continue
			}
			session = &peerSession{pc: pc, queued: queued}
			sessions[peer] = session
			if err := pc.SetRemoteDescription(*message.SDP); err != nil {
				log.Printf("peer=%s remote description: %v", peer, err)
				continue
			}
			session.remoteSet = true
			for _, candidate := range session.queued {
				_ = pc.AddICECandidate(candidate)
			}
			session.queued = nil
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				continue
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				continue
			}
			signaler.SendAsync(peer, wire.Signal{Type: "answer", SDP: pc.LocalDescription()})
		case "candidate":
			if message.Candidate == nil {
				continue
			}
			if session == nil {
				session = &peerSession{}
				sessions[peer] = session
			}
			if !session.remoteSet || session.pc == nil {
				session.queued = append(session.queued, *message.Candidate)
			} else {
				_ = session.pc.AddICECandidate(*message.Candidate)
			}
		}
	}
}

// newPeerConnection builds a peer connection that trickles its ICE
// candidates to the browser via the signaler and wires each incoming data
// channel to a fresh tunnel bridge.
func newPeerConnection(api *webrtc.API, configuration webrtc.Configuration, signaler *signaling.Signaler, peer string, base *url.URL) (*webrtc.PeerConnection, error) {
	pc, err := api.NewPeerConnection(configuration)
	if err != nil {
		return nil, err
	}
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			init := candidate.ToJSON()
			signaler.SendAsync(peer, wire.Signal{Type: "candidate", Candidate: &init})
		}
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("peer=%s ICE state=%s", peer, state)
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			pair, pairErr := pc.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
			if pairErr == nil {
				log.Printf("peer=%s DIRECT selected pair local=%s:%d(%s) remote=%s:%d(%s)", peer, pair.Local.Address, pair.Local.Port, pair.Local.Typ, pair.Remote.Address, pair.Remote.Port, pair.Remote.Typ)
			}
		}
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("peer=%s data channel label=%s", peer, dc.Label())
		bridge := tunnel.NewBridge(base, dc)
		dc.OnOpen(func() { log.Printf("peer=%s data channel open", peer) })
		dc.OnMessage(func(message webrtc.DataChannelMessage) { bridge.Handle(message.Data) })
	})
	return pc, nil
}
