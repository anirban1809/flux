package llm

type Registry struct {
	Providers map[ProviderName]Provider
}

func NewRegistry() Registry {
	providers := map[ProviderName]Provider{
		OpenAIProvider:        &OpenAI{},
		OpenRouterAPIProvider: &OpenRouterProvider{},
		AnthropicProvider:     &Anthropic{},
		BedrockProvider:       &Bedrock{},
	}

	return Registry{
		Providers: providers,
	}
}

func (r Registry) GetProvider(name ProviderName) Provider {
	return r.Providers[name]
}

func (r Registry) ProviderList() []ProviderName {
	return []ProviderName{OpenAIProvider, OpenRouterAPIProvider, AnthropicProvider, BedrockProvider}
}

func (r Registry) ContextWindowFor(providerName ProviderName, modelID string) int {
	provider := r.GetProvider(providerName)
	if provider == nil {
		return 0
	}
	for _, m := range provider.Models() {
		if m.ID == modelID {
			return m.ContextWindow
		}
	}
	return 0
}

// CostFor returns the input/output cost per 1M tokens (USD) for the given
// provider+model. Returns zeros if the model is not registered or pricing
// is unknown.
func (r Registry) CostFor(providerName ProviderName, modelID string) (inputPerMillion, outputPerMillion float64) {
	provider := r.GetProvider(providerName)
	if provider == nil {
		return 0, 0
	}
	for _, m := range provider.Models() {
		if m.ID == modelID {
			return m.InputCostPerMillion, m.OutputCostPerMillion
		}
	}
	return 0, 0
}

// CacheRatesFor returns the cache-read and cache-write cost per 1M tokens
// (USD) for the given provider+model. Returns zeros if the model is not
// registered or does not support prompt caching.
func (r Registry) CacheRatesFor(providerName ProviderName, modelID string) (readPerMillion, writePerMillion float64) {
	provider := r.GetProvider(providerName)
	if provider == nil {
		return 0, 0
	}
	for _, m := range provider.Models() {
		if m.ID == modelID {
			return m.CacheReadCostPerMillion, m.CacheWriteCostPerMillion
		}
	}
	return 0, 0
}
