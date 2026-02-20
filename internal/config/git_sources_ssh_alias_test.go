package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

func TestResolveSSHAliasEndpointHostNameAndPort(t *testing.T) {
	t.Setenv("SSH_CONFIG", "")
	cfgPath := filepath.Join(t.TempDir(), "config")
	cfg := `Host gh-devops
  HostName github.com
  Port 2222
  User git
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}
	ep, err := transport.NewEndpoint("git@gh-devops:sentinelgo/synergy-devops.git")
	if err != nil {
		t.Fatalf("new endpoint: %v", err)
	}
	out, err := resolveSSHAliasEndpoint(ep, cfgPath, LoadOptions{})
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	if out.Host != "github.com" {
		t.Fatalf("host mismatch: got %q want %q", out.Host, "github.com")
	}
	if out.Port != 2222 {
		t.Fatalf("port mismatch: got %d want %d", out.Port, 2222)
	}
}

func TestResolveSSHAliasEndpointNoopForHTTPS(t *testing.T) {
	ep, err := transport.NewEndpoint("https://github.com/sentinelgo/synergy-devops.git")
	if err != nil {
		t.Fatalf("new endpoint: %v", err)
	}
	out, err := resolveSSHAliasEndpoint(ep, "", LoadOptions{})
	if err != nil {
		t.Fatalf("resolve endpoint: %v", err)
	}
	if out.Host != "github.com" {
		t.Fatalf("host mismatch: got %q", out.Host)
	}
}
