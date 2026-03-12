package swarm

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerConfigDirUsesEnvAndHome(t *testing.T) {
	prev := os.Getenv("DOCKER_CONFIG")
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("DOCKER_CONFIG")
			return
		}
		_ = os.Setenv("DOCKER_CONFIG", prev)
	})

	custom := t.TempDir()
	if err := os.Setenv("DOCKER_CONFIG", custom); err != nil {
		t.Fatalf("Setenv: %v", err)
	}
	if got := dockerConfigDir(); got != custom {
		t.Fatalf("expected dockerConfigDir to use env override, got %q", got)
	}

	if err := os.Unsetenv("DOCKER_CONFIG"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	if got := dockerConfigDir(); got == "" {
		t.Fatalf("expected dockerConfigDir to fall back to a non-empty path")
	}
}

func TestContextDirHashesName(t *testing.T) {
	sum := sha256.Sum256([]byte("prod"))
	want := hex.EncodeToString(sum[:])
	if got := contextDir("prod"); got != want {
		t.Fatalf("unexpected context dir hash: want %q got %q", want, got)
	}
}

func TestLoadTLSConfig(t *testing.T) {
	configDir := t.TempDir()
	contextID := contextDir("prod")

	cfg, err := loadTLSConfig(configDir, contextID, false)
	if err != nil {
		t.Fatalf("loadTLSConfig empty dir: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil TLS config when no files exist and skipVerify is false")
	}

	tlsDir := filepath.Join(configDir, "contexts", "tls", contextID, "docker")
	if err := os.MkdirAll(tlsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "cert.pem"), []byte("not-a-cert"), 0o644); err != nil {
		t.Fatalf("WriteFile cert: %v", err)
	}
	if _, err := loadTLSConfig(configDir, contextID, false); err == nil {
		t.Fatalf("expected missing key alongside cert to fail")
	}

	skipContextID := contextDir("skip-verify")
	if cfg, err = loadTLSConfig(configDir, skipContextID, true); err != nil {
		t.Fatalf("loadTLSConfig skip verify: %v", err)
	} else if cfg == nil || !cfg.InsecureSkipVerify {
		t.Fatalf("expected insecure TLS config when skipVerify is true: %#v", cfg)
	}
}
