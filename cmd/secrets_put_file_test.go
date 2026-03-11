package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmmoran/swarmcp/internal/secrets"
)

func TestSecretsPutWritesToFile(t *testing.T) {
	prevOpts := opts
	prevFromFile := secretsPutFromFile
	prevStdin := secretsPutStdin
	prevStack := secretsPutStack
	prevPartition := secretsPutPartition
	prevService := secretsPutService
	t.Cleanup(func() {
		opts = prevOpts
		secretsPutFromFile = prevFromFile
		secretsPutStdin = prevStdin
		secretsPutStack = prevStack
		secretsPutPartition = prevPartition
		secretsPutService = prevService
	})

	configPath := filepath.Join(t.TempDir(), "swarmcp.yaml")
	if err := os.WriteFile(configPath, []byte("project:\n  name: test\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	secretsPath := filepath.Join(t.TempDir(), "secrets.yaml")
	opts.ConfigPaths = []string{configPath}
	opts.SecretsFile = secretsPath

	if err := secretsPutCmd.RunE(secretsPutCmd, []string{"db_password", "value"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	store, err := secrets.Load(secretsPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Values["db_password"] != "value" {
		t.Fatalf("expected value, got %q", store.Values["db_password"])
	}
}

func TestSecretsPutWritesFromFile(t *testing.T) {
	prevOpts := opts
	prevFromFile := secretsPutFromFile
	prevStdin := secretsPutStdin
	t.Cleanup(func() {
		opts = prevOpts
		secretsPutFromFile = prevFromFile
		secretsPutStdin = prevStdin
	})

	configPath := filepath.Join(t.TempDir(), "swarmcp.yaml")
	if err := os.WriteFile(configPath, []byte("project:\n  name: test\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	secretsPath := filepath.Join(t.TempDir(), "secrets.yaml")
	opts.ConfigPaths = []string{configPath}
	opts.SecretsFile = secretsPath

	valuePath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(valuePath, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write value file: %v", err)
	}
	secretsPutFromFile = valuePath

	if err := secretsPutCmd.RunE(secretsPutCmd, []string{"db_password"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	store, err := secrets.Load(secretsPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Values["db_password"] != "from-file" {
		t.Fatalf("expected value, got %q", store.Values["db_password"])
	}
}

func TestSecretsPutWritesFromStdin(t *testing.T) {
	prevOpts := opts
	prevFromFile := secretsPutFromFile
	prevStdin := secretsPutStdin
	prevStdinFile := os.Stdin
	t.Cleanup(func() {
		opts = prevOpts
		secretsPutFromFile = prevFromFile
		secretsPutStdin = prevStdin
		os.Stdin = prevStdinFile
	})

	configPath := filepath.Join(t.TempDir(), "swarmcp.yaml")
	if err := os.WriteFile(configPath, []byte("project:\n  name: test\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	secretsPath := filepath.Join(t.TempDir(), "secrets.yaml")
	opts.ConfigPaths = []string{configPath}
	opts.SecretsFile = secretsPath
	secretsPutStdin = true

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := writer.WriteString("from-stdin\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	os.Stdin = reader

	if err := secretsPutCmd.RunE(secretsPutCmd, []string{"db_password"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}

	store, err := secrets.Load(secretsPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if store.Values["db_password"] != "from-stdin" {
		t.Fatalf("expected value, got %q", store.Values["db_password"])
	}
}
