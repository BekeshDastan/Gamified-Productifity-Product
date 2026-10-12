package blob

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateBlobRoundTripAndBoundaries(t *testing.T) {
	directory := t.TempDir()
	store, err := NewLocal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	object, err := store.Put(ctx, strings.NewReader("private text"), 100)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.Get(ctx, object.Key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(data) != "private text" {
		t.Fatal("round trip failed")
	}
	for _, key := range []string{"../private", "/absolute", strings.Repeat("0", 64)} {
		if _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
			t.Fatal("invalid or unknown key accepted")
		}
	}
	if _, err := store.Put(ctx, strings.NewReader("too large"), 2); !errors.Is(err, ErrTooLarge) {
		t.Fatal("size limit failed")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("partial upload remained")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Put(cancelled, strings.NewReader("x"), 10); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled upload accepted")
	}
	if verdict, err := (UnavailableScanner{}).Scan(ctx, object); verdict != Pending || !errors.Is(err, ErrScannerUnavailable) {
		t.Fatal("scanner failed open")
	}
	if err := store.Delete(ctx, object.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocal(filepath.Join(t.TempDir(), "public", "blobs")); err == nil {
		t.Fatal("public root accepted")
	}
}
