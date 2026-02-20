package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRewriteGitURLLongestPrefixWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_LOCAL", "")

	globalCfg := filepath.Join(home, ".gitconfig")
	content := `[url "git@gh-org:"]
	insteadOf = https://github.com/sentinelgo/
[url "git@gh-repo:"]
	insteadOf = https://github.com/sentinelgo/synergy-common
`
	if err := os.WriteFile(globalCfg, []byte(content), 0o600); err != nil {
		t.Fatalf("write gitconfig: %v", err)
	}

	got := rewriteGitURL("https://github.com/sentinelgo/synergy-common")
	want := "git@gh-repo:"
	if got != want {
		t.Fatalf("rewrite mismatch: got %q want %q", got, want)
	}
}

func TestRewriteGitURLLocalOverridesGlobalOnEqualPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	globalCfg := filepath.Join(home, ".gitconfig")
	global := `[url "git@gh-global:"]
	insteadOf = https://github.com/sentinelgo/synergy-devops
`
	if err := os.WriteFile(globalCfg, []byte(global), 0o600); err != nil {
		t.Fatalf("write global gitconfig: %v", err)
	}

	repoRoot := t.TempDir()
	localGit := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(localGit, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	localCfg := filepath.Join(localGit, "config")
	local := `[url "git@gh-local:"]
	insteadOf = https://github.com/sentinelgo/synergy-devops
`
	if err := os.WriteFile(localCfg, []byte(local), 0o600); err != nil {
		t.Fatalf("write local git config: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	got := rewriteGitURL("https://github.com/sentinelgo/synergy-devops")
	want := "git@gh-local:"
	if got != want {
		t.Fatalf("rewrite mismatch: got %q want %q", got, want)
	}
}

func TestRewriteGitURLNoMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_LOCAL", "")

	globalCfg := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(globalCfg, []byte(`[user]
	name = test
`), 0o600); err != nil {
		t.Fatalf("write global gitconfig: %v", err)
	}

	in := "https://example.com/org/repo"
	got := rewriteGitURL(in)
	if got != in {
		t.Fatalf("expected URL unchanged, got %q", got)
	}
}
