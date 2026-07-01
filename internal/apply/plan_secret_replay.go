package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/render"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/swarm"
	"github.com/cmmoran/swarmcp/internal/templates"
)

const (
	PlanSecretModePayload   = "payload"
	PlanSecretModeReference = "reference"
	PlanSecretModeRecipe    = "recipe"
	PlanSecretModeMixed     = "mixed"
)

func OmitReplayableSecretPayloads(plan *Plan, sources []PlanSecretSource) {
	sourceByName := planSecretSourcesByName(sources)
	for i := range plan.CreateSecrets {
		secret := &plan.CreateSecrets[i]
		source, ok := sourceByName[secret.Name]
		if !ok {
			continue
		}
		if !isDirectReplayableSecret(source, secretDataHash(secret.Data)) {
			continue
		}
		secret.Data = nil
		secret.HasData = false
	}
}

func OmitReplayableSecretPayloadsFromPlan(ctx context.Context, planFile *PlanFile) {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	fileStoreByPath := map[string]*secrets.Store{}
	for i := range planFile.Plan.CreateSecrets {
		secret := &planFile.Plan.CreateSecrets[i]
		if !secretHasPayload(*secret) {
			continue
		}
		source, ok := sourceByName[secret.Name]
		if !ok {
			continue
		}
		payloadHash := secretDataHash(secret.Data)
		if isDirectReplayableSecret(source, payloadHash) {
			secret.Data = nil
			secret.HasData = false
			continue
		}
		if !isRecipeReplayableSecret(*planFile, source, payloadHash) {
			continue
		}
		rendered, err := resolvePlanSecretSource(ctx, *planFile, source, fileStoreByPath)
		if err != nil || secretValueHash(rendered) != payloadHash {
			continue
		}
		secret.Data = nil
		secret.HasData = false
	}
}

func SetPlanSecretMode(planFile *PlanFile) {
	hasPayload := false
	hasReference := false
	hasRecipe := false
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for _, secret := range planFile.Plan.CreateSecrets {
		if secretHasPayload(secret) {
			hasPayload = true
			continue
		}
		if _, ok := sourceByName[secret.Name]; ok {
			hasReference = true
			if sourceByName[secret.Name].Recipe != nil {
				hasRecipe = true
			}
		}
	}
	switch {
	case hasPayload && hasReference:
		planFile.Secrets.Mode = PlanSecretModeMixed
	case hasRecipe:
		planFile.Secrets.Mode = PlanSecretModeRecipe
	case hasReference:
		planFile.Secrets.Mode = PlanSecretModeReference
	default:
		planFile.Secrets.Mode = PlanSecretModePayload
	}
}

func ValidatePlanFile(planFile PlanFile) error {
	if planFile.APIVersion != PlanFileAPIVersion {
		return fmt.Errorf("unsupported plan api_version %q", planFile.APIVersion)
	}
	mode := NormalizedPlanSecretMode(planFile)
	switch mode {
	case PlanSecretModePayload, PlanSecretModeReference, PlanSecretModeRecipe, PlanSecretModeMixed:
	default:
		return fmt.Errorf("unsupported plan secrets.mode %q", planFile.Secrets.Mode)
	}
	hasPayload := PlanHasSecretPayloads(planFile.Plan)
	hasReference := planHasReferenceSecrets(planFile)
	switch mode {
	case PlanSecretModePayload:
		if hasReference {
			return fmt.Errorf("plan secrets.mode payload cannot contain payloadless replay secrets")
		}
		if err := validatePayloadModeSecrets(planFile); err != nil {
			return err
		}
	case PlanSecretModeReference:
		if hasPayload {
			return fmt.Errorf("plan secrets.mode reference cannot contain secret payloads")
		}
		if !hasReference && len(planFile.Plan.CreateSecrets) > 0 {
			return fmt.Errorf("plan secrets.mode reference requires replay sources for created secrets")
		}
	case PlanSecretModeRecipe:
		if hasPayload {
			return fmt.Errorf("plan secrets.mode recipe cannot contain secret payloads")
		}
		if !hasReference && len(planFile.Plan.CreateSecrets) > 0 {
			return fmt.Errorf("plan secrets.mode recipe requires replay sources for created secrets")
		}
	case PlanSecretModeMixed:
		if !hasPayload || !hasReference {
			return fmt.Errorf("plan secrets.mode mixed requires both payload and replay secrets")
		}
	}
	if planHasOperations(planFile.Plan) && planAssumptionCount(planFile.Plan.Assumptions) == 0 {
		return fmt.Errorf("plan has operations but no recorded assumptions")
	}
	if err := validatePlanSourceInputs(planFile.SourceInputs); err != nil {
		return err
	}
	return validatePlanSecretSources(planFile)
}

