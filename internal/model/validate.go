package model

import (
	"errors"
	"fmt"
	"strconv"

	units "github.com/docker/go-units"
)

// ValidateConfigSpec ensures the config spec is safe for Swarm consumption.
func ValidateConfigSpec(cfg ConfigSpec) error {
	if cfg.Name == "" {
		return errors.New("config name is required")
	}
	if cfg.Target.Target == "" {
		return fmt.Errorf("config %s: target path is required", cfg.Name)
	}
	return nil
}

// ValidateSecretSpec ensures the secret spec is safe for Swarm consumption.
func ValidateSecretSpec(sec SecretSpec) error {
	if sec.Name == "" {
		return errors.New("secret name is required")
	}
	if sec.Target.Target == "" {
		return fmt.Errorf("secret %s: target path is required", sec.Name)
	}
	return nil
}

// ValidateServiceSpec ensures the service spec is compatible with Swarm.
func ValidateServiceSpec(svc ServiceSpec) error {
	if svc.Name == "" {
		return errors.New("service name is required")
	}
	if svc.Image == "" {
		return fmt.Errorf("service %s: image is required", svc.Name)
	}
	for _, cfg := range svc.Configs {
		if err := ValidateConfigSpec(cfg); err != nil {
			return err
		}
	}
	for _, sec := range svc.Secrets {
		if err := ValidateSecretSpec(sec); err != nil {
			return err
		}
	}
	// Validate resources can be parsed into Docker-native units when provided.
	if _, err := parseNanoCPUs(svc.Deployment.Resources.Limits.CPUs); err != nil {
		return fmt.Errorf("service %s limits.cpus: %w", svc.Name, err)
	}
	if _, err := parseNanoCPUs(svc.Deployment.Resources.Reservations.CPUs); err != nil {
		return fmt.Errorf("service %s reservations.cpus: %w", svc.Name, err)
	}
	if _, err := parseBytes(svc.Deployment.Resources.Limits.Memory); err != nil {
		return fmt.Errorf("service %s limits.memory: %w", svc.Name, err)
	}
	if _, err := parseBytes(svc.Deployment.Resources.Reservations.Memory); err != nil {
		return fmt.Errorf("service %s reservations.memory: %w", svc.Name, err)
	}
	return nil
}

// ValidateRenderedService validates both the service spec and its rendered assets.
func ValidateRenderedService(svc RenderedService) error {
	if err := ValidateServiceSpec(svc.Spec); err != nil {
		return err
	}
	for _, cfg := range svc.Configs {
		if cfg.Data == nil {
			return fmt.Errorf("config %s: rendered data is nil", cfg.Spec.Name)
		}
	}
	for _, sec := range svc.Secrets {
		if sec.Data == nil {
			return fmt.Errorf("secret %s: rendered data is nil", sec.Spec.Name)
		}
	}
	return nil
}

func parseNanoCPUs(in string) (int64, error) {
	if in == "" {
		return 0, nil
	}
	cpus, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return 0, err
	}
	return int64(cpus * 1e9), nil
}

func parseBytes(in string) (int64, error) {
	if in == "" {
		return 0, nil
	}
	return units.RAMInBytes(in)
}
