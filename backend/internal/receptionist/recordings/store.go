package recordings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrNotFound = errors.New("recording not found")

type TranscriptLine struct {
	Role string `json:"role"`
	Text string `json:"text"`
	TS   int64  `json:"ts"`
}

type Meta struct {
	ID         string           `json:"id"`
	RecorderID string           `json:"recorder_id"`
	SessionID  string           `json:"session_id,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	DurationMS int              `json:"duration_ms"`
	AudioMIME  string           `json:"audio_mime"`
	Transcript []TranscriptLine `json:"transcript,omitempty"`
}

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("recordings directory is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve recordings directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create recordings directory: %w", err)
	}
	return &Store{root: abs}, nil
}

func SafeID(id string) error {
	if id == "" || len(id) > 128 {
		return errors.New("id must contain between 1 and 128 characters")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return errors.New("id contains invalid characters")
	}
	return nil
}

func (s *Store) Save(meta Meta, audio io.Reader) error {
	if err := SafeID(meta.RecorderID); err != nil {
		return fmt.Errorf("invalid recorder id: %w", err)
	}
	if err := SafeID(meta.ID); err != nil {
		return fmt.Errorf("invalid recording id: %w", err)
	}
	if audio == nil {
		return errors.New("audio is required")
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}

	dir := s.recorderDir(meta.RecorderID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create recorder directory: %w", err)
	}

	audioPath := filepath.Join(dir, meta.ID+audioExtension(meta.AudioMIME))
	if err := writeAtomic(audioPath, audio, 0o640); err != nil {
		return fmt.Errorf("save recording audio: %w", err)
	}

	metadata, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.Remove(audioPath)
		return fmt.Errorf("encode recording metadata: %w", err)
	}
	if err := writeAtomic(s.metaPath(meta.RecorderID, meta.ID), strings.NewReader(string(metadata)), 0o640); err != nil {
		_ = os.Remove(audioPath)
		return fmt.Errorf("save recording metadata: %w", err)
	}
	return nil
}

func (s *Store) List(recorderID string) ([]Meta, error) {
	if err := SafeID(recorderID); err != nil {
		return nil, fmt.Errorf("invalid recorder id: %w", err)
	}
	entries, err := os.ReadDir(s.recorderDir(recorderID))
	if errors.Is(err, os.ErrNotExist) {
		return []Meta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}

	items := make([]Meta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		meta, err := s.Get(recorderID, id)
		if err != nil {
			return nil, fmt.Errorf("read recording %q: %w", id, err)
		}
		items = append(items, *meta)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) Get(recorderID, id string) (*Meta, error) {
	if err := SafeID(recorderID); err != nil {
		return nil, fmt.Errorf("invalid recorder id: %w", err)
	}
	if err := SafeID(id); err != nil {
		return nil, fmt.Errorf("invalid recording id: %w", err)
	}
	b, err := os.ReadFile(s.metaPath(recorderID, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read recording metadata: %w", err)
	}
	var meta Meta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("decode recording metadata: %w", err)
	}
	if meta.ID != id || meta.RecorderID != recorderID {
		return nil, errors.New("recording metadata does not match requested owner")
	}
	return &meta, nil
}

func (s *Store) AudioPath(recorderID, id string) (string, error) {
	meta, err := s.Get(recorderID, id)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.recorderDir(recorderID), id+audioExtension(meta.AudioMIME))
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("stat recording audio: %w", err)
	}
	return path, nil
}

func (s *Store) Delete(recorderID, id string) error {
	meta, err := s.Get(recorderID, id)
	if err != nil {
		return err
	}
	audioPath := filepath.Join(s.recorderDir(recorderID), id+audioExtension(meta.AudioMIME))
	if err := os.Remove(audioPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete recording audio: %w", err)
	}
	if err := os.Remove(s.metaPath(recorderID, id)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("delete recording metadata: %w", err)
	}
	return nil
}

func (s *Store) DeleteAll(recorderID string) (int, error) {
	items, err := s.List(recorderID)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(s.recorderDir(recorderID)); err != nil {
		return 0, fmt.Errorf("delete recorder directory: %w", err)
	}
	return len(items), nil
}

func (s *Store) recorderDir(recorderID string) string {
	return filepath.Join(s.root, recorderID)
}

func (s *Store) metaPath(recorderID, id string) string {
	return filepath.Join(s.recorderDir(recorderID), id+".json")
}

func audioExtension(mime string) string {
	if strings.Contains(strings.ToLower(mime), "wav") {
		return ".wav"
	}
	return ".webm"
}

func writeAtomic(path string, src io.Reader, mode os.FileMode) (retErr error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recording-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, src); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
