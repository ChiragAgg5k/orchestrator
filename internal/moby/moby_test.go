package moby

import (
	"context"
	"errors"
	"orchestrator/internal/apperrors"
	"orchestrator/internal/volume"
	"slices"
	"testing"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// daemonVersion stands in for a daemon reporting its API version.
type daemonVersion string

func (v daemonVersion) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{APIVersion: string(v)}, nil
}

func TestVolumeMounts(t *testing.T) {
	t.Parallel()
	mounts, err := Mounts(context.Background(), daemonVersion("1.47"), []volume.Volume{
		{Source: "data", Path: "/data", ReadOnly: true},
		{Source: "cache", Path: "/cache", SubPath: "sub"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("got %d mounts, want 2", len(mounts))
	}
	if mounts[0].Type != mount.TypeVolume || mounts[0].Source != "data" || mounts[0].Target != "/data" || !mounts[0].ReadOnly {
		t.Errorf("mount[0] = %+v", mounts[0])
	}
	if mounts[0].VolumeOptions != nil {
		t.Error("mount[0] should have no VolumeOptions without a subPath")
	}
	if mounts[1].VolumeOptions == nil || mounts[1].VolumeOptions.Subpath != "sub" {
		t.Errorf("mount[1] subPath = %+v, want sub", mounts[1].VolumeOptions)
	}
}

// A daemon older than API 1.45 ignores a subPath and mounts the whole volume;
// the request is refused instead of widened.
func TestVolumeMounts_SubPathNeedsAPI145(t *testing.T) {
	t.Parallel()
	vols := []volume.Volume{
		{Source: "data", Path: "/data"},
		{Source: "builds", Path: "/builds", SubPath: "app-1/dep-1"},
	}

	for _, v := range []string{"1.41", "1.44"} {
		if _, err := Mounts(context.Background(), daemonVersion(v), vols); !errors.Is(err, apperrors.ErrValidation) {
			t.Errorf("API %s: Mounts() error = %v, want a validation error", v, err)
		}
	}
	for _, v := range []string{"1.45", "1.51"} {
		if _, err := Mounts(context.Background(), daemonVersion(v), vols); err != nil {
			t.Errorf("API %s: Mounts() error = %v", v, err)
		}
	}
	// Without a subPath there is nothing an old daemon could widen.
	if _, err := Mounts(context.Background(), daemonVersion("1.41"), vols[:1]); err != nil {
		t.Errorf("API 1.41 without subPath: Mounts() error = %v", err)
	}
}

// A user's command: no raw sockets on the shared bridge, no privilege gain
// through setuid binaries, a process cap, and the resources it asked for.
func TestWorkloadHostConfig_Hardened(t *testing.T) {
	t.Parallel()
	hc := WorkloadHostConfig(nil, 0.5, 256, nil)
	if !slices.Contains(hc.CapDrop, "NET_RAW") || !slices.Contains(hc.SecurityOpt, "no-new-privileges") || hc.Privileged {
		t.Errorf("CapDrop = %v, SecurityOpt = %v, Privileged = %v; want NET_RAW dropped and no-new-privileges", hc.CapDrop, hc.SecurityOpt, hc.Privileged)
	}
	if hc.PidsLimit == nil || *hc.PidsLimit <= 0 {
		t.Errorf("PidsLimit = %v, want a cap", hc.PidsLimit)
	}
	if hc.Memory != 256*1024*1024 || hc.NanoCPUs != 5e8 {
		t.Errorf("Memory = %d, NanoCPUs = %d; want the requested resources", hc.Memory, hc.NanoCPUs)
	}
}

// A file mover keeps only the capabilities file access needs.
func TestFileAccessHostConfig_FileAccessOnly(t *testing.T) {
	t.Parallel()
	hc := FileAccessHostConfig(nil, nil)
	if !slices.Contains(hc.CapDrop, "ALL") || !slices.Contains(hc.SecurityOpt, "no-new-privileges") || hc.Privileged {
		t.Errorf("CapDrop = %v, SecurityOpt = %v, Privileged = %v; want ALL dropped and no-new-privileges", hc.CapDrop, hc.SecurityOpt, hc.Privileged)
	}
	for _, c := range hc.CapAdd {
		if c != "CHOWN" && c != "DAC_OVERRIDE" && c != "FOWNER" {
			t.Errorf("CapAdd carries %s, want file access only", c)
		}
	}
	if hc.PidsLimit == nil {
		t.Error("no process cap")
	}
}
