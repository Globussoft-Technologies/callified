package recordings

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStoreLifecycle(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	older := Meta{
		ID: "old", RecorderID: "phone_919999999999", SessionID: "call-1",
		CreatedAt: time.Unix(10, 0).UTC(), AudioMIME: "audio/webm",
		Transcript: []TranscriptLine{{Role: "user", Text: "hello", TS: 10}},
	}
	newer := Meta{
		ID: "new", RecorderID: older.RecorderID, CreatedAt: time.Unix(20, 0).UTC(),
		AudioMIME: "audio/wav",
	}
	if err := store.Save(older, strings.NewReader("old audio")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(newer, strings.NewReader("new audio")); err != nil {
		t.Fatal(err)
	}

	items, err := store.List(older.RecorderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("unexpected list order: %#v", items)
	}
	path, err := store.AudioPath(older.RecorderID, newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filepathExt := strings.ToLower(path[len(path)-4:]); filepathExt != ".wav" {
		t.Fatalf("expected wav path, got %q", path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new audio" {
		t.Fatalf("unexpected audio: %q, %v", got, err)
	}

	if err := store.Delete(older.RecorderID, older.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(older.RecorderID, older.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if count, err := store.DeleteAll(older.RecorderID); err != nil || count != 1 {
		t.Fatalf("DeleteAll() = %d, %v", count, err)
	}
}

func TestSafeIDRejectsTraversal(t *testing.T) {
	for _, id := range []string{"", "../other", "a/b", "a.b", "has space"} {
		if err := SafeID(id); err == nil {
			t.Errorf("SafeID(%q) unexpectedly succeeded", id)
		}
	}
	for _, id := range []string{"abc", "phone_919999999999", "call-id"} {
		if err := SafeID(id); err != nil {
			t.Errorf("SafeID(%q) failed: %v", id, err)
		}
	}
}

func TestStoreRejectsCrossRecorderLookup(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta := Meta{ID: "recording", RecorderID: "owner", AudioMIME: "audio/wav"}
	if err := store.Save(meta, strings.NewReader("audio")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("someone-else", meta.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cross-recorder lookup to return ErrNotFound, got %v", err)
	}
}
