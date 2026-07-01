package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

func TestPrintPlanFileSummary(t *testing.T) {
	planFile := apply.NewPlanFile("test-version", "demo", "prod", "blue", "core", "prod-context", true, apply.Plan{
		CreateNetworks: []swarm.NetworkSpec{{Name: "net"}},
		CreateConfigs:  []swarm.ConfigSpec{{Name: "cfg"}},
		CreateSecrets:  []swarm.SecretSpec{{Name: "sec"}},
		StackDeploys:   []apply.StackDeploy{{Name: "demo_blue_core"}},
		DeleteConfigs:  []swarm.Config{{Name: "old-cfg"}},
		DeleteSecrets:  []swarm.Secret{{Name: "old-sec"}},
	})
	planFile.Secrets.Mode = apply.PlanSecretModeReference
	planFile.Inputs = []apply.PlanInput{{Kind: "project", Path: "project.yaml", SHA256: "abc123"}}
	planFile.SourceInputs = []apply.PlanSourceInput{{
		Kind:    "git",
		URL:     "ssh://git@example.com/repo.git",
		Ref:     "v1.2.3",
		Commit:  "0123456789abcdef",
		Path:    "deploy/app",
		Subtree: "abcdef0123456789",
	}}
	planFile.SecretSources = []apply.PlanSecretSource{{
		SecretName: "sec",
		Dependencies: []apply.PlanSecretDependency{{
			Name: "token",
		}},
	}}
	var out bytes.Buffer
	printPlanFileSummary(&out, "plan.yaml", planFile)
	got := out.String()
	for _, want := range []string{
		"show OK",
		"plan artifact: plan.yaml",
		"project: demo",
		"deployment: prod",
		"secret mode: reference",
		"inputs: 1",
		"source inputs: 1",
		"networks to create: 1",
		"stacks:",
		"secret sources:",
		"sec (1 dependency)",
		"source inputs:",
		"ssh://git@example.com/repo.git@v1.2.3#deploy/app commit=0123456789abcdef subtree=abcdef0123456789",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}
