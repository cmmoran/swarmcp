package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
	"github.com/cmmoran/swarmcp/internal/swarm"
)

const (
	PlanSecretModePayload   = "payload"
	PlanSecretModeReference = "reference"
	PlanSecretModeMixed     = "mixed"
)

func OmitReplayableSecretPayloads(plan *Plan, sources []PlanSecretSource) {
	sourceByName := planSecretSourcesByName(sources)
	for i := range plan.CreateSecrets {
		secret := &plan.CreateSecrets[i]
		source, ok := sourceByName[secret.Name]
		if !ok || !isDirectReplayableSecret(source, secretDataHash(secret.Data)) {
			continue
		}
		secret.Data = nil
		secret.HasData = false
	}
}

func SetPlanSecretMode(planFile *PlanFile) {
	hasPayload := false
	hasReference := false
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for _, secret := range planFile.Plan.CreateSecrets {
		if secretHasPayload(secret) {
			hasPayload = true
			continue
		}
		if _, ok := sourceByName[secret.Name]; ok {
			hasReference = true
		}
	}
	switch {
	case hasPayload && hasReference:
		planFile.Secrets.Mode = PlanSecretModeMixed
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
	case PlanSecretModePayload, PlanSecretModeReference, PlanSecretModeMixed:
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
	case PlanSecretModeReference:
		if hasPayload {
			return fmt.Errorf("plan secrets.mode reference cannot contain secret payloads")
		}
		if !hasReference && len(planFile.Plan.CreateSecrets) > 0 {
			return fmt.Errorf("plan secrets.mode reference requires replay sources for created secrets")
		}
	case PlanSecretModeMixed:
		if !hasPayload || !hasReference {
			return fmt.Errorf("plan secrets.mode mixed requires both payload and replay secrets")
		}
	}
	return validatePlanSecretSources(planFile)
}

func ResolvePlanSecretPayloads(ctx context.Context, planFile *PlanFile) error {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for i := range planFile.Plan.CreateSecrets {
		secret := &planFile.Plan.CreateSecrets[i]
		if secretHasPayload(*secret) {
			continue
		}
		source, ok := sourceByName[secret.Name]
		if !ok {
			return fmt.Errorf("plan secret %q has no payload and no replay source", secret.Name)
		}
		if len(source.Dependencies) != 1 {
			return fmt.Errorf("plan secret %q has no payload and cannot replay %d dependencies", secret.Name, len(source.Dependencies))
		}
		dep := source.Dependencies[0]
		auth := configAuth(dep.Auth)
		resolved, err := secrets.ResolveFromMetadata(ctx, secrets.SecretMetadata{
			Provider: dep.Provider,
			Addr:     dep.Addr,
			Auth:     auth,
			Mount:    dep.Mount,
			Path:     dep.Path,
			Key:      dep.Key,
			Version:  dep.Version,
		})
		if err != nil {
			return fmt.Errorf("plan secret %q dependency %q: %w", secret.Name, dep.Name, err)
		}
		if got := secretValueHash(resolved.Value); got != dep.Hash {
			return fmt.Errorf("plan secret %q dependency %q hash mismatch: got %s want %s", secret.Name, dep.Name, got, dep.Hash)
		}
		secret.Data = []byte(resolved.Value)
		secret.HasData = true
	}
	return nil
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
		if len(source.Dependencies) != 1 {
			return fmt.Errorf("plan secret %q has no payload and cannot replay %d dependencies", secret.Name, len(source.Dependencies))
		}
		dep := source.Dependencies[0]
		if !isReplayableDependency(dep) {
			return fmt.Errorf("plan secret %q dependency %q is not replayable", secret.Name, dep.Name)
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
		return isReplayableDependency(dep)
	default:
		return false
	}
}

func isReplayableDependency(dep PlanSecretDependency) bool {
	if dep.Hash == "" {
		return false
	}
	switch dep.Provider {
	case "vault", "bao", "openbao":
		return dep.Addr != "" && dep.Mount != "" && dep.Path != "" && dep.Key != ""
	default:
		return false
	}
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
