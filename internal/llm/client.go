package llm

import "context"

// LLMClient abstracts interaction with LLM providers.
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
