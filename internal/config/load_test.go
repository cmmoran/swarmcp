package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateLogicalNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", maxLogicalNameLen+1)
	cfg := &Config{
		Project: Project{
			Name: "primary",
			Configs: map[string]ConfigDef{
				longName: {Source: "path"},
			},
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for long logical name")
	}
}

func TestValidateCoreStackPartitioned(t *testing.T) {
	cfg := &Config{
		Project: Project{Name: "primary"},
		Stacks: map[string]Stack{
			"core": {Mode: "partitioned"},
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for partitioned core stack")
	}
}

func TestValidateDeploymentMissingFromAllowedList(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployments: []string{"nonprod"},
			Deployment:  "prod",
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for deployment not in project.deployments")
	}
}

func TestValidateContextMissingFromAllowedList(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:        "primary",
			Deployments: []string{"nonprod"},
			Contexts: map[string]string{
				"prod": "prod",
			},
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for context not in project.deployments")
	}
}

func TestValidatePartitionOverlayMissingPartition(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name:       "primary",
			Partitions: []string{"dev"},
		},
		Overlays: Overlays{
			Partitions: PartitionOverlays{
				Rules: []PartitionOverlay{
					{
						Name:  "qa",
						Match: OverlayMatch{Partition: OverlayMatchPartition{Pattern: "qa"}},
					},
				},
			},
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for unknown partition overlay")
	}
}

func TestValidateSecretsEngineMissingAddr(t *testing.T) {
	cfg := &Config{
		Project: Project{
			Name: "primary",
			SecretsEngine: &SecretsEngine{
				Provider: "vault",
				Vault: &VaultKV{
					Mount:        "kv",
					PathTemplate: "{project}",
				},
			},
		},
	}

	if err := Validate(cfg); err == nil {
		t.Fatalf("expected validation error for missing secrets_engine.addr")
	}
}

func TestStackSecretsList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "swarmcp.yaml")
	config := `
project:
  name: primary
stacks:
  tools:
    secrets:
      - rpc_secret
      - vault_secret
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadWithOptions(path, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	stackDefs := cfg.StackSecretDefs("tools", "")
	if def, ok := stackDefs["rpc_secret"]; !ok || def.Source == "" {
		t.Fatalf("expected stack secret rpc_secret with default source")
	}
	if def, ok := stackDefs["vault_secret"]; !ok || def.Source == "" {
		t.Fatalf("expected stack secret vault_secret with default source")
	}
}

func TestStackConfigsList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "swarmcp.yaml")
	config := `
project:
  name: primary
stacks:
  tools:
    configs:
      - name: drone_env
        source: values#/drone/env
      - name: vault_conf
        source: values#/vault/conf
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadWithOptions(path, LoadOptions{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	stackDefs := cfg.StackConfigDefs("tools", "")
	if def, ok := stackDefs["drone_env"]; !ok || def.Source == "" {
		t.Fatalf("expected stack config drone_env with source")
	}
	if _, ok := stackDefs["vault_conf"]; !ok {
		t.Fatalf("expected stack config vault_conf")
	}
}

func TestNormalizeTemplateScalarsQuotesBareMappingTemplate(t *testing.T) {
	input := "source: {{ config_value \"app\" }}\n"
	got := normalizeTemplateScalars(input)
	want := "source: '{{ config_value \"app\" }}'\n"
	if got != want {
		t.Fatalf("unexpected normalized mapping scalar:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsQuotesBareListTemplate(t *testing.T) {
	input := "- {{ config_value \"app\" }}\n"
	got := normalizeTemplateScalars(input)
	want := "- '{{ config_value \"app\" }}'\n"
	if got != want {
		t.Fatalf("unexpected normalized list scalar:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsPreservesInlineComment(t *testing.T) {
	input := "source: {{ config_value \"app\" }} # comment\n"
	got := normalizeTemplateScalars(input)
	want := "source: '{{ config_value \"app\" }}' # comment\n"
	if got != want {
		t.Fatalf("unexpected normalized scalar with comment:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsDoesNotTouchQuotedTemplate(t *testing.T) {
	input := "source: \"{{ config_value \\\"app\\\" }}\"\n"
	got := normalizeTemplateScalars(input)
	if got != input {
		t.Fatalf("expected quoted template scalar to remain unchanged:\nwant: %q\ngot:  %q", input, got)
	}
}

func TestNormalizeTemplateScalarsDoesNotTouchEmbeddedTemplateText(t *testing.T) {
	input := "source: prefix-{{ config_value \"app\" }}\n"
	got := normalizeTemplateScalars(input)
	want := "source: 'prefix-{{ config_value \"app\" }}'\n"
	if got != want {
		t.Fatalf("expected embedded template text to be quoted:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsQuotesBareFlowTemplate(t *testing.T) {
	input := "source: {{ config_value \"app\" }}\n"
	got := normalizeTemplateScalars(input)
	want := "source: '{{ config_value \"app\" }}'\n"
	if got != want {
		t.Fatalf("unexpected normalized bare flow template:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsDoesNotTouchUnbalancedTemplateText(t *testing.T) {
	input := "source: prefix-{{ config_value \"app\"\n"
	got := normalizeTemplateScalars(input)
	if got != input {
		t.Fatalf("expected unbalanced template text to remain unchanged:\nwant: %q\ngot:  %q", input, got)
	}
}

func TestNormalizeTemplateScalarsFallbackHandlesRuntimeValueWithBackticks(t *testing.T) {
	input := "source: {{ runtime_value `{project}_{stack}` }}\n"
	got := normalizeTemplateScalars(input)
	want := "source: '{{ runtime_value `{project}_{stack}` }}'\n"
	if got != want {
		t.Fatalf("unexpected normalized fallback scalar:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeTemplateScalarsFallbackHandlesEmbeddedTemplateWithBackticks(t *testing.T) {
	input := "source: prefix-{{ runtime_value `{project}_{stack}` }}\n"
	got := normalizeTemplateScalars(input)
	want := "source: 'prefix-{{ runtime_value `{project}_{stack}` }}'\n"
	if got != want {
		t.Fatalf("unexpected normalized fallback embedded scalar:\nwant: %q\ngot:  %q", want, got)
	}
}
