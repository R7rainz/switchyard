package credential

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound means no credential with that owner, provider, and name exists.
// A credential belonging to someone else is not found either.
var ErrNotFound = errors.New("credential: not found")

// Record is one stored credential. It carries no plaintext: everything secret
// about it is inside Ciphertext, which only a Keyring can open.
type Record struct {
	ID         string
	OwnerID    string
	Provider   string // the integration the secret is for: "github", "openrouter", ...
	Name       string // distinguishes several credentials for one provider
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Store is the persistence this package needs, declared here where it is
// consumed so the service can be exercised without a database. The SQL
// implementation arrives with the database package.
//
// Ownership is a parameter of every lookup, never an optional filter: an
// implementation must put "ownerId" in the WHERE clause of each of these, so
// returning another user's credential takes a deliberately wrong query rather
// than a forgotten argument.
type Store interface {
	// Save inserts the record, or replaces the one already stored under the
	// same owner, provider, and name — keeping that row's ID and CreatedAt,
	// the way an upsert on the unique triple does.
	Save(ctx context.Context, record Record) error

	// Get returns ownerID's credential for this provider and name, or
	// ErrNotFound.
	Get(ctx context.Context, ownerID, provider, name string) (Record, error)

	// List returns every credential belonging to ownerID.
	List(ctx context.Context, ownerID string) ([]Record, error)

	// Delete removes ownerID's credential for this provider and name, or
	// returns ErrNotFound.
	Delete(ctx context.Context, ownerID, provider, name string) error

	// Stale returns up to limit (which must be positive) records sealed under
	// some key version other than keyVersion, ordered stably so repeated calls
	// make progress. Rotation is the one operation that crosses owners, and
	// the only one: it moves ciphertext between keys and never hands plaintext
	// to anybody.
	Stale(ctx context.Context, keyVersion, limit int) ([]Record, error)
}

// Service is the credential API the rest of Switchyard calls. It owns the
// crypto; the Store owns the rows, and never sees a key.
type Service struct {
	store Store
	keys  *Keyring
	now   func() time.Time
}

func NewService(store Store, keys *Keyring) *Service {
	return &Service{store: store, keys: keys, now: time.Now}
}

// Put encrypts secret under the current master key and stores it for ownerID,
// replacing whatever was held for the same provider and name.
//
// An OAuth token is just a secret with structure: marshal the whole token
// document — access token, refresh token, expiry — and store it as one
// credential. Refreshing it is the integration's job, not this package's; it
// only has to survive a round trip.
func (s *Service) Put(ctx context.Context, ownerID, provider, name string, secret Secret) error {
	if ownerID == "" || provider == "" || name == "" {
		return errors.New("credential: owner, provider, and name are all required")
	}
	if len(secret) == 0 {
		return errors.New("credential: secret is empty")
	}

	ciphertext, nonce, version := s.keys.seal(secret, aad(ownerID, provider, name))

	now := s.now().UTC()
	return s.store.Save(ctx, Record{
		// Used only when this is an insert; a replace keeps the row's own id.
		// rand.Text is 128+ bits of base32, close enough to the text ids Better
		// Auth writes without adding a UUID dependency for it.
		ID:         rand.Text(),
		OwnerID:    ownerID,
		Provider:   provider,
		Name:       name,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		KeyVersion: version,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
}

// Get decrypts ownerID's credential for this provider and name. The plaintext
// lives in the returned Secret and nowhere else — it is not logged, and no
// error below carries it.
func (s *Service) Get(ctx context.Context, ownerID, provider, name string) (Secret, error) {
	record, err := s.store.Get(ctx, ownerID, provider, name)
	if err != nil {
		return nil, err
	}

	// The requested owner binds the ciphertext, not the one the store handed
	// back: if a store ever ignored its owner predicate, the returned row would
	// fail to authenticate instead of quietly decrypting someone else's key.
	return s.keys.open(record.KeyVersion, record.Ciphertext, record.Nonce, aad(ownerID, provider, name))
}

// List returns ownerID's credentials. Records hold ciphertext only, so this is
// the safe listing for a settings page.
func (s *Service) List(ctx context.Context, ownerID string) ([]Record, error) {
	return s.store.List(ctx, ownerID)
}

// Delete removes ownerID's credential for this provider and name.
func (s *Service) Delete(ctx context.Context, ownerID, provider, name string) error {
	return s.store.Delete(ctx, ownerID, provider, name)
}

// Rotate re-encrypts up to limit credentials still sealed under an older key
// and reports how many it rewrote. Call it until it returns 0; that is the
// signal that the previous key can leave the keyring.
//
// Nothing tracks rotation progress except the key version on each row, so a
// run that dies halfway needs no recovery — the rows it finished are done, the
// rest are still readable under the old key, and the next call finds exactly
// what is left. Batching exists for the same reason: a rotation that has to
// complete in one transaction is a rotation nobody runs.
func (s *Service) Rotate(ctx context.Context, limit int) (int, error) {
	records, err := s.store.Stale(ctx, s.keys.current, limit)
	if err != nil {
		return 0, err
	}

	rotated := 0
	for _, record := range records {
		bound := aad(record.OwnerID, record.Provider, record.Name)

		secret, err := s.keys.open(record.KeyVersion, record.Ciphertext, record.Nonce, bound)
		if err != nil {
			// The id names the row an operator has to look at; the secret it
			// failed to open is not in here.
			return rotated, fmt.Errorf("credential: rotating %s: %w", record.ID, err)
		}

		record.Ciphertext, record.Nonce, record.KeyVersion = s.keys.seal(secret, bound)
		record.UpdatedAt = s.now().UTC()
		// Rotation is the one place holding plaintext it does not need to
		// return, so it wipes its copy as soon as the new one is sealed.
		clear(secret)

		if err := s.store.Save(ctx, record); err != nil {
			return rotated, fmt.Errorf("credential: rotating %s: %w", record.ID, err)
		}
		rotated++
	}
	return rotated, nil
}
