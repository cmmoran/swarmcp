package cmd

import (
	"fmt"
	"sort"

	"github.com/cmmoran/swarmcp/internal/apply"
	"github.com/cmmoran/swarmcp/internal/cmdutil"
	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/templates"
)

func buildPlanInputs(cfg *config.Config, configPath string, configPaths []string, releaseConfigPaths []string, valuesSources []string, secretsFile string) ([]apply.PlanInput, error) {
	var inputs []apply.PlanInput
	configInputs, err := apply.FileInputs("project", configPaths)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, configInputs...)
	releaseInputs, err := apply.FileInputs("release", releaseConfigPaths)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, releaseInputs...)
	valuesInputs, err := apply.FileInputs("values", localPlanInputs(cmdutil.InferValuesFiles(configPath, valuesSources)))
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, valuesInputs...)
	secretInputs, err := apply.FileInputs("secrets", []string{cmdutil.InferSecretsFile(cfg, configPath, secretsFile)})
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, secretInputs...)
	return inputs, nil
}

func localPlanInputs(paths []string) []string {
	var out []string
	for _, path := range paths {
		base, _ := templates.SplitSource(path)
		if config.IsGitSource(base) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func planArtifactWarnings(inputs []apply.PlanInput) []string {
	var warnings []string
	for _, input := range inputs {
		if input.Kind != "values" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("plan artifact reproducibility depends on recovering exact local values input %q with sha256 %s; store/version this file externally", input.Path, input.SHA256))
	}
	return warnings
}

type planSourceRef struct {
	Source string
	Origin string
}

func buildPlanSourceInputs(cfg *config.Config, desired apply.DesiredState, plan apply.Plan, valuesSources []string) ([]apply.PlanSourceInput, error) {
	refs := planSourceRefs(desired, plan, valuesSources)
	loadOpts := config.LoadOptions{CacheDir: cfg.CacheDir, Offline: opts.Offline, Debug: opts.Debug}
	return buildPlanSourceInputsFromRefs(refs, loadOpts, config.ReadSourceMetadata)
}

func planSourceRefs(desired apply.DesiredState, plan apply.Plan, valuesSources []string) []planSourceRef {
	var refs []planSourceRef
	for _, source := range valuesSources {
		refs = append(refs, planSourceRef{
			Source: source,
			Origin: "values input",
		})
	}
	for _, def := range desired.Defs {
		if def.Source == "" {
			continue
		}
		refs = append(refs, planSourceRef{
			Source: def.Source,
			Origin: def.Scope + " " + def.Kind + " " + def.Name,
		})
	}
	for _, deploy := range plan.StackDeploys {
		for _, source := range deploy.SourceRefs {
			refs = append(refs, planSourceRef{
				Source: source,
				Origin: "stack deploy " + deploy.Name,
			})
		}
	}
	return refs
}

func buildPlanSourceInputsFromRefs(refs []planSourceRef, loadOpts config.LoadOptions, read func(string, string, string, config.LoadOptions) (config.SourceMetadata, bool, error)) ([]apply.PlanSourceInput, error) {
	byKey := map[string]apply.PlanSourceInput{}
	for _, ref := range refs {
		base, _ := templates.SplitSource(ref.Source)
		if !config.IsGitSource(base) {
			continue
		}
		parsed, ok, err := config.ParseGitSource(base)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		meta, found, err := read(parsed.URL, parsed.Ref, parsed.Path, loadOpts)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("source metadata missing for %s; refusing to re-resolve git ref while writing plan artifact", ref.Origin)
		}
		input := apply.PlanSourceInput{
			Kind:    "git",
			Origin:  ref.Origin,
			URL:     meta.URL,
			Ref:     meta.Ref,
			Commit:  meta.Commit,
			Path:    meta.Path,
			Subtree: meta.Subtree,
		}
		key := input.URL + "\x00" + input.Ref + "\x00" + input.Commit + "\x00" + input.Path + "\x00" + input.Subtree
		if _, ok := byKey[key]; !ok {
			byKey[key] = input
		}
	}
	out := make([]apply.PlanSourceInput, 0, len(byKey))
	for _, input := range byKey {
		out = append(out, input)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URL != out[j].URL {
			return out[i].URL < out[j].URL
		}
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Origin < out[j].Origin
	})
	return out, nil
}