func validatePayloadModeSecrets(planFile PlanFile) error {
	for _, secret := range planFile.Plan.CreateSecrets {
		if !secretHasPayload(secret) {
			return fmt.Errorf("plan secret %q has no payload", secret.Name)
		}
	}
	return nil
}

func planHasOperations(plan Plan) bool {
	return len(plan.CreateConfigs) > 0 ||
		len(plan.CreateSecrets) > 0 ||
		len(plan.CreateNetworks) > 0 ||
		len(plan.DeleteConfigs) > 0 ||
		len(plan.DeleteSecrets) > 0 ||
		len(plan.StackDeploys) > 0
}

func planAssumptionCount(assumptions PlanAssumptions) int {
	return len(assumptions.AbsentConfigs) +
		len(assumptions.AbsentSecrets) +
		len(assumptions.AbsentNetworks) +
		len(assumptions.AbsentServices) +
		len(assumptions.PresentConfigs) +
		len(assumptions.PresentSecrets) +
		len(assumptions.PresentServices)
}

func ResolvePlanSecretPayloads(ctx context.Context, planFile *PlanFile) error {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	fileStoreByPath := map[string]*secrets.Store{}
	for i := range planFile.Plan.CreateSecrets {
		secret := &planFile.Plan.CreateSecrets[i]
		if secretHasPayload(*secret) {
			continue
		}
		source, ok := sourceByName[secret.Name]
		if !ok {
			return fmt.Errorf("plan secret %q has no payload and no replay source", secret.Name)
		}
		payload, err := resolvePlanSecretSource(ctx, *planFile, source, fileStoreByPath)
		if err != nil {
			return fmt.Errorf("plan secret %q: %w", secret.Name, err)
		}
		secret.Data = []byte(payload)
		secret.HasData = true
	}
	return nil
}

func resolvePlanSecretSource(ctx context.Context, planFile PlanFile, source PlanSecretSource, fileStoreByPath map[string]*secrets.Store) (string, error) {
	if len(source.Dependencies) == 1 {
		dep := source.Dependencies[0]
		resolved, err := resolvePlanSecretDependency(ctx, planFile, dep, fileStoreByPath)
		if err != nil {
			return "", fmt.Errorf("dependency %q: %w", dep.Name, err)
		}
		if got := secretValueHash(resolved.Value); got != dep.Hash {
			return "", fmt.Errorf("dependency %q hash mismatch: got %s want %s", dep.Name, got, dep.Hash)
		}
		return resolved.Value, nil
	}
	return resolvePlanSecretRecipe(ctx, planFile, source, fileStoreByPath)
}

func resolvePlanSecretDependency(ctx context.Context, planFile PlanFile, dep PlanSecretDependency, fileStoreByPath map[string]*secrets.Store) (secrets.ResolvedSecret, error) {
	switch dep.Provider {
	case "file":
		return resolveFilePlanSecretDependency(planFile, dep, fileStoreByPath)
	default:
		auth := configAuth(dep.Auth)
		return secrets.ResolveFromMetadata(ctx, secrets.SecretMetadata{
			Provider: dep.Provider,
			Addr:     dep.Addr,
			Auth:     auth,
			Mount:    dep.Mount,
			Path:     dep.Path,
			Key:      dep.Key,
			Version:  dep.Version,
		})
	}
}

