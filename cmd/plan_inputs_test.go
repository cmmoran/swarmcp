package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
)

func TestBuildPlanInputsIncludesInferredValues(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project.yaml")
	release := filepath.Join(dir, "release.yaml")
	secrets := filepath.Join(dir, "secrets.yaml")
	valuesDir := filepath.Join(dir, "values")
	values := filepath.Join(valuesDir, "values.yaml")
	if err := os.MkdirAll(valuesDir, 0o755); err != nil {
		t.Fatalf("mkdir values: %v", err)
	}
	for path, content := range map[string]string{
		project: "project: {}\n",
		release: "stacks: {}\n",
		secrets: "token: value\n",
		values:  "global: {}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	inputs, err := buildPlanInputs(&config.Config{}, project, []string{project}, []string{release}, nil, "")
	if err != nil {
		t.Fatalf("buildPlanInputs: %v", err)
	}
	seen := map[string]bool{}
	for _, input := range inputs {
		seen[input.Kind+":"+input.Path] = true
	}
	for _, want := range []string{
		"project:" + project,
		"release:" + release,
		"secrets:" + secrets,
		"values:" + values,
	} {
		if !seen[want] {
			t.Fatalf("expected input %q in %#v", want, inputs)
		}
	}
}

func TestPlanArtifactWarningsFlagLocalValuesInputs(t *testing.T) {
	warnings := planArtifactWarnings([]apply.PlanInput{
		{Kind: "project", Path: "/tmp/project.yaml", SHA256: "projecthash"},
		{Kind: "values", Path: "/tmp/values.yaml.tmpl", SHA256: "valueshash"},
	})
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", warnings)
	}
	for _, want := range []string{"/tmp/values.yaml.tmpl", "valueshash", "recovering exact local values input"} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("expected warning to contain %q, got %q", want, warnings[0])
		}
	}
}

func TestBuildPlanSourceInputsRecordsResolvedGitMetadata(t *testing.T) {
	refs := []planSourceRef{
		{Source: "/tmp/local"},
		{
			Source: "git:ssh%3A%2F%2Fgit%40example.com%2Frepo.git|v1.2.3|deploy%2Fapp",
			Origin: "stack app config message",
		},
	}
	read := func(url, ref, path string, opts config.LoadOptions) (config.SourceMetadata, bool, error) {
		if url != "ssh://git@example.com/repo.git" || ref != "v1.2.3" || path != "deploy/app" {
			t.Fatalf("unexpected read args: url=%q ref=%q path=%q", url, ref, path)
		}
		return config.SourceMetadata{
			URL:     url,
			Ref:     ref,
			Commit:  "0123456789abcdef",
			Path:    path,
			Subtree: "abcdef0123456789",
		}, true, nil
	}

	inputs, err := buildPlanSourceInputsFromRefs(refs, config.LoadOptions{}, read)
	if err != nil {
		t.Fatalf("buildPlanSourceInputsFromRefs: %v", err)
	}
	want := []apply.PlanSourceInput{{
		Kind:    "git",
		Origin:  "stack app config message",
		URL:     "ssh://git@example.com/repo.git",
		Ref:     "v1.2.3",
		Commit:  "0123456789abcdef",
		Path:    "deploy/app",
		Subtree: "abcdef0123456789",
	}}
	if len(inputs) != len(want) || inputs[0] != want[0] {
		t.Fatalf("unexpected source inputs: %#v", inputs)
	}
}

func TestBuildPlanSourceInputsRequiresObservedMetadata(t *testing.T) {
	read := func(string, string, string, config.LoadOptions) (config.SourceMetadata, bool, error) {
		return config.SourceMetadata{}, false, nil
	}
	_, err := buildPlanSourceInputsFromRefs([]planSourceRef{{
		Source: "git:ssh%3A%2F%2Fgit%40example.com%2Frepo.git|main|deploy%2Fapp",
		Origin: "stack deploy app",
	}}, config.LoadOptions{}, read)
	if err == nil || !strings.Contains(err.Error(), "refusing to re-resolve git ref") {
		t.Fatalf("expected missing metadata error, got %v", err)
	}
}

