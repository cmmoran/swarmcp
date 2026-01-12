package secrets

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Store struct {
	Values map[string]string `yaml:"values"`
}

func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var store Store
	if err := yaml.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	if store.Values == nil {
		store.Values = make(map[string]string)
	}

	return &store, nil
}

func Save(path string, store *Store) error {
	if store == nil {
		store = &Store{Values: map[string]string{}}
	}
	if store.Values == nil {
		store.Values = make(map[string]string)
	}
	data, err := yaml.Marshal(store)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