func resolvePlanSecretRecipe(ctx context.Context, planFile PlanFile, source PlanSecretSource, fileStoreByPath map[string]*secrets.Store) (string, error) {
	if source.Recipe == nil {
		return "", fmt.Errorf("cannot replay %d dependencies without recipe metadata", len(source.Dependencies))
	}
	if !isRecipeSourceReplayable(source) {
		return "", fmt.Errorf("recipe source is not replayable")
	}
	values, err := resolvePlanSecretRecipeDependencies(ctx, planFile, source, fileStoreByPath)
	if err != nil {
		return "", err
	}
	resolvedSource, err := resolvePlanRecipeSource(planFile, source.Recipe.Source)
	if err != nil {
		return "", err
	}
	scope := templates.Scope{
		Project:    source.Scope.Project,
		Deployment: source.Scope.Deployment,
		Stack:      source.Scope.Stack,
		Partition:  source.Scope.Partition,
		Service:    source.Scope.Service,
	}
	data := render.TemplateData{
		Project:    source.Scope.Project,
		Deployment: source.Scope.Deployment,
		Stack:      source.Scope.Stack,
		Partition:  source.Scope.Partition,
		Service:    source.Scope.Service,
	}
	rendered, _, err := templates.ResolveSourceWithMetadata(resolvedSource, scope, data, templates.New(planRecipeResolver{scope: source.Scope, values: values}), nil, "", planRecipeLoadOptions(planFile))
	if err != nil {
		return "", err
	}
	if got := secretValueHash(rendered); got != source.Recipe.RenderedHash {
		return "", fmt.Errorf("rendered hash mismatch: got %s want %s", got, source.Recipe.RenderedHash)
	}
	return rendered, nil
}

func resolvePlanSecretRecipeDependencies(ctx context.Context, planFile PlanFile, source PlanSecretSource, fileStoreByPath map[string]*secrets.Store) (map[string]string, error) {
	out := make(map[string]string, len(source.Dependencies))
	for _, dep := range source.Dependencies {
		resolved, err := resolvePlanSecretDependency(ctx, planFile, dep, fileStoreByPath)
		if err != nil {
			return nil, fmt.Errorf("dependency %q: %w", dep.Name, err)
		}
		if got := secretValueHash(resolved.Value); got != dep.Hash {
			return nil, fmt.Errorf("dependency %q hash mismatch: got %s want %s", dep.Name, got, dep.Hash)
		}
		out[planRecipeSecretKey(dep.Scope, dep.Name)] = resolved.Value
		out[planRecipeSecretKey(PlanScope{}, dep.Name)] = resolved.Value
	}
	return out, nil
}

func resolvePlanRecipeSource(planFile PlanFile, source string) (string, error) {
	parsed, ok, err := config.ParseGitSource(source)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("recipe source must be git-backed")
	}
	input, ok := planSourceInputForGitSource(planFile.SourceInputs, parsed)
	if !ok {
		return "", fmt.Errorf("recipe source metadata missing")
	}
	return config.ResolveSourceRef(config.SourceRef{
		URL:  input.URL,
		Ref:  input.Commit,
		Path: input.Path,
	}, "", planRecipeLoadOptions(planFile))
}

func planSourceInputForGitSource(inputs []PlanSourceInput, source config.GitSource) (PlanSourceInput, bool) {
	for _, input := range inputs {
		if input.Kind != "git" {
			continue
		}
		if input.URL == source.URL && input.Ref == source.Ref && input.Path == source.Path && input.Commit != "" && input.Subtree != "" {
			return input, true
		}
	}
	return PlanSourceInput{}, false
}

func planRecipeLoadOptions(planFile PlanFile) config.LoadOptions {
	return config.LoadOptions{CacheDir: planRecipeCacheDir(planFile)}
}

func planRecipeCacheDir(planFile PlanFile) string {
	for _, input := range planFile.Inputs {
		if input.Kind == "project" && input.Path != "" {
			return filepath.Join(filepath.Dir(input.Path), ".swarmcp", "sources")
		}
	}
	return ""
}