func TestBuildPlanSourceInputsFromImportedGitStack(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "stack-repo")
	cacheDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(filepath.Join(repo, "deploy", "app", "templates"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, repo, "init", ".")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "deploy", "app", "stack.yaml"), []byte(`
mode: shared
configs:
  message:
    source: templates/message.txt
services:
  api:
    source:
      path: service.yaml
`), 0o600); err != nil {
		t.Fatalf("write stack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "app", "service.yaml"), []byte(`
image: nginx:alpine
configs:
  - name: message
`), 0o600); err != nil {
		t.Fatalf("write service: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "app", "templates", "message.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "app", "overrides.yaml"), []byte("mode: shared\n"), 0o600); err != nil {
		t.Fatalf("write stack overrides: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "deploy", "tools", "templates"), 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "tools", "stack.yaml"), []byte(`
mode: shared
configs:
  tools:
    source: templates/tools.txt
services:
  tools:
    image: busybox:latest
    configs:
      - name: tools
`), 0o600); err != nil {
		t.Fatalf("write tools stack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "tools", "templates", "tools.txt"), []byte("tools\n"), 0o600); err != nil {
		t.Fatalf("write tools template: %v", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add stack")
	commit := runGit(t, repo, "rev-parse", "HEAD")
	url := "file://" + filepath.ToSlash(repo)
	cacheRepo := filepath.Join(cacheDir, "repos", testSourceHashKey(url))
	if err := os.MkdirAll(filepath.Dir(cacheRepo), 0o755); err != nil {
		t.Fatalf("mkdir cache repos: %v", err)
	}
	runGit(t, dir, "clone", "--bare", repo, cacheRepo)

	project := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(project, []byte(`
project:
  name: demo
  deployment: qa
stacks:
  app:
    source:
      url: `+url+`
      ref: main
      path: deploy/app/stack.yaml
      overrides_path: deploy/app/overrides.yaml
  tools:
    source:
      url: `+url+`
      ref: main
      path: deploy/tools/stack.yaml
overlays:
  deployments:
    qa:
      stacks:
        app:
          services:
            api:
              replicas: 2
`), 0o600); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := config.LoadWithOptions(project, config.LoadOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	desired, err := apply.BuildDesiredState(cfg, &secrets.Store{Values: map[string]string{}}, nil, nil, []string{"app"}, false, true)
	if err != nil {
		t.Fatalf("BuildDesiredState: %v", err)
	}
	deploys, err := apply.BuildStackDeploys(cfg, desired, nil, nil, []string{"app"}, nil, nil, nil, true)
	if err != nil {
		t.Fatalf("BuildStackDeploys: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "deploy", "app", "templates", "message.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("rewrite template: %v", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "move main after render")
	if newCommit := runGit(t, repo, "rev-parse", "HEAD"); newCommit == commit {
		t.Fatalf("expected branch to move after render")
	}
	inputs, err := buildPlanSourceInputs(cfg, desired, apply.Plan{StackDeploys: deploys}, nil)
	if err != nil {
		t.Fatalf("buildPlanSourceInputs: %v", err)
	}
	seen := map[string]apply.PlanSourceInput{}
	for _, input := range inputs {
		seen[input.Path] = input
		if input.Commit != commit {
			t.Fatalf("expected commit %s, got source input %#v", commit, input)
		}
		if input.Subtree == "" {
			t.Fatalf("expected subtree hash, got source input %#v", input)
		}
	}
	for _, path := range []string{"deploy/app/stack.yaml", "deploy/app/overrides.yaml", "deploy/app/service.yaml", "deploy/app/templates/message.txt"} {
		if _, ok := seen[path]; !ok {
			t.Fatalf("expected source input for %q, got %#v", path, inputs)
		}
	}
	for _, path := range []string{"deploy/tools/stack.yaml", "deploy/tools/templates/tools.txt"} {
		if _, ok := seen[path]; ok {
			t.Fatalf("did not expect source input for unselected path %q, got %#v", path, inputs)
		}
	}
}

func TestBuildPlanSourceInputsIncludesGitValues(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "values-repo")
	cacheDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(filepath.Join(repo, "values"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, repo, "init", ".")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "checkout", "-b", "main")
	valuesContent := "global:\n  image: nginx:alpine\n"
	if err := os.WriteFile(filepath.Join(repo, "values", "values.yaml"), []byte(valuesContent), 0o600); err != nil {
		t.Fatalf("write values: %v", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add values")
	commit := runGit(t, repo, "rev-parse", "HEAD")
	url := "file://" + filepath.ToSlash(repo)
	cacheRepo := filepath.Join(cacheDir, "repos", testSourceHashKey(url))
	if err := os.MkdirAll(filepath.Dir(cacheRepo), 0o755); err != nil {
		t.Fatalf("mkdir cache repos: %v", err)
	}
	runGit(t, dir, "clone", "--bare", repo, cacheRepo)

	project := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(project, []byte(`
project:
  name: demo
  deployment: qa
  values:
    - name: platform
      url: `+url+`
      ref: main
      path: values/values.yaml
stacks: {}
`), 0o600); err != nil {
		t.Fatalf("write project: %v", err)
	}

	cfg, err := config.LoadWithOptions(project, config.LoadOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	valuesSources, err := cmdutil.InferValuesSources(cfg, project, nil)
	if err != nil {
		t.Fatalf("InferValuesSources: %v", err)
	}
	inputs, err := buildPlanInputs(cfg, project, []string{project}, nil, valuesSources, "")
	if err != nil {
		t.Fatalf("buildPlanInputs: %v", err)
	}
	for _, input := range inputs {
		if input.Kind == "values" {
			t.Fatalf("did not expect git values in local plan inputs: %#v", inputs)
		}
	}

	sourceInputs, err := buildPlanSourceInputs(cfg, apply.DesiredState{}, apply.Plan{}, valuesSources)
	if err != nil {
		t.Fatalf("buildPlanSourceInputs: %v", err)
	}
	if len(sourceInputs) != 1 {
		t.Fatalf("expected one source input, got %#v", sourceInputs)
	}
	got := sourceInputs[0]
	if got.Origin != "values input" || got.URL != url || got.Ref != "main" || got.Commit != commit || got.Path != "values/values.yaml" || got.Subtree == "" {
		t.Fatalf("unexpected values source input: %#v", got)
	}
}

func testSourceHashKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
