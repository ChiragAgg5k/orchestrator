package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServiceConfig_APIKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	emptyFile := filepath.Join(dir, "empty")
	if err := os.WriteFile(emptyFile, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		file    string
		env     string
		allow   string
		want    string
		wantErr bool
	}{
		{name: "file", file: keyFile, want: "from-file"},
		{name: "file wins over env", file: keyFile, env: "from-env", want: "from-file"},
		{name: "env", env: " from-env ", want: "from-env"},
		{name: "none", wantErr: true},
		{name: "none, opted in", allow: "true", want: ""},
		{name: "none, opt-in misspelt", allow: "1", wantErr: true},
		{name: "unreadable file", file: filepath.Join(dir, "missing"), wantErr: true},
		{name: "unreadable file with env", file: filepath.Join(dir, "missing"), env: "from-env", wantErr: true},
		{name: "empty file", file: emptyFile, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("API_KEY_FILE", tt.file)
			t.Setenv("API_KEY", tt.env)
			t.Setenv(EnvAllowUnauthenticated, tt.allow)

			cfg, err := LoadServiceConfig()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadServiceConfig() = %q, want an error", cfg.APIKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadServiceConfig() error = %v", err)
			}
			if cfg.APIKey != tt.want {
				t.Errorf("APIKey = %q, want %q", cfg.APIKey, tt.want)
			}
		})
	}
}