func resolveFilePlanSecretDependency(planFile PlanFile, dep PlanSecretDependency, fileStoreByPath map[string]*secrets.Store) (secrets.ResolvedSecret, error) {
	input, err := planSecretsInput(planFile)
	if err != nil {
		return secrets.ResolvedSecret{}, err
	}
	hash, err := fileSHA256(input.Path)
	if err != nil {
		return secrets.ResolvedSecret{}, fmt.Errorf("secrets input %q: %w", input.Path, err)
	}
	if hash != input.SHA256 {
		return secrets.ResolvedSecret{}, fmt.Errorf("secrets input %q sha256 mismatch: got %s want %s", input.Path, hash, input.SHA256)
	}
	store := fileStoreByPath[input.Path]
	if store == nil {
		store, err = secrets.Load(input.Path)
		if err != nil {
			return secrets.ResolvedSecret{}, fmt.Errorf("secrets input %q: %w", input.Path, err)
		}
		fileStoreByPath[input.Path] = store
	}
	value, ok := store.Values[dep.Key]
	if !ok {
		return secrets.ResolvedSecret{}, fmt.Errorf("%w: %s", secrets.ErrSecretNotFound, dep.Key)
	}
	return secrets.ResolvedSecret{
		Value: value,
		Metadata: secrets.SecretMetadata{
			Provider: "file",
			Key:      dep.Key,
		},
	}, nil
}

type planRecipeResolver struct {
	scope  PlanScope
	values map[string]string
}

func (r planRecipeResolver) ConfigValue(name string) (any, error) {
	return "", fmt.Errorf("config_value %q is not replayable in secret recipes", name)
}

func (r planRecipeResolver) ConfigRef(name string) (string, error) {
	return "", fmt.Errorf("config_ref %q is not replayable in secret recipes", name)
}

func (r planRecipeResolver) ConfigRefs(pattern string) ([]string, error) {
	return nil, fmt.Errorf("config_refs %q is not replayable in secret recipes", pattern)
}

func (r planRecipeResolver) SecretValue(name string) (string, error) {
	if value, ok := r.values[planRecipeSecretKey(r.scope, name)]; ok {
		return value, nil
	}
	if value, ok := r.values[planRecipeSecretKey(PlanScope{}, name)]; ok {
		return value, nil
	}
	return "", fmt.Errorf("%w: %s", secrets.ErrSecretNotFound, name)
}

func (r planRecipeResolver) SecretRef(name string) (string, error) {
	return "", fmt.Errorf("secret_ref %q is not replayable in secret recipes", name)
}

func (r planRecipeResolver) SecretRefs(pattern string) ([]string, error) {
	return nil, fmt.Errorf("secret_refs %q is not replayable in secret recipes", pattern)
}

func (r planRecipeResolver) RuntimeValue(args ...string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	return "", fmt.Errorf("runtime_value is not replayable in secret recipes")
}

func planRecipeSecretKey(scope PlanScope, name string) string {
	return strings.Join([]string{scope.Project, scope.Deployment, scope.Stack, scope.Partition, scope.Service, name}, "\x00")
}

func NormalizedPlanSecretMode(planFile PlanFile) string {
	mode := strings.TrimSpace(planFile.Secrets.Mode)
	if mode == "" {
		return PlanSecretModePayload
	}
	return mode
}

func planHasReferenceSecrets(planFile PlanFile) bool {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for _, secret := range planFile.Plan.CreateSecrets {
		if secretHasPayload(secret) {
			continue
		}
		if _, ok := sourceByName[secret.Name]; ok {
			return true
		}
	}
	return false
}

func validatePlanSecretSources(planFile PlanFile) error {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for _, secret := range planFile.Plan.CreateSecrets {
		if secretHasPayload(secret) {
			continue
		}
		source, ok := sourceByName[secret.Name]
		if !ok {
			return fmt.Errorf("plan secret %q has no payload and no replay source", secret.Name)
		}
		if len(source.Dependencies) == 0 {
			return fmt.Errorf("plan secret %q has no payload and no replay dependencies", secret.Name)
		}
		if len(source.Dependencies) > 1 && !isRecipeSourceReplayable(source) {
			return fmt.Errorf("plan secret %q has no payload and cannot replay %d dependencies without recipe metadata", secret.Name, len(source.Dependencies))
		}
		for _, dep := range source.Dependencies {
			if !isReplayableDependency(planFile, dep) {
				return fmt.Errorf("plan secret %q dependency %q is not replayable", secret.Name, dep.Name)
			}
		}
	}
	return nil
}

