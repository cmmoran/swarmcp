package store

import (
	"errors"
	"os"

	"github.com/cmmoran/swarmcp/internal/spec"
)

var ErrUnknownBackend = errors.New("unknown secrets backend")

func New(cfg spec.SecretsProviderSpec) (Client, error) {
	switch cfg.Backend {
	case spec.BackendAuto:
		switch {
		case os.Getenv("BAO_ADDR") != "":
			return newBao(cfg)
		case os.Getenv("VAULT_ADDR") != "":
			return newVault(cfg)
		default:
			return newBao(cfg)
		}

	case spec.BackendBao:
		return newBao(cfg)
	case spec.BackendVault:
		return newVault(cfg)
	default:
		return nil, ErrUnknownBackend
	}
}
