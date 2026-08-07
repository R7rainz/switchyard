package artifact

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalStoreRoundTripAndDelete(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Put(context.Background(), "ws-1", "report.txt", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if metadata.Size != 5 || metadata.ID == "" {
		t.Fatalf("metadata = %+v", metadata)
	}
	got, body, err := store.Open(context.Background(), "ws-1", metadata.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	content, _ := io.ReadAll(body)
	_ = body.Close()
	if got.Name != "report.txt" || string(content) != "hello" {
		t.Fatalf("opened = %+v, %q", got, content)
	}
	if err := store.Delete(context.Background(), "ws-1", metadata.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := store.Open(context.Background(), "ws-1", metadata.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open deleted: %v", err)
	}
}

func TestLocalStoreRejectsTraversalAndOversize(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "../other", "file", "", strings.NewReader("x")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("workspace traversal: %v", err)
	}
	if _, err := store.Put(context.Background(), "ws", "../file", "", strings.NewReader("x")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("name traversal: %v", err)
	}
	if _, err := store.Put(context.Background(), "ws", "file", "", io.LimitReader(strings.NewReader(strings.Repeat("x", MaxSize+1)), MaxSize+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize: %v", err)
	}
}
