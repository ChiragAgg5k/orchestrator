package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// The workload can write the workspace, markerDir included. A link it leaves
// there must not carry the root sidecar's marker writes and removals out of
// the workspace.
func TestMarker_StaysInsideWorkspace(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, string(markerReady))
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, markerDir)); err != nil {
		t.Fatal(err)
	}

	if err := markerReady.write(ws); err == nil {
		t.Error("write through a linked markerDir succeeded")
	}
	if err := markerReady.clear(ws); err == nil {
		t.Error("clear through a linked markerDir succeeded")
	}
	if markerReady.exists(ws) {
		t.Error("exists followed a linked markerDir")
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("file outside the workspace: %v", err)
	}
	if string(got) != "keep" {
		t.Errorf("file outside the workspace = %q, want it untouched", got)
	}
}
