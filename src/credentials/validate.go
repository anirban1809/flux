package credentials

import (
	llm "flux/src/llm/provider"
	"fmt"
	"time"
)

type Validator struct {
	providers map[llm.ProviderName]llm.Provider
	cache     map[llm.ProviderName]ValidationResult
	store     *Store
}

type ValidationStatus string

const (
	Valid         ValidationStatus = "Valid"
	Rejected      ValidationStatus = "Rejected"
	Unverified    ValidationStatus = "Unverified"
	Unchecked     ValidationStatus = "Unchecked"
	NotConfigured ValidationStatus = "Not Configured"
)

type ValidationResult struct {
	Status    ValidationStatus
	CheckedAt string
	LastError string
}

func NewValidator(
	providers map[llm.ProviderName]llm.Provider,
	store *Store,
) *Validator {
	validator := &Validator{
		providers: providers,
		cache:     make(map[llm.ProviderName]ValidationResult),
		store:     store,
	}

	for k := range providers {
		providerKey, ok := store.Get(k)
		if !ok {
			validator.cache[k] = ValidationResult{Status: NotConfigured}
			continue
		}
		status := ValidationStatus(providerKey.Status)
		if status == "" {
			status = Unchecked
		}
		validator.cache[k] = ValidationResult{
			Status:    status,
			CheckedAt: providerKey.LastValidated,
		}
	}

	return validator
}

func (v *Validator) Validate(
	name llm.ProviderName,
	key string,
) ValidationResult {
	provider, ok := v.providers[name]

	if !ok {
		return ValidationResult{
			Status:    Unchecked,
			LastError: "Unknown Provider",
			CheckedAt: time.Now().Format(time.RFC3339),
		}
	}

	authResult := provider.AuthCheck(key)

	if authResult.Status == 401 || authResult.Status == 403 {
		result := ValidationResult{
			Status:    Rejected,
			CheckedAt: time.Now().Format(time.RFC3339),
			LastError: authResult.ErrorMessage,
		}

		v.cache[name] = result
		return result
	}

	if authResult.Status == 200 {
		result := ValidationResult{
			Status:    Valid,
			CheckedAt: time.Now().Format(time.RFC3339),
		}
		v.cache[name] = result
		return result
	}

	result := ValidationResult{
		Status:    Unverified,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	v.cache[name] = result
	return result
}

func (v *Validator) ValidateLazy(name llm.ProviderName) ValidationResult {
	result := v.cache[name]

	if result.Status == Unchecked || result.Status == Unverified {
		creds, ok := v.store.Get(name)
		if !ok {
			return ValidationResult{
				Status:    NotConfigured,
				LastError: fmt.Sprintf("No key configured for %s", name),
				CheckedAt: time.Now().Format(time.RFC3339),
			}
		}

		result := v.Validate(name, creds.APIKey)
		v.cache[name] = result
		return result

	}

	return result
}

func (v *Validator) Invalidate(name llm.ProviderName) {
	delete(v.cache, name)
}

func (v Validator) Status(name llm.ProviderName) ValidationResult {
	return v.cache[name]
}
