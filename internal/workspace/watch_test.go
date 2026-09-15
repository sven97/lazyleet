package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWatcherCloseUnblocksPendingReader guards against the watcher leaking a
// goroutine forever: a caller blocked on <-w.Events must wake up (with
// ok=false) once Close returns, instead of hanging until the process exits.
func TestWatcherCloseUnblocksPendingReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "solution.py")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan bool, 1)
	go func() {
		_, ok := <-w.Events
		done <- ok
	}()

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected Events to be closed (ok=false), got a value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reader blocked on <-w.Events after Close: goroutine leak")
	}
}
