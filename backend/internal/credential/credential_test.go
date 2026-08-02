package credential

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	alice    = "user-alice"
	bob      = "user-bob"
	provider = "github"
	name     = "default"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	rand.Read(key)
	return key
}

// newService wires a service over a fresh in-memory store, sealing under
// version 1.
func newService(t *testing.T) (*Service, *MemoryStore, []byte) {
	t.Helper()
	key := testKey(t)
	ring, err := NewKeyring(1, map[int][]byte{1: key})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	store := NewMemoryStore()
	return NewService(store, ring), store, key
}

func TestPutGetRoundTrip(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()
	secret := Secret("ghp_averyrealtoken")

	if err := service.Put(ctx, alice, provider, name, secret); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := service.Get(ctx, alice, provider, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(secret) {
		t.Errorf("Get returned %d bytes that do not match what was stored", len(got))
	}
}

// An OAuth token is stored the same way anything else is: as bytes.
func TestPutGetOAuthTokenDocument(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()
	token := `{"access_token":"at-1","refresh_token":"rt-1","expires_at":"2026-01-01T00:00:00Z"}`

	if err := service.Put(ctx, alice, "slack", "workspace", Secret(token)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := service.Get(ctx, alice, "slack", "workspace")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != token {
		t.Error("the OAuth document did not survive the round trip")
	}
}

func TestStoredRecordHoldsNoPlaintext(t *testing.T) {
	service, store, _ := newService(t)
	ctx := context.Background()
	secret := Secret("ghp_averyrealtoken")

	if err := service.Put(ctx, alice, provider, name, secret); err != nil {
		t.Fatalf("Put: %v", err)
	}

	record, err := store.Get(ctx, alice, provider, name)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if strings.Contains(string(record.Ciphertext), string(secret)) {
		t.Error("the ciphertext contains the plaintext")
	}
	if record.KeyVersion != 1 {
		t.Errorf("KeyVersion = %d, want 1", record.KeyVersion)
	}
	if record.ID == "" || record.CreatedAt.IsZero() {
		t.Errorf("record is missing an id or a timestamp: %+v", record)
	}
	// GCM appends a 16-byte tag, so the ciphertext is never the plaintext
	// length, and the nonce has to be stored to decrypt at all.
	if len(record.Nonce) != 12 {
		t.Errorf("nonce is %d bytes, want 12", len(record.Nonce))
	}
}

func TestPutReplacesInPlace(t *testing.T) {
	service, store, _ := newService(t)
	ctx := context.Background()

	if err := service.Put(ctx, alice, provider, name, Secret("first")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	first, _ := store.Get(ctx, alice, provider, name)

	if err := service.Put(ctx, alice, provider, name, Secret("second")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	second, _ := store.Get(ctx, alice, provider, name)

	if second.ID != first.ID {
		t.Errorf("replacing the secret changed the row id: %q -> %q", first.ID, second.ID)
	}
	got, err := service.Get(ctx, alice, provider, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Get returned the stale secret")
	}

	records, err := service.List(ctx, alice)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("List returned %d records, want 1", len(records))
	}
}

func TestPutRejectsIncompleteInput(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()

	tests := []struct {
		name                     string
		owner, provider, credent string
		secret                   Secret
	}{
		{"no owner", "", provider, name, Secret("s")},
		{"no provider", alice, "", name, Secret("s")},
		{"no name", alice, provider, "", Secret("s")},
		{"empty secret", alice, provider, name, Secret("")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := service.Put(ctx, tc.owner, tc.provider, tc.credent, tc.secret); err == nil {
				t.Error("Put accepted it")
			}
		})
	}
}

// Ownership is the whole permission model, so this is the test that matters:
// Bob asking for Alice's credential gets nothing, not hers.
func TestOneOwnerCannotReadAnother(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()

	if err := service.Put(ctx, alice, provider, name, Secret("alice-token")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := service.Get(ctx, bob, provider, name); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get for the wrong owner returned %v, want ErrNotFound", err)
	}
	if err := service.Delete(ctx, bob, provider, name); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete for the wrong owner returned %v, want ErrNotFound", err)
	}

	records, err := service.List(ctx, bob)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("List returned %d of another owner's records", len(records))
	}

	// Alice's credential is untouched by any of that.
	if _, err := service.Get(ctx, alice, provider, name); err != nil {
		t.Errorf("Get for the owner: %v", err)
	}
}

