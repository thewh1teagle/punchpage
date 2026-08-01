// Package signaling exchanges encrypted WebRTC signaling messages between
// the PunchPage host and browsers through public Nostr relays. Payloads are
// sealed with AES-256-GCM using a shared key that only ever travels in the
// share URL fragment.
package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	nostr "github.com/nbd-wtf/go-nostr"

	"github.com/thewh1teagle/punchpage/internal/wire"
)

// signalKind is the Nostr event kind used for PunchPage signaling events.
const signalKind = 24242

// payload is the plaintext JSON carried inside an encrypted Nostr event.
type payload struct {
	Role   string      `json:"role"`
	Peer   string      `json:"peer"`
	Signal wire.Signal `json:"signal"`
}

// Received is a decrypted signaling message from a browser peer.
type Received struct {
	Peer   string
	Signal wire.Signal
}

// Signaler publishes and receives encrypted signaling events for one room
// across a set of Nostr relays.
type Signaler struct {
	ctx    context.Context
	pool   *nostr.SimplePool
	relays []string
	room   string
	key    []byte
	secret string
	pubkey string

	// Messages delivers decrypted signals sent by browser peers.
	Messages chan Received
}

// New creates a Signaler with a fresh Nostr identity and starts listening on
// the given relays for browser signals addressed to the room. The key must be
// exactly 32 bytes. The subscription lives until ctx is canceled.
func New(ctx context.Context, relays []string, room string, key []byte) (*Signaler, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("signaling key must be 32 bytes")
	}
	secret := nostr.GeneratePrivateKey()
	pubkey, err := nostr.GetPublicKey(secret)
	if err != nil {
		return nil, fmt.Errorf("derive nostr public key: %w", err)
	}
	client := &Signaler{
		ctx:      ctx,
		pool:     nostr.NewSimplePool(ctx),
		relays:   relays,
		room:     room,
		key:      append([]byte(nil), key...),
		secret:   secret,
		pubkey:   pubkey,
		Messages: make(chan Received, 128),
	}
	client.subscribe()
	return client, nil
}

// subscribe starts a goroutine that decrypts incoming room events and
// forwards valid browser signals to Messages, dropping duplicates and events
// from this host's own key.
func (s *Signaler) subscribe() {
	since := nostr.Timestamp(time.Now().Add(-30 * time.Second).Unix())
	events := s.pool.SubscribeMany(s.ctx, s.relays, nostr.Filter{
		Kinds: []int{signalKind},
		Tags:  nostr.TagMap{"d": []string{s.room}},
		Since: &since,
	})
	go func() {
		seen := make(map[string]struct{})
		for relayEvent := range events {
			event := relayEvent.Event
			if event == nil || event.PubKey == s.pubkey {
				continue
			}
			if _, ok := seen[event.ID]; ok {
				continue
			}
			seen[event.ID] = struct{}{}
			plaintext, err := decrypt(s.key, event.Content)
			if err != nil {
				continue
			}
			var decoded payload
			if json.Unmarshal(plaintext, &decoded) != nil || decoded.Role != "browser" || decoded.Peer == "" {
				continue
			}
			select {
			case s.Messages <- Received{Peer: decoded.Peer, Signal: decoded.Signal}:
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// Send encrypts, signs, and publishes a signal addressed to peer. It succeeds
// if at least one relay accepts the event; otherwise it returns the joined
// relay errors.
func (s *Signaler) Send(peer string, message wire.Signal) error {
	plaintext, err := json.Marshal(payload{Role: "host", Peer: peer, Signal: message})
	if err != nil {
		return err
	}
	content, err := encrypt(s.key, plaintext)
	if err != nil {
		return err
	}
	event := nostr.Event{
		PubKey:    s.pubkey,
		CreatedAt: nostr.Now(),
		Kind:      signalKind,
		Tags:      nostr.Tags{{"d", s.room}},
		Content:   content,
	}
	if err := event.Sign(s.secret); err != nil {
		return fmt.Errorf("sign signaling event: %w", err)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	var publishErrors []error
	published := 0
	for result := range s.pool.PublishMany(ctx, s.relays, event) {
		if result.Error == nil {
			published++
			continue
		}
		publishErrors = append(publishErrors, result.Error)
	}
	if published > 0 {
		return nil
	}
	if len(publishErrors) == 0 {
		return errors.New("no signaling relay accepted the event")
	}
	return errors.Join(publishErrors...)
}

// SendAsync publishes a signal in the background, logging any failure.
func (s *Signaler) SendAsync(peer string, message wire.Signal) {
	go func() {
		if err := s.Send(peer, message); err != nil {
			log.Printf("peer=%s publish signal: %v", peer, err)
		}
	}()
}
