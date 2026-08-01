package signaling

import (
	"bytes"
	"testing"
)

func TestSignalEncryptionRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plaintext := []byte(`{"type":"offer"}`)
	encoded, err := encrypt(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decrypt(key, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plaintext) {
		t.Fatalf("round trip changed plaintext: %q", decoded)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	encoded, err := encrypt(bytes.Repeat([]byte{7}, 32), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decrypt(bytes.Repeat([]byte{8}, 32), encoded); err == nil {
		t.Fatal("expected authentication failure with the wrong key")
	}
}
