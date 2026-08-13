package secretbox

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func newKEK(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func TestSealOpen_RoundTrip(t *testing.T) {
	kek := newKEK(t)
	for _, pt := range [][]byte{
		[]byte("totp-secret-JBSWY3DPEHPK3PXP"),
		[]byte(""), // empty plaintext must still round-trip
		bytes.Repeat([]byte{0x00}, 1024),
	} {
		ct, err := Seal(kek, pt)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		got, err := Open(kek, ct)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("round-trip mismatch: got %q want %q", got, pt)
		}
	}
}

func TestSeal_RejectsBadKeySize(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := Seal(make([]byte, n), []byte("x")); err != ErrKeySize {
			t.Errorf("Seal with %d-byte key: err = %v, want ErrKeySize", n, err)
		}
	}
}

func TestOpen_RejectsBadKeySize(t *testing.T) {
	if _, err := Open(make([]byte, 16), make([]byte, 40)); err != ErrKeySize {
		t.Errorf("Open with 16-byte key: err = %v, want ErrKeySize", err)
	}
}

func TestOpen_WrongKeyFails(t *testing.T) {
	ct, err := Seal(newKEK(t), []byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, err := Open(newKEK(t), ct); err == nil {
		t.Fatal("Open with the wrong key must fail authentication, got nil error")
	}
}

func TestOpen_TamperedCiphertextFails(t *testing.T) {
	kek := newKEK(t)
	ct, err := Seal(kek, []byte("secret payload"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	ct[len(ct)-1] ^= 0xFF // flip a bit in the GCM tag
	if _, err := Open(kek, ct); err == nil {
		t.Fatal("Open must reject tampered ciphertext (GCM auth), got nil error")
	}
}

func TestOpen_ShortCiphertextFails(t *testing.T) {
	kek := newKEK(t)
	if _, err := Open(kek, []byte("tiny")); err == nil {
		t.Fatal("Open must reject a ciphertext shorter than the nonce")
	}
}

// TestSeal_NonDeterministic confirms a fresh random nonce per call: sealing the
// same plaintext twice must not produce identical ciphertext.
func TestSeal_NonDeterministic(t *testing.T) {
	kek := newKEK(t)
	pt := []byte("same plaintext")
	a, _ := Seal(kek, pt)
	b, _ := Seal(kek, pt)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext are identical — nonce not random")
	}
}
