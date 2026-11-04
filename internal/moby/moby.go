// Package moby is the shared Docker-side plumbing: the counterpart to
// internal/kube, for the parts of talking to the engine that are the same
// whatever workload is being run — translating our volumes into mounts, making
// sure an image is present, and attaching to the configured network.
//
// Named for the engine's own project because the obvious names are taken: the
// three per-domain adapters are each `package docker`, and `container`,
// `network`, `image` and `mount` are all packages of the Docker SDK these files
// import.
package moby

import (
	"context"
	"fmt"
	"orchestrator/internal/apperrors"
	"orchestrator/internal/volume"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/versions"
)

// SubpathMinAPIVersion is the first Engine API version that honours a volume
// mount's Subpath (Docker Engine 26). Older daemons drop the field without an
// error and mount the whole volume.
const SubpathMinAPIVersion = "1.45"

// Pinger is the part of the Docker client Mounts needs: the daemon's API version.
type Pinger interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
}

// Mounts translates declared volumes into Docker mounts. The Kubernetes
// counterpart is kube.PersistentVolumes.
//
// A volume with a subPath is refused when the daemon speaks an API older than
// SubpathMinAPIVersion: it would ignore the subPath and give the container the
// whole volume, which is wider than anything the request asked for.
func Mounts(ctx context.Context, cli Pinger, vols []volume.Volume) ([]mount.Mount, error) {
	mounts := make([]mount.Mount, 0, len(vols))
	for i, v := range vols {
		m := mount.Mount{Type: mount.TypeVolume, Source: v.Source, Target: v.Path, ReadOnly: v.ReadOnly}
		if v.SubPath != "" {
			ping, err := cli.Ping(ctx, client.PingOptions{})
			if err != nil {
				return nil, apperrors.Internal("docker.ping", err)
			}
			if apiVersion := ping.APIVersion; versions.LessThan(apiVersion, SubpathMinAPIVersion) {
				return nil, apperrors.Validation(fmt.Sprintf("volumes[%d].subPath", i),
					fmt.Sprintf("subPath needs Docker Engine API %s or newer (Docker 26+); this daemon speaks %s and would mount the whole volume", SubpathMinAPIVersion, apiVersion))
			}
			m.VolumeOptions = &mount.VolumeOptions{Subpath: v.SubPath}
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}

// PullImage makes sure an image is present locally, pulling it if it is not. A
// present image is left alone — no tag is re-resolved mid-flight, so a running
// workload and the next one started from the same tag are the same bytes.
func PullImage(ctx context.Context, cli *client.Client, ref string) error {
	if _, err := cli.ImageInspect(ctx, ref); err == nil {
		return nil
	}
	pull, err := cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	return pull.Wait(ctx)
}

// NetworkingConfig attaches a container to the configured network, or returns
// nil for the default bridge when no network is configured.
func NetworkingConfig(name string) *network.NetworkingConfig {
	if name == "" {
		return nil
	}
	return &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{name: {}},
	}
}

// Process caps. A build spawns compilers and bundlers, far below what
// exhausts the host; the artifact sidecar is a single Go process.
const (
	workloadPidsLimit   = 16384
	fileAccessPidsLimit = 1024
)

// WorkloadHostConfig is the Docker counterpart of kube.WorkloadSecurityContext,
// for a container running the user's own command (a build, a deployment, a
// sandbox): it keeps the image's user and a writable filesystem, since
// installers run as root and write everywhere, but drops CAP_NET_RAW — on a
// shared bridge a raw socket reaches neighbouring containers' traffic —
// forbids gaining privileges through setuid binaries and caps the process
// count. The daemon applies its default seccomp profile on its own.
func WorkloadHostConfig(mounts []mount.Mount, cpu float64, memoryMiB int, extraHosts []string) *container.HostConfig {
	pids := int64(workloadPidsLimit)
	return &container.HostConfig{
		Mounts:      mounts,
		NanoCPUs:    int64(cpu * 1e9),
		Memory:      int64(memoryMiB) * 1024 * 1024,
		PidsLimit:   &pids,
		ExtraHosts:  extraHosts,
		CapDrop:     []string{"NET_RAW"},
		SecurityOpt: []string{"no-new-privileges"},
	}
}

// FileAccessHostConfig is the Docker counterpart of
// kube.FileAccessSecurityContext, for the artifact sidecar and anything else
// that only moves files in the shared workspace: root with only the
// capabilities needed to read, write and re-own files of any owner or mode,
// and no privilege gain.
func FileAccessHostConfig(mounts []mount.Mount, extraHosts []string) *container.HostConfig {
	pids := int64(fileAccessPidsLimit)
	return &container.HostConfig{
		Mounts:      mounts,
		PidsLimit:   &pids,
		ExtraHosts:  extraHosts,
		CapDrop:     []string{"ALL"},
		CapAdd:      []string{"CHOWN", "DAC_OVERRIDE", "FOWNER"},
		SecurityOpt: []string{"no-new-privileges"},
	}
}
