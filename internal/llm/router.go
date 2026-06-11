package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/borch-ai/powerword/internal/config"
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

	if target, newPrompt := r.checkExplicitPrefix(prompt); target != "" {
		return target, newPrompt, nil
	}

	if target := r.checkRegexRules(prompt); target != "" {
		return target, prompt, nil
	}

	if target := r.checkClassifier(ctx, prompt); target != "" {
		return target, prompt, nil
	}

	return r.defaultModel, prompt, nil
}

func (r *ModelRouter) checkExplicitPrefix(prompt string) (string, string) {
	if !strings.HasPrefix(prompt, "@") {
		return "", prompt
	}
	parts := strings.SplitN(prompt, " ", 2)
	if len(parts) == 0 {
		return "", prompt
	}
	modelOverride := strings.TrimPrefix(parts[0], "@")
	if modelOverride == "" {
		return "", prompt
	}
	newPrompt := ""
	if len(parts) > 1 {
		newPrompt = strings.TrimSpace(parts[1])
	}
	return modelOverride, newPrompt
}

func (r *ModelRouter) checkRegexRules(prompt string) string {
	for pattern, targetModel := range r.routes {
		matched, err := regexp.MatchString(pattern, prompt)
		if err == nil && matched {
			return targetModel
		}
	}
	return ""
}

func (r *ModelRouter) checkClassifier(ctx context.Context, prompt string) string {
	if r.classifierClient == nil || r.classifierModel == "" {
		return ""
	}
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
		if !strings.Contains(targetModel, " ") && targetModel != "" {
			return targetModel
		}
	}
	return ""
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
