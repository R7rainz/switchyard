package credential

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// MemoryStore is a Store that keeps records in a map. It exists so the service
// can be exercised without a database; the SQL store replaces it, and both must
// behave the same way about ownership and replacement.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]Record)}
}

// key is the unique triple the SQL table constrains on, flattened.
func (m *MemoryStore) key(ownerID, provider, name string) string {
	return string(aad(ownerID, provider, name))
}

func (m *MemoryStore) Save(_ context.Context, record Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.key(record.OwnerID, record.Provider, record.Name)
	if existing, ok := m.records[key]; ok {
		// What "on conflict do update" leaves alone.
		record.ID, record.CreatedAt = existing.ID, existing.CreatedAt
	}
	m.records[key] = record
	return nil
}

func (m *MemoryStore) Get(_ context.Context, ownerID, provider, name string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, ok := m.records[m.key(ownerID, provider, name)]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

func (m *MemoryStore) List(_ context.Context, ownerID string) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var found []Record
	for _, record := range m.records {
		if record.OwnerID == ownerID {
			found = append(found, record)
		}
	}
	sortRecords(found)
	return found, nil
}

func (m *MemoryStore) Delete(_ context.Context, ownerID, provider, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.key(ownerID, provider, name)
	if _, ok := m.records[key]; !ok {
		return ErrNotFound
	}
	delete(m.records, key)
	return nil
}

func (m *MemoryStore) Stale(_ context.Context, keyVersion, limit int) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var found []Record
	for _, record := range m.records {
		if record.KeyVersion != keyVersion {
			found = append(found, record)
		}
	}
	// Map iteration order is random; sorting keeps repeated rotation batches
	// from returning the same rows in a different order.
	sortRecords(found)
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

func sortRecords(records []Record) {
	slices.SortFunc(records, func(a, b Record) int {
		return strings.Compare(
			a.OwnerID+"\x00"+a.Provider+"\x00"+a.Name,
			b.OwnerID+"\x00"+b.Provider+"\x00"+b.Name,
		)
	})
}
