package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchHeartbeatCreatesAndRefreshes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goheishamon.packet")

	if err := touchHeartbeat(path); err != nil {
		t.Fatalf("create: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if err := touchHeartbeat(path); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(fi.ModTime()); age > time.Minute {
		t.Errorf("mtime not refreshed, age %v", age)
	}
}

func TestTouchHeartbeatReportsError(t *testing.T) {
	if err := touchHeartbeat(filepath.Join(t.TempDir(), "missing-dir", "f")); err == nil {
		t.Error("expected an error for a missing directory")
	}
}
