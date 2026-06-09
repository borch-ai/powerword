package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"powerword/internal/config"
)

// ModelRouter is responsible for determining which LLM model to use
// for a given prompt based on rules, explicit prefixes, or classification.
type ModelRouter struct {
	defaultModel     string
	routes           map[string]string // Map of regex pattern to model name
	classifierClient LLMClient
	classifierModel  string
}

// NewRouter creates a new ModelRouter.
func NewRouter(cfg *config.Config, classifierClient LLMClient) *ModelRouter {
	return &ModelRouter{
		defaultModel:     cfg.Model,
		routes:           cfg.Route,
		classifierClient: classifierClient,
		classifierModel:  cfg.ClassifierModel,
	}
}

// Route determines the appropriate model for the given prompt.
// It returns the target model name, a potentially modified prompt (if an explicit prefix was removed), and an error.
func (r *ModelRouter) Route(ctx context.Context, prompt string) (string, string, error) {
	if prompt == "" {
		return r.defaultModel, prompt, nil
	}

	// 1. Explicit Prefixes: e.g., "@gemini-1.5-pro list files"
	if strings.HasPrefix(prompt, "@") {
		parts := strings.SplitN(prompt, " ", 2)
		if len(parts) > 0 {
			modelOverride := strings.TrimPrefix(parts[0], "@")
			if modelOverride != "" {
				newPrompt := ""
				if len(parts) > 1 {
					newPrompt = strings.TrimSpace(parts[1])
				}
				return modelOverride, newPrompt, nil
			}
		}
	}

	// 2. Rule-based Regex Matching
	for pattern, targetModel := range r.routes {
		matched, err := regexp.MatchString(pattern, prompt)
		if err != nil {
			// Skip invalid regexes
			continue
		}
		if matched {
			return targetModel, prompt, nil
		}
	}

	// 3. Prompt-based Classification
	if r.classifierClient != nil && r.classifierModel != "" {
		sysPrompt := fmt.Sprintf(`You are an intelligent router. Evaluate the following prompt's complexity and domain.
Available models are: %s.
Respond with ONLY the exact name of the target model from the list above, and nothing else.`,
			r.getAvailableModelsStr())

		messages := []Message{
			{Role: RoleSystem, Content: sysPrompt},
			{Role: RoleUser, Content: prompt},
		}

		resp, err := r.classifierClient.Generate(ctx, messages, nil)
		if err == nil && resp != nil && resp.Content != "" {
			targetModel := strings.TrimSpace(resp.Content)
			// Simple validation to ensure the classifier didn't return a whole paragraph
			if !strings.Contains(targetModel, " ") && targetModel != "" {
				return targetModel, prompt, nil
			}
		}
	}

	// 4. Fallback
	return r.defaultModel, prompt, nil
}

func (r *ModelRouter) getAvailableModelsStr() string {
	models := []string{r.defaultModel}
	for _, m := range r.routes {
		models = append(models, m)
	}
	// unique them
	unique := make(map[string]bool)
	var final []string
	for _, m := range models {
		if !unique[m] {
			unique[m] = true
			final = append(final, m)
		}
	}
	return strings.Join(final, ", ")
}
