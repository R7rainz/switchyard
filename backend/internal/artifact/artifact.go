// Package artifact stores execution files. LocalStore is the V1 backend; the
// Store interface keeps an S3 implementation a drop-in later without making
// the API or execution packages know where bytes live.
package artifact

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotFound = errors.New("artifact: not found")
var ErrInvalid = errors.New("artifact: invalid")
var ErrTooLarge = errors.New("artifact: too large")

const MaxSize = 32 << 20

type Metadata struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Store interface {
	Put(ctx context.Context, workspaceID, name, contentType string, body io.Reader) (Metadata, error)
	Open(ctx context.Context, workspaceID, id string) (Metadata, io.ReadCloser, error)
	Delete(ctx context.Context, workspaceID, id string) error
}

type LocalStore struct{ root string }

func NewLocalStore(root string) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("artifact: root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("artifact: creating root: %w", err)
	}
	return &LocalStore{root: root}, nil
}

func (s *LocalStore) Put(ctx context.Context, workspaceID, name, contentType string, body io.Reader) (Metadata, error) {
	if !safeSegment(workspaceID) {
		return Metadata{}, fmt.Errorf("%w workspace", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || len(name) > 255 || strings.ContainsAny(name, "\r\n") {
		return Metadata{}, fmt.Errorf("%w name must be one safe filename", ErrInvalid)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	dir := filepath.Join(s.root, workspaceID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Metadata{}, fmt.Errorf("artifact: creating workspace directory: %w", err)
	}
	id := rand.Text()
	dataPath, metaPath := s.paths(workspaceID, id)
	temp, err := os.CreateTemp(dir, ".artifact-*")
	if err != nil {
		return Metadata{}, fmt.Errorf("artifact: creating upload: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	limited := io.LimitReader(body, MaxSize+1)
	size, copyErr := io.Copy(temp, limited)
	if copyErr != nil {
		_ = temp.Close()
		return Metadata{}, fmt.Errorf("artifact: writing upload: %w", copyErr)
	}
	if size > MaxSize {
		_ = temp.Close()
		return Metadata{}, ErrTooLarge
	}
	if err := temp.Close(); err != nil {
		return Metadata{}, fmt.Errorf("artifact: closing upload: %w", err)
	}
	select {
	case <-ctx.Done():
		return Metadata{}, ctx.Err()
	default:
	}
	if err := os.Rename(tempName, dataPath); err != nil {
		return Metadata{}, fmt.Errorf("artifact: committing upload: %w", err)
	}
	metadata := Metadata{ID: id, WorkspaceID: workspaceID, Name: name, ContentType: contentType, Size: size, CreatedAt: time.Now().UTC()}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		_ = os.Remove(dataPath)
		return Metadata{}, fmt.Errorf("artifact: encoding metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, encoded, 0o600); err != nil {
		_ = os.Remove(dataPath)
		return Metadata{}, fmt.Errorf("artifact: writing metadata: %w", err)
	}
	return metadata, nil
}

func (s *LocalStore) Open(_ context.Context, workspaceID, id string) (Metadata, io.ReadCloser, error) {
	if !safeSegment(workspaceID) || !safeSegment(id) {
		return Metadata{}, nil, ErrNotFound
	}
	metadataBytes, err := os.ReadFile(s.metaPath(workspaceID, id))
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, nil, ErrNotFound
	}
	if err != nil {
		return Metadata{}, nil, fmt.Errorf("artifact: reading metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return Metadata{}, nil, fmt.Errorf("artifact: decoding metadata: %w", err)
	}
	body, err := os.Open(s.dataPath(workspaceID, id))
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, nil, ErrNotFound
	}
	if err != nil {
		return Metadata{}, nil, fmt.Errorf("artifact: opening data: %w", err)
	}
	return metadata, body, nil
}

func (s *LocalStore) Delete(_ context.Context, workspaceID, id string) error {
	if !safeSegment(workspaceID) || !safeSegment(id) {
		return ErrNotFound
	}
	if err := os.Remove(s.metaPath(workspaceID, id)); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("artifact: deleting metadata: %w", err)
	}
	if err := os.Remove(s.dataPath(workspaceID, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("artifact: deleting data: %w", err)
	}
	return nil
}

func (s *LocalStore) paths(workspaceID, id string) (string, string) {
	return s.dataPath(workspaceID, id), s.metaPath(workspaceID, id)
}

func (s *LocalStore) dataPath(workspaceID, id string) string {
	return filepath.Join(s.root, workspaceID, id+".data")
}
func (s *LocalStore) metaPath(workspaceID, id string) string {
	return filepath.Join(s.root, workspaceID, id+".json")
}

func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

var _ Store = (*LocalStore)(nil)
