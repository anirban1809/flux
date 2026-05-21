package credentials

import (
	"bytes"
	"errors"
	"flux/src/config"
	llm "flux/src/llm/provider"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type CredSource int

const (
	file CredSource = iota
	env
)

type Store struct {
	Providers map[llm.ProviderName]ProviderKey
	Source    map[llm.ProviderName]CredSource // file | env
}

type ProviderCreds struct {
	APIKey string
}

type Config struct {
	Providers map[string]ProviderKey `toml:"providers"`
}

type ProviderKey struct {
	APIKey        string `toml:"api_key"`
	Status        string `toml:"status"`
	LastValidated string `toml:"last_validated"`
}

func NewStore() *Store {
	return &Store{
		Providers: make(map[llm.ProviderName]ProviderKey),
		Source:    make(map[llm.ProviderName]CredSource),
	}
}

func (s *Store) Load() error {
	var cfg Config
	_, err := toml.DecodeFile(config.Cfg.CredentialsPath, &cfg)

	switch {
	case err == nil:
		for name, value := range cfg.Providers {
			providerName, err := llm.GetProviderName(name)
			if err != nil {
				continue
			}
			s.Providers[providerName] = ProviderKey{
				APIKey:        value.APIKey,
				Status:        value.Status,
				LastValidated: value.LastValidated,
			}
			s.Source[providerName] = file
		}
	case errors.Is(err, os.ErrNotExist):
		// No credentials file yet — env vars below are the only source.
	default:
		return err
	}

	//env override

	supportedProviders := llm.GetSupportedProviders()
	for _, provider := range supportedProviders {
		envVar, err := llm.GetProviderEnvVar(provider)
		if err != nil {
			continue
		}
		envKey := os.Getenv(envVar)
		if envKey == "" {
			continue
		}
		entry := ProviderKey{APIKey: envKey}
		if existing, ok := s.Providers[provider]; ok &&
			existing.APIKey == envKey {
			entry.Status = existing.Status
			entry.LastValidated = existing.LastValidated
		}
		s.Providers[provider] = entry
		s.Source[provider] = env
	}

	return nil
}

func (s *Store) Remove(name llm.ProviderName) error {
	cfg := Config{Providers: make(map[string]ProviderKey)}
	for k, v := range s.Providers {
		if k != name {
			cfg.Providers[string(k)] = ProviderKey{APIKey: v.APIKey}
		}
	}
	var content bytes.Buffer

	if err := toml.NewEncoder(&content).Encode(cfg); err != nil {
		return err
	}

	err := AtomicWrite(config.Cfg.CredentialsPath, content.Bytes())

	if err != nil {
		return err
	}
	delete(s.Providers, name)
	return nil
}

func (s *Store) Set(name llm.ProviderName, key string) error {
	cfg := Config{Providers: make(map[string]ProviderKey)}
	for k, v := range s.Providers {
		cfg.Providers[string(k)] = v
	}
	entry := ProviderKey{
		APIKey:        key,
		Status:        "Valid",
		LastValidated: time.Now().UTC().Format(time.RFC3339),
	}
	cfg.Providers[string(name)] = entry

	var content bytes.Buffer

	if err := toml.NewEncoder(&content).Encode(cfg); err != nil {
		return err
	}

	err := AtomicWrite(config.Cfg.CredentialsPath, content.Bytes())

	if err != nil {
		return err
	}

	s.Source[name] = file
	s.Providers[name] = entry

	return nil
}

func (s *Store) Get(name llm.ProviderName) (ProviderKey, bool) {
	creds, ok := s.Providers[name]
	if !ok || creds.APIKey == "" {
		return ProviderKey{}, false
	}
	return creds, true
}

func (s *Store) ConfiguredProviders() []llm.ProviderName {
	var providers []llm.ProviderName

	for name, value := range s.Providers {
		if value.APIKey != "" {
			providers = append(providers, name)
		}
	}

	return providers
}
