// Package push sends Web Push reminders using only the standard library:
// VAPID (RFC 8292) for authentication, aes128gcm (RFC 8188, 8291) for payloads.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"time"
)

var b64 = base64.RawURLEncoding

// ErrGone means the browser dropped the subscription.
var ErrGone = errors.New("subscription gone")

// Keys is a VAPID key pair, base64url encoded.
type Keys struct {
	Public  string
	Private string
}

// GenerateKeys creates a VAPID key pair.
func GenerateKeys() (Keys, error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Keys{}, fmt.Errorf("generate vapid key: %w", err)
	}
	return Keys{Public: b64.EncodeToString(key.PublicKey().Bytes()), Private: b64.EncodeToString(key.Bytes())}, nil
}

// Subscription is the browser's push endpoint.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Encrypt builds an aes128gcm body for the subscription. The ephemeral key and
// salt are parameters so the RFC 8291 test vector can run against it.
func Encrypt(message []byte, p256dh, auth []byte, ephemeral *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	receiver, err := ecdh.P256().NewPublicKey(p256dh)
	if err != nil {
		return nil, fmt.Errorf("receiver key: %w", err)
	}
	secret, err := ephemeral.ECDH(receiver)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}
	sender := ephemeral.PublicKey().Bytes()
	info := append(append([]byte("WebPush: info\x00"), p256dh...), sender...)
	ikm, err := hkdf.Key(sha256.New, secret, auth, string(info), 32)
	if err != nil {
		return nil, fmt.Errorf("derive ikm: %w", err)
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("derive nonce: %w", err)
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	plain := append(bytes.Clone(message), 0x02) // last-record padding delimiter
	var out bytes.Buffer
	out.Write(salt)
	_ = binary.Write(&out, binary.BigEndian, uint32(4096))
	out.WriteByte(byte(len(sender)))
	out.Write(sender)
	out.Write(gcm.Seal(nil, nonce, plain, nil))
	return out.Bytes(), nil
}

// vapidHeader signs the audience of the endpoint.
func vapidHeader(endpoint string, keys Keys, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("endpoint: %w", err)
	}
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": "https://github.com/seals187/habitd",
	})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + b64.EncodeToString(claims)
	raw, err := b64.DecodeString(keys.Private)
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}
	pub, err := b64.DecodeString(keys.Public)
	if err != nil {
		return "", fmt.Errorf("public key: %w", err)
	}
	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:65])},
		D:         new(big.Int).SetBytes(raw),
	}
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return "vapid t=" + unsigned + "." + b64.EncodeToString(sig) + ", k=" + keys.Public, nil
}

// Send delivers a message to one subscription.
func Send(ctx context.Context, client *http.Client, sub Subscription, keys Keys, message []byte) error {
	p256dh, err := b64.DecodeString(sub.P256dh)
	if err != nil {
		return fmt.Errorf("p256dh: %w", err)
	}
	auth, err := b64.DecodeString(sub.Auth)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("ephemeral key: %w", err)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("salt: %w", err)
	}
	body, err := Encrypt(message, p256dh, auth, ephemeral, salt)
	if err != nil {
		return err
	}
	authHeader, err := vapidHeader(sub.Endpoint, keys, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "3600")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 300:
		return fmt.Errorf("push service answered %s", resp.Status)
	}
	return nil
}