// A store that ignores its owner predicate must not turn into a leak: the owner
// authenticates the ciphertext, so the wrong one cannot open it.
func TestOwnerBindsTheCiphertext(t *testing.T) {
	service, store, _ := newService(t)
	ctx := context.Background()

	if err := service.Put(ctx, alice, provider, name, Secret("alice-token")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Bob copies Alice's row onto his own, which is what a compromised or
	// simply buggy store amounts to.
	stolen, _ := store.Get(ctx, alice, provider, name)
	stolen.OwnerID = bob
	if err := store.Save(ctx, stolen); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	if _, err := service.Get(ctx, bob, provider, name); !errors.Is(err, ErrDecrypt) {
		t.Errorf("Get on a stolen row returned %v, want ErrDecrypt", err)
	}
}

func TestDelete(t *testing.T) {
	service, _, _ := newService(t)
	ctx := context.Background()

	if err := service.Put(ctx, alice, provider, name, Secret("token")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := service.Delete(ctx, alice, provider, name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := service.Get(ctx, alice, provider, name); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete returned %v, want ErrNotFound", err)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	service, _, _ := newService(t)
	if _, err := service.Get(context.Background(), alice, provider, "nothing-here"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get returned %v, want ErrNotFound", err)
	}
}

func TestDecryptFailsWithTheWrongKey(t *testing.T) {
	service, store, _ := newService(t)
	ctx := context.Background()

	if err := service.Put(ctx, alice, provider, name, Secret("token")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Same version number, different key material: the row is unreadable, and
	// says only that.
	other, err := NewKeyring(1, map[int][]byte{1: testKey(t)})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	impostor := NewService(store, other)

	if _, err := impostor.Get(ctx, alice, provider, name); !errors.Is(err, ErrDecrypt) {
		t.Errorf("Get with the wrong key returned %v, want ErrDecrypt", err)
	}
}

func TestTamperedCiphertextIsRejected(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		damage func(*Record)
	}{
		{"flipped bit in the ciphertext", func(r *Record) { r.Ciphertext[0] ^= 0x01 }},
		{"flipped bit in the tag", func(r *Record) { r.Ciphertext[len(r.Ciphertext)-1] ^= 0x01 }},
		{"truncated ciphertext", func(r *Record) { r.Ciphertext = r.Ciphertext[:len(r.Ciphertext)-1] }},
		{"different nonce", func(r *Record) { r.Nonce[0] ^= 0x01 }},
		{"truncated nonce", func(r *Record) { r.Nonce = r.Nonce[:len(r.Nonce)-1] }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, store, _ := newService(t)
			if err := service.Put(ctx, alice, provider, name, Secret("token")); err != nil {
				t.Fatalf("Put: %v", err)
			}

			record, _ := store.Get(ctx, alice, provider, name)
			// Only the ciphertext and nonce are damaged, so the row keeps its
			// identity and the service asks for exactly this one back.
			tc.damage(&record)
			if err := store.Save(ctx, record); err != nil {
				t.Fatalf("store.Save: %v", err)
			}

			if _, err := service.Get(ctx, alice, provider, name); !errors.Is(err, ErrDecrypt) {
				t.Errorf("Get returned %v, want ErrDecrypt", err)
			}
		})
	}
}

// Reusing a nonce under one key breaks GCM outright, so this is checked rather
// than assumed.
func TestNoncesDifferAcrossEncryptions(t *testing.T) {
	service, store, _ := newService(t)
	ctx := context.Background()
	secret := Secret("the same secret every time")

	seen := make(map[string]bool)
	for i := range 100 {
		credentialName := fmt.Sprintf("key-%d", i)
		if err := service.Put(ctx, alice, provider, credentialName, secret); err != nil {
			t.Fatalf("Put: %v", err)
		}
		record, _ := store.Get(ctx, alice, provider, credentialName)

		if seen[string(record.Nonce)] {
			t.Fatalf("nonce repeated after %d encryptions", i+1)
		}
		seen[string(record.Nonce)] = true

		// Identical plaintext must not produce identical ciphertext either.
		if seen[string(record.Ciphertext)] {
			t.Fatalf("ciphertext repeated after %d encryptions", i+1)
		}
		seen[string(record.Ciphertext)] = true
	}
}

func TestRotateReEncryptsUnderTheNewKey(t *testing.T) {
	ctx := context.Background()
	oldKey, newKey := testKey(t), testKey(t)

	oldRing, err := NewKeyring(1, map[int][]byte{1: oldKey})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	store := NewMemoryStore()
	before := NewService(store, oldRing)

	secrets := map[string]string{"a": "secret-a", "b": "secret-b", "c": "secret-c"}
	for credentialName, secret := range secrets {
		if err := before.Put(ctx, alice, provider, credentialName, Secret(secret)); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	// Both keys on the ring, version 2 in front: this is the state a rotation
	// runs in.
	bothRings, err := NewKeyring(2, map[int][]byte{1: oldKey, 2: newKey})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	after := NewService(store, bothRings)

	// A batch smaller than the backlog, so the run is genuinely partial.
	rotated, err := after.Rotate(ctx, 2)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated != 2 {
		t.Fatalf("Rotate reported %d rows, want 2", rotated)
	}

	// Mid-rotation: every credential still reads, whichever key holds it.
	versions := map[int]int{}
	for credentialName, want := range secrets {
		got, err := after.Get(ctx, alice, provider, credentialName)
		if err != nil {
			t.Fatalf("Get(%s) mid-rotation: %v", credentialName, err)
		}
		if string(got) != want {
			t.Errorf("Get(%s) = %q, want %q", credentialName, got, want)
		}
		record, _ := store.Get(ctx, alice, provider, credentialName)
		versions[record.KeyVersion]++
	}
	if versions[1] != 1 || versions[2] != 2 {
		t.Errorf("key versions after a partial rotation = %v, want one row on 1 and two on 2", versions)
	}

	// Resuming picks up exactly what is left, and a third call finds nothing —
	// which is the signal that the old key can go.
	rotated, err = after.Rotate(ctx, 2)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated != 1 {
		t.Errorf("resumed rotation reported %d rows, want 1", rotated)
	}
	rotated, err = after.Rotate(ctx, 2)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated != 0 {
		t.Errorf("a finished rotation reported %d rows, want 0", rotated)
	}

	// With the old key retired, everything still opens.
	newOnly, err := NewKeyring(2, map[int][]byte{2: newKey})
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	retired := NewService(store, newOnly)
	for credentialName, want := range secrets {
		got, err := retired.Get(ctx, alice, provider, credentialName)
		if err != nil {
			t.Fatalf("Get(%s) after rotation: %v", credentialName, err)
		}
		if string(got) != want {
			t.Errorf("Get(%s) = %q, want %q", credentialName, got, want)
		}
	}
}

// Retiring a key too early is an operator error, and it has to read like one
// rather than like a corrupt row.
func TestGetWithARetiredKeyVersion(t *testing.T) {
	ctx := context.Background()
	oldKey := testKey(t)

	oldRing, _ := NewKeyring(1, map[int][]byte{1: oldKey})
	store := NewMemoryStore()
	if err := NewService(store, oldRing).Put(ctx, alice, provider, name, Secret("token")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	newOnly, _ := NewKeyring(2, map[int][]byte{2: testKey(t)})
	_, err := NewService(store, newOnly).Get(ctx, alice, provider, name)
	if err == nil || !strings.Contains(err.Error(), "no key for version 1") {
		t.Errorf("Get = %v, want it to name the missing key version", err)
	}
	if _, err := NewService(store, newOnly).Rotate(ctx, 10); err == nil {
		t.Error("Rotate succeeded without the key the rows were sealed under")
	}
}

func TestNewKeyringRejects(t *testing.T) {
	short := make([]byte, KeySize-1)
	good := make([]byte, KeySize)

	tests := []struct {
		name    string
		current int
		keys    map[int][]byte
		want    string
	}{
		{"no keys", 1, nil, "no master keys"},
		{"short key", 1, map[int][]byte{1: short}, "want 32"},
		{"zero version", 0, map[int][]byte{0: good}, "must be positive"},
		{"current not on the ring", 3, map[int][]byte{1: good}, "current version 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKeyring(tc.current, tc.keys)
			if err == nil {
				t.Fatal("NewKeyring accepted it")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Key material must not appear in an error either: the message says what is
// wrong with the key, never what it is.
func TestKeyringErrorsCarryNoKeyMaterial(t *testing.T) {
	key := []byte("this-key-is-the-wrong-length")
	_, err := NewKeyring(1, map[int][]byte{1: key})
	if err == nil {
		t.Fatal("NewKeyring accepted a short key")
	}
	if strings.Contains(err.Error(), string(key)) {
		t.Errorf("error leaks the key: %v", err)
	}
}

// The redaction is the last line of defence for a secret that reaches a log
// line or a response body by accident.
func TestSecretRedactsItself(t *testing.T) {
	secret := Secret("ghp_averyrealtoken")

	for _, got := range []string{
		secret.String(),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%s", secret),
		fmt.Sprintf("%q", secret),
		fmt.Sprintf("%x", secret),
		fmt.Sprintf("%#v", secret),
		fmt.Sprintf("%v", struct{ Token Secret }{secret}),
		fmt.Sprintf("%+v", map[string]Secret{"token": secret}),
	} {
		if strings.Contains(got, "ghp_") {
			t.Errorf("formatted secret leaked the plaintext: %s", got)
		}
	}

	encoded, err := json.Marshal(struct {
		Token Secret `json:"token"`
	}{secret})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "ghp_") {
		t.Errorf("JSON leaked the plaintext: %s", encoded)
	}
}
