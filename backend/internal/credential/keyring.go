package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// KeySize is the master key length AES-256 requires.
const KeySize = 32

// ErrDecrypt is what every failed decryption returns. A wrong key, a tampered
// ciphertext, and a row copied from another user are one error on purpose:
// telling them apart helps whoever is doing it, and helps no one else.
var ErrDecrypt = errors.New("credential: decryption failed")

// Secret is plaintext key material: an API key, an OAuth token document,
// anything that must not be seen outside the caller that asked for it.
//
// String, GoString, and MarshalJSON all redact, so a secret that reaches a log
// line, a %v, or a JSON response by accident prints a placeholder. Reading the
// real bytes has to be deliberate.
type Secret []byte

func (s Secret) String() string { return "[redacted]" }

func (s Secret) GoString() string { return `credential.Secret("[redacted]")` }

// MarshalJSON redacts rather than failing: a handler that accidentally
// serialises a struct holding a Secret should ship a placeholder, not a 500
// that someone fixes by reaching for the raw bytes.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// Keyring holds the master keys. Exactly one version encrypts new data; any
// others are on the ring only so rows written before a rotation can still be
// read.
//
// That is what makes rotation incremental rather than a stop-the-world
// migration. Every stored row names the key version that sealed it, so old and
// new ciphertext coexist happily: put the new key in front, keep the old one
// on the ring, re-encrypt rows in batches, and drop the old key once no row
// still names it. A run interrupted halfway leaves a database that is entirely
// readable, and the next run picks up whatever is left — the key version
// column is the progress marker, so no separate job state can drift from it.
type Keyring struct {
	current int
	keys    map[int]cipher.AEAD
}

// NewKeyring builds a keyring from raw master keys, keyed by version. current
// is the version that encrypts new credentials and must be one of them.
func NewKeyring(current int, keys map[int][]byte) (*Keyring, error) {
	if len(keys) == 0 {
		return nil, errors.New("credential: keyring has no master keys")
	}

	ring := &Keyring{current: current, keys: make(map[int]cipher.AEAD, len(keys))}
	for version, key := range keys {
		if version <= 0 {
			return nil, fmt.Errorf("credential: key version %d must be positive", version)
		}
		// The length of a key is not a secret; the key is, so it stays out of
		// the message.
		if len(key) != KeySize {
			return nil, fmt.Errorf("credential: key version %d is %d bytes, want %d", version, len(key), KeySize)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("credential: key version %d: %w", version, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("credential: key version %d: %w", version, err)
		}
		ring.keys[version] = aead
	}

	if _, ok := ring.keys[current]; !ok {
		return nil, fmt.Errorf("credential: keyring has no key for the current version %d", current)
	}
	return ring, nil
}

// seal encrypts plaintext under the current key and returns the ciphertext,
// its nonce, and the key version that has to be stored beside them.
//
// The nonce is fresh randomness on every call and is never derived from
// anything: GCM loses all of its guarantees the moment a nonce repeats under
// one key. Ninety-six random bits stay far clear of the birthday bound at any
// volume this platform will reach, and a rotation resets the count anyway.
func (k *Keyring) seal(plaintext, aad []byte) (ciphertext, nonce []byte, version int) {
	aead := k.keys[k.current]

	nonce = make([]byte, aead.NonceSize())
	// crypto/rand.Read fills the buffer or crashes the process; since Go 1.24
	// there is no partial-entropy path left to handle, so sealing cannot fail.
	rand.Read(nonce)

	return aead.Seal(nil, nonce, plaintext, aad), nonce, k.current
}

// open decrypts a stored ciphertext with the key version it was sealed under.
func (k *Keyring) open(version int, ciphertext, nonce, aad []byte) (Secret, error) {
	aead, ok := k.keys[version]
	if !ok {
		// Distinct from ErrDecrypt: this one is an operator's problem — a key
		// was retired while rows still named it.
		return nil, fmt.Errorf("credential: keyring has no key for version %d", version)
	}
	if len(nonce) != aead.NonceSize() {
		return nil, ErrDecrypt
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return Secret(plaintext), nil
}

// aad binds a ciphertext to the row it belongs to. Anyone able to write the
// database could otherwise paste another user's ciphertext into their own row
// and have this service decrypt it for them; with the owner and the
// credential's identity authenticated, that forgery simply fails to open.
//
// NUL separates the fields so no two different triples can build the same
// bytes.
func aad(ownerID, provider, name string) []byte {
	return []byte(ownerID + "\x00" + provider + "\x00" + name)
}
