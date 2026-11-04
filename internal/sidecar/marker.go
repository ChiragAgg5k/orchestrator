package sidecar

import (
	"errors"
	"fmt"
	"orchestrator/internal/artifact"
	"os"
	"path/filepath"
)

// A marker is the sidecar's crash-safe record that a phase completed. Markers
// live in the shared workspace because that is the only state that survives a
// container restart: a restarted sidecar reads them — alongside the kernel
// mount table for mount adoption — to recover in place instead of redoing
// work a running workload may depend on.
//
// Every marker lives under markerDir, a runner-owned corner of the workspace,
// so an artifact's out path can never collide with runner state. Nothing
// outside this binary reads the paths — probes exec the -check-* flags — so
// the layout ships atomically with the sidecar.
type marker string

const (
	// markerDir is the runner-owned directory inside the shared workspace.
	markerDir = ".sidecar"

	// markerReady gates the worker in the combined flow: pre-job artifacts
	// are processed and mounts are established. Docker health checks and
	// Kubernetes startup probes poll it via -check-ready.
	markerReady marker = "ready"

	// markerMountsReady gates the worker in the split flow: the post sidecar
	// has established every artifact mount. Polled via -check-mounts.
	markerMountsReady marker = "mounts-ready"

	// markerArtifactsComplete records that the pre-job artifact phase
	// finished. A restarted sidecar skips the phase instead of re-fetching
	// files a running workload may be mounted on.
	markerArtifactsComplete marker = "artifacts-complete"
)

// name of m relative to the workspace.
func (m marker) name() string {
	return filepath.Join(markerDir, string(m))
}

// path of m inside workspace.
func (m marker) path(workspace string) string {
	return filepath.Join(workspace, m.name())
}

// exists reports whether m is set in workspace.
func (m marker) exists(workspace string) bool {
	ws, err := os.OpenRoot(workspace)
	if err != nil {
		return false
	}
	defer ws.Close()
	_, err = ws.Stat(m.name())
	return err == nil
}

// write sets m in workspace. Through a root, like every sidecar write there:
// the workload can replace markerDir with a link, and the sidecar is root.
func (m marker) write(workspace string) error {
	ws, err := artifact.OpenWorkspace(workspace)
	if err != nil {
		return err
	}
	defer ws.Close()
	if err := ws.MkdirAll(markerDir, 0o755); err != nil {
		return fmt.Errorf("create marker directory: %w", err)
	}
	if err := ws.WriteFile(m.name(), []byte{}, 0o644); err != nil {
		return fmt.Errorf("write %s marker: %w", m, err)
	}
	return nil
}

// clear removes m from workspace; a marker that was never set is fine.
func (m marker) clear(workspace string) error {
	ws, err := artifact.OpenWorkspace(workspace)
	if err != nil {
		return err
	}
	defer ws.Close()
	if err := ws.Remove(m.name()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear %s marker: %w", m, err)
	}
	return nil
}

// CheckReady reports whether the ready marker exists: pre-job artifacts are
// processed and mounts are established. Docker health checks and Kubernetes
// startup probes poll it to decide when the worker can start.
func CheckReady(sharedVolumePath string) bool {
	return markerReady.exists(sharedVolumePath)
}

// CheckMountsReady reports whether the mounts-ready marker exists. The
// Kubernetes native sidecar writes it after mounting; the worker's startup
// probe waits on it so the worker only starts once its mounts are present.
func CheckMountsReady(sharedVolumePath string) bool {
	return markerMountsReady.exists(sharedVolumePath)
}

// ReadyMarkerPath is where the ready marker lives inside workspace. The
// marker layout is private to this binary; this accessor exists for tests
// that observe the marker from outside it.
func ReadyMarkerPath(workspace string) string {
	return markerReady.path(workspace)
}
