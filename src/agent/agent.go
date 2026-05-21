package agent

import (
	"flux/src/config"
	"flux/src/credentials"
	llm "flux/src/llm/provider"
	"flux/src/tools"
	"fmt"
)

type Agent struct {
	Name         string
	SystemPrompt string
	Conversation llm.Conversation
	LastResponse llm.Message
	Tools        *[]tools.Tool
	Registry     *llm.Registry
	initial      bool
	Validator    credentials.Validator
	Config       config.Config
}

func NewAgent(
	systemPrompt string,
	tools *[]tools.Tool,
	registry *llm.Registry,
	validator *credentials.Validator,
) Agent {
	return Agent{
		SystemPrompt: systemPrompt,
		Tools:        tools,
		initial:      false,
		Validator:    *validator,
		Registry:     registry,
	}
}

func (a *Agent) RestoreConversation(messages []llm.Message) {
	seeded := []llm.Message{{Role: "system", Content: a.SystemPrompt}}
	seeded = append(seeded, messages...)
	a.Conversation = llm.Conversation{
		Messages: seeded,
		Tools:    *a.Tools,
	}
	a.initial = true
}

func (a *Agent) RunStep(messages ...llm.Message) (*llm.Conversation, error) {
	if !a.initial {
		a.Conversation = llm.Conversation{
			Messages: []llm.Message{
				{
					Content: a.SystemPrompt,
					Role:    "system",
				},
			},
			Tools: *a.Tools,
		}

		a.initial = true
	}

	a.Conversation.Messages = append(a.Conversation.Messages, messages...)
	a.Conversation.Tools = *a.Tools

	if config.Cfg.ActiveProviderName == "" {
		return nil, fmt.Errorf(
			"Error: No active provider configured, configure a provider from /providers to proceed",
		)
	}

	validationResult := a.Validator.ValidateLazy(
		llm.ProviderName(config.Cfg.ActiveProviderName),
	)

	if validationResult.Status == credentials.Rejected {
		return nil, fmt.Errorf(
			"Error: Failed to validate credentials for %s",
			config.Cfg.ActiveProviderName,
		)
	}

	if validationResult.Status == credentials.NotConfigured {
		return nil, fmt.Errorf(
			"Error: No credentials have been configured for %s",
			config.Cfg.ActiveProviderName,
		)
	}

	conv, err := a.Chat(&a.Conversation)

	if err != nil {
		return nil, err
	}

	a.Conversation = *conv
	return conv, nil
}

func (a *Agent) Chat(prev *llm.Conversation) (*llm.Conversation, error) {
	var chatRequest llm.ChatRequest

	chatRequest.Messages = prev.Messages
	chatRequest.Model = config.Cfg.CurrentModel
	chatRequest.Tools = prev.Tools

	currentProvider := a.Registry.GetProvider(
		llm.ProviderName(config.Cfg.ActiveProviderName),
	)

	value, err := currentProvider.Complete(chatRequest)

	if err != nil {
		return nil, err
	}

	usage := value.Usage
	value.Message.Usage = &usage
	prev.Messages = append(prev.Messages, value.Message)
	prev.Usage = value.Usage

	return prev, nil
}