func validatePlanSourceInputs(inputs []PlanSourceInput) error {
	for _, input := range inputs {
		switch input.Kind {
		case "git":
			if input.URL == "" {
				return fmt.Errorf("plan git source input has no url")
			}
			if input.Commit == "" {
				return fmt.Errorf("plan git source input %q has no commit", input.URL)
			}
			if input.Subtree == "" {
				return fmt.Errorf("plan git source input %q has no subtree", input.URL)
			}
		default:
			return fmt.Errorf("unsupported plan source input kind %q", input.Kind)
		}
	}
	return nil
}

func PlanHasSecretPayloads(plan Plan) bool {
	for _, secret := range plan.CreateSecrets {
		if secretHasPayload(secret) {
			return true
		}
	}
	return false
}

func secretHasPayload(secret swarm.SecretSpec) bool {
	return secret.HasData || len(secret.Data) > 0
}

func planSecretSourcesByName(sources []PlanSecretSource) map[string]PlanSecretSource {
	out := make(map[string]PlanSecretSource, len(sources))
	for _, source := range sources {
		if source.SecretName == "" {
			continue
		}
		out[source.SecretName] = source
	}
	return out
}

func isDirectReplayableSecret(source PlanSecretSource, payloadHash string) bool {
	if len(source.Dependencies) != 1 {
		return false
	}
	dep := source.Dependencies[0]
	if dep.Hash == "" || dep.Hash != payloadHash {
		return false
	}
	switch dep.Provider {
	case "vault", "bao", "openbao":
		return isVaultReplayableDependency(dep)
	case "file":
		return dep.Key != ""
	default:
		return false
	}
}

func isRecipeReplayableSecret(planFile PlanFile, source PlanSecretSource, payloadHash string) bool {
	if len(source.Dependencies) <= 1 {
		return false
	}
	if source.Recipe == nil || source.Recipe.RenderedHash == "" || source.Recipe.RenderedHash != payloadHash {
		return false
	}
	if !isRecipeSourceReplayable(source) {
		return false
	}
	for _, dep := range source.Dependencies {
		if !isReplayableDependency(planFile, dep) {
			return false
		}
	}
	return true
}

func isRecipeSourceReplayable(source PlanSecretSource) bool {
	if source.Recipe == nil || source.Recipe.Source == "" || source.Recipe.RenderedHash == "" {
		return false
	}
	return config.IsGitSource(source.Recipe.Source)
}

func isReplayableDependency(planFile PlanFile, dep PlanSecretDependency) bool {
	if dep.Hash == "" {
		return false
	}
	switch dep.Provider {
	case "vault", "bao", "openbao":
		return isVaultReplayableDependency(dep)
	case "file":
		if dep.Key == "" {
			return false
		}
		_, err := planSecretsInput(planFile)
		return err == nil
	default:
		return false
	}
}

func isVaultReplayableDependency(dep PlanSecretDependency) bool {
	return dep.Addr != "" && dep.Mount != "" && dep.Path != "" && dep.Key != ""
}

func planSecretsInput(planFile PlanFile) (PlanInput, error) {
	var found *PlanInput
	for i := range planFile.Inputs {
		input := planFile.Inputs[i]
		if input.Kind != "secrets" {
			continue
		}
		if found != nil {
			return PlanInput{}, fmt.Errorf("plan has multiple secrets inputs")
		}
		found = &input
	}
	if found == nil || found.Path == "" || found.SHA256 == "" {
		return PlanInput{}, fmt.Errorf("plan has file-backed secrets but no secrets input fingerprint")
	}
	return *found, nil
}

func secretDataHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func secretValueHash(value string) string {
	return secretDataHash([]byte(value))
}

func configAuth(auth *config.AuthConfig) config.AuthConfig {
	if auth == nil {
		return config.AuthConfig{}
	}
	return *auth
}
