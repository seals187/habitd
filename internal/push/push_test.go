package push

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
	"time"
)

// TestEncryptRFC8291 is the worked example from RFC 8291, appendix A.
func TestEncryptRFC8291(t *testing.T) {
	decode := func(s string) []byte {
		b, err := b64.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ephemeral, err := ecdh.P256().NewPrivateKey(decode("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Encrypt(
		[]byte("When I grow up, I want to be a watermelon"),
		decode("BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		decode("BTBZMqHH6r4Tts7J_aSIgg"),
		ephemeral,
		decode("DGv6ra1nlYgDCS1FRnbzlw"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Fatalf("got %s", b64.EncodeToString(got))
	}
}

func TestVapidHeaderVerifies(t *testing.T) {
	keys, err := GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	header, err := vapidHeader("https://push.example.net/send/abc", keys, time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	jwt := strings.TrimPrefix(strings.Split(header, ", k=")[0], "vapid t=")
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt: %s", jwt)
	}
	claims, _ := b64.DecodeString(parts[1])
	if !strings.Contains(string(claims), `"aud":"https://push.example.net"`) {
		t.Fatalf("claims: %s", claims)
	}
	sig, _ := b64.DecodeString(parts[2])
	pub, _ := b64.DecodeString(keys.Public)
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pub)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
}
