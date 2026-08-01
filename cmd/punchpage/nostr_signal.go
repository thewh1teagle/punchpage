package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	nostr "github.com/nbd-wtf/go-nostr"
)

const signalKind = 24242

type signalPayload struct {
	Role   string `json:"role"`
	Peer   string `json:"peer"`
	Signal signal `json:"signal"`
}

type receivedSignal struct {
	Peer   string
	Signal signal
}

type nostrSignaler struct {
	ctx      context.Context
	pool     *nostr.SimplePool
	relays   []string
	room     string
	key      []byte
	secret   string
	pubkey   string
	messages chan receivedSignal
}

func newNostrSignaler(ctx context.Context, relays []string, room string, key []byte) (*nostrSignaler, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("signaling key must be 32 bytes")
	}
	secret := nostr.GeneratePrivateKey()
	pubkey, err := nostr.GetPublicKey(secret)
	if err != nil {
		return nil, err
	}
	client := &nostrSignaler{
		ctx:      ctx,
		pool:     nostr.NewSimplePool(ctx),
		relays:   relays,
		room:     room,
		key:      append([]byte(nil), key...),
		secret:   secret,
		pubkey:   pubkey,
		messages: make(chan receivedSignal, 128),
	}
	client.subscribe()
	return client, nil
}

func (s *nostrSignaler) subscribe() {
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
			plaintext, err := decryptSignal(s.key, event.Content)
			if err != nil {
				continue
			}
			var payload signalPayload
			if json.Unmarshal(plaintext, &payload) != nil || payload.Role != "browser" || payload.Peer == "" {
				continue
			}
			select {
			case s.messages <- receivedSignal{Peer: payload.Peer, Signal: payload.Signal}:
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *nostrSignaler) send(peer string, message signal) error {
	payload, err := json.Marshal(signalPayload{Role: "host", Peer: peer, Signal: message})
	if err != nil {
		return err
	}
	content, err := encryptSignal(s.key, payload)
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
		return err
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

func (s *nostrSignaler) sendAsync(peer string, message signal) {
	go func() {
		if err := s.send(peer, message); err != nil {
			log.Printf("peer=%s publish signal: %v", peer, err)
		}
	}()
}

func encryptSignal(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptSignal(key []byte, encoded string) ([]byte, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("encrypted signaling message is too short")
	}
	return aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
}
