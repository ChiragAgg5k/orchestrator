package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// MaxReadBytes caps a read artifact. Its content is carried inline in a
// callback and buffered in memory on the way; a build writes the file (e.g. a
// site's manifest.json), so the cap keeps a large or boundless one from
// exhausting the sidecar and the service that receives the callback.
const MaxReadBytes = 5 << 20

// Read reads file contents for inclusion in a callback event.
type Read struct {
	ID      string `json:"id"`
	In      string `json:"in"`               // Path to read from
	Format  string `json:"format,omitempty"` // "text" (default) delivers a raw string; "json" delivers the decoded value
	Depends string `json:"depends,omitempty"`
}

func (a *Read) ArtifactID() string   { return a.ID }
func (a *Read) ArtifactType() string { return "read" }
func (a *Read) DependsOn() string    { return a.Depends }

// Apply reads file contents for inclusion in a callback event.
func (a *Read) Apply(ctx context.Context, root *os.Root) *Result {
	f, info, err := openRegular(root, a.In)
	if err != nil {
		return &Result{Status: "failed", Error: fmt.Errorf("failed to read file: %w", err)}
	}
	defer f.Close()

	if info.Size() > MaxReadBytes {
		return &Result{Status: "failed", Error: fmt.Errorf("file is %d bytes, over the %d-byte read limit", info.Size(), MaxReadBytes)}
	}

	// LimitReader guards against a file that grows between the stat and the
	// read; +1 so exactly MaxReadBytes is allowed and one more is caught.
	content, err := io.ReadAll(io.LimitReader(f, MaxReadBytes+1))
	if err != nil {
		return &Result{Status: "failed", Error: fmt.Errorf("failed to read file: %w", err)}
	}
	if int64(len(content)) > MaxReadBytes {
		return &Result{Status: "failed", Error: fmt.Errorf("file exceeds the %d-byte read limit", MaxReadBytes)}
	}

	if a.Format == "json" {
		var decoded any
		if err := json.Unmarshal(content, &decoded); err != nil {
			return &Result{Status: "failed", Error: fmt.Errorf("failed to parse %s as JSON: %w", a.In, err)}
		}
		return &Result{Status: "success", Content: decoded}
	}

	return &Result{Status: "success", Content: string(content)}
}
