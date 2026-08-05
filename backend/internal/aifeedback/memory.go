package aifeedback

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu      sync.Mutex
	records []Record
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) Create(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *MemoryStore) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Record(nil), s.records...)
}

var _ Store = (*MemoryStore)(nil)
