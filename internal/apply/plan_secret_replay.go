package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/cmmoran/swarmcp/internal/config"
	"github.com/cmmoran/swarmcp/internal/secrets"
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
	}
}

func ResolvePlanSecretPayloads(ctx context.Context, planFile *PlanFile) error {
	sourceByName := planSecretSourcesByName(planFile.SecretSources)
	for i := range planFile.Plan.CreateSecrets {
		secret := &planFile.Plan.CreateSecrets[i]
		if len(secret.Data) > 0 {
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
	}
	return nil
}

func PlanHasSecretPayloads(plan Plan) bool {
	for _, secret := range plan.CreateSecrets {
		if len(secret.Data) > 0 {
			return true
		}
	}
	return false
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
