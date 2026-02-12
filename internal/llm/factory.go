package llm

import (
	"context"
	"fmt"
	"strings"
)

// MultiProviderClient implements LLMClient by dispatching to the appropriate
// provider based on the ModelConfig.Provider field.
//
// This allows a single LLMActivities instance to support multiple providers
// without knowing which one will be used at registration time.
type MultiProviderClient struct {
	openai    *OpenAIClient
	anthropic *AnthropicClient
}

// NewMultiProviderClient creates a client that can dispatch to multiple providers.
func NewMultiProviderClient() *MultiProviderClient {
	return &MultiProviderClient{
		openai:    NewOpenAIClient(),
		anthropic: NewAnthropicClient(),
	}
}

// Call dispatches to the appropriate provider based on ModelConfig.Provider.
func (c *MultiProviderClient) Call(ctx context.Context, request LLMRequest) (LLMResponse, error) {
	// Default to OpenAI if provider not specified (backward compatibility)
	provider := request.ModelConfig.Provider
	if provider == "" {
		provider = "openai"
	}

	switch provider {
	case "openai":
		return c.openai.Call(ctx, request)
	case "anthropic":
		return c.anthropic.Call(ctx, request)
	default:
		return LLMResponse{}, fmt.Errorf("unsupported LLM provider: %s (supported: openai, anthropic)", provider)
	}
}

// Compact dispatches to the appropriate provider based on CompactRequest.Model.
func (c *MultiProviderClient) Compact(ctx context.Context, request CompactRequest) (CompactResponse, error) {
	provider := detectProviderFromModel(request.Model)

	switch provider {
	case "openai":
		resp, err := c.openai.Compact(ctx, request)
		if err != nil {
			// Fall back to local compaction via Anthropic-style summarization
			return c.anthropic.Compact(ctx, request)
		}
		return resp, nil
	case "anthropic":
		return c.anthropic.Compact(ctx, request)
	default:
		return c.anthropic.Compact(ctx, request)
	}
}

// detectProviderFromModel infers the provider from the model name.
func detectProviderFromModel(model string) string {
	if strings.HasPrefix(model, "claude") {
		return "anthropic"
	}
	return "openai"
}

// NewLLMClient creates the appropriate LLM client based on provider name.
// This is a convenience function for cases where you know the provider at init time.
//
// For most use cases, prefer NewMultiProviderClient() which can handle multiple providers.
func NewLLMClient(provider string) (LLMClient, error) {
	switch provider {
	case "openai", "":
		return NewOpenAIClient(), nil
	case "anthropic":
		return NewAnthropicClient(), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s (supported: openai, anthropic)", provider)
	}
}
