// Package wire defines the JSON message shapes exchanged between the
// PunchPage host and the browser client, both over the Nostr signaling
// channel and over the WebRTC data channel. The field names and omitempty
// behavior are part of the protocol and must not change.
package wire

import "github.com/pion/webrtc/v4"

// Signal is a WebRTC signaling message (offer, answer, or ICE candidate)
// exchanged through the encrypted Nostr channel.
type Signal struct {
	Type      string                     `json:"type"`
	SDP       *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
}

// Message is a single frame on the WebRTC data channel. One struct covers
// every message type ("request", "response-body", "ws-open", ...); unused
// fields are omitted from the JSON encoding.
type Message struct {
	Type      string              `json:"type"`
	ID        string              `json:"id,omitempty"`
	URL       string              `json:"url,omitempty"`
	Method    string              `json:"method,omitempty"`
	Headers   map[string][]string `json:"headers,omitempty"`
	Data      string              `json:"data,omitempty"`
	Status    int                 `json:"status,omitempty"`
	Error     string              `json:"error,omitempty"`
	Binary    bool                `json:"binary,omitempty"`
	Code      int                 `json:"code,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	Protocols []string            `json:"protocols,omitempty"`
	Protocol  string              `json:"protocol,omitempty"`
	Cookies   []string            `json:"cookies,omitempty"`
}
