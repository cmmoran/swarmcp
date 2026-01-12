package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type sourceMetadata struct {
	URL       string `json:"url"`
	Ref       string `json:"ref"`
	Commit    string `json:"commit"`
	FetchedAt string `json:"fetched_at"`
}

func FetchRepoRoot(url string, ref string, opts LoadOptions) (string, error) {
	if url == "" {
		return "", fmt.Errorf("source url is required")
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		return "", fmt.Errorf("cache dir is required for repo fetch")
	}
	key := hashKey(url)
	refKey := hashKey(strings.TrimSpace(ref))
	if refKey == "" {
		refKey = "head"
	}
	repoDir := filepath.Join(cacheDir, "repos", key, refKey)
	if _, err := os.Stat(repoDir); err != nil {
		if opts.Offline {
			return "", fmt.Errorf("repo %q not cached and offline is enabled", url)
		}
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return "", err
		}
		if err := runGit("", "clone", url, repoDir); err != nil {
			return "", err
		}
		if ref != "" {
			if err := runGit(repoDir, "fetch", "--tags", "origin"); err != nil {
				return "", err
			}
			if err := runGit(repoDir, "checkout", ref); err != nil {
				return "", err
			}
		}
	} else if !opts.Offline {
		if err := runGit(repoDir, "fetch", "--tags", "origin"); err != nil {
			return "", err
		}
		if ref != "" {
			if err := runGit(repoDir, "checkout", ref); err != nil {
				return "", err
			}
		}
	}

	commit, err := gitOutput(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	commit = strings.TrimSpace(commit)
	if err := writeSourceMetadata(repoDir, url, ref, commit); err != nil {
		return "", err
	}
	return repoDir, nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func writeSourceMetadata(repoDir, url, ref, commit string) error {
	data := sourceMetadata{
		URL:       url,
		Ref:       ref,
		Commit:    commit,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(repoDir, ".swarmcp_source.json")
	return os.WriteFile(path, encoded, 0o644)
}

func hashKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
