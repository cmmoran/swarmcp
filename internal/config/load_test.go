package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
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

func TestNormalizeTemplateScalarsHandlesManyBareTemplateScalars(t *testing.T) {
	input := `mode: shared
services:
  portainer:
    image: {{ config_value "portainer_image" }}
    labels:
      traefik.http.routers.portainer.rule: {{ printf "Host(` + "`" + `m.%s` + "`" + `)" (config_value "internal_domain") }}
      traefik.swarm.network: {{ runtime_value "{networks_shared}" }}
  ingress:
    image: {{ config_value "traefik_image" }}
    env:
      AWS_HOSTED_ZONE_ID: {{ config_value "aws_hosted_zone_id" }}
      AWS_REGION: {{ config_value "aws_region" }}
      AWS_SHARED_CREDENTIALS_FILE: {{ secret_ref "aws_shared_credentials" }}
      VAULT_LOCK_ID: {{ escape_template "{{ .Service.Name }}.{{ .Task.Slot }}" }}
    configs:
      - name: traefik_yml
        source: templates/stacks/core/services/ingress/configs/traefik.yml.tmpl
      - name: dynamic_yml
        source: templates/stacks/core/services/ingress/configs/dynamic.yml.tmpl
    secrets:
      - name: aws_shared_credentials
        source: templates/stacks/core/services/ingress/secrets/aws_shared_credentials.tmpl
`
	normalized := normalizeTemplateScalars(input)
	var parsed any
	if err := yaml.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatalf("expected normalized YAML to parse, got: %v\nnormalized:\n%s", err, normalized)
	}
}

func TestRewriteTemplateScalarLineSkipsAlreadyQuotedTemplateScalar(t *testing.T) {
	input := `source: '{{ config_value "app" }}'`
	got, changed := rewriteTemplateScalarLine(input)
	if changed {
		t.Fatalf("expected already-quoted template scalar to remain unchanged, got %q", got)
	}
	if got != input {
		t.Fatalf("unexpected rewritten line: want %q got %q", input, got)
	}
}

func TestNormalizeTemplateScalarsParsesSynergyDevopsCoreStack(t *testing.T) {
	data, err := os.ReadFile("/home/user/code/sentinel/synergy-devops/deploy/traefik-portainer/core.stack.yaml.tmpl")
	if err != nil {
		t.Skipf("read downstream core stack: %v", err)
	}
	normalized := normalizeTemplateScalars(string(data))
	var parsed any
	if err := yaml.Unmarshal([]byte(normalized), &parsed); err != nil {
		t.Fatalf("expected normalized downstream core stack to parse, got: %v\nnormalized:\n%s", err, normalized)
	}
}
