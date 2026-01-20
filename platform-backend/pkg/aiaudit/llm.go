package aiaudit

import (
	"context"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

// callLLM dispatches the prompt to either the real LLM (if configured) or the mock implementation.
func callLLM(ctx context.Context, prompt string) (string, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_API_URL")
	model := os.Getenv("LLM_MODEL")

	if apiKey != "" {
		return callRealLLM(ctx, apiKey, baseURL, model, prompt)
	}

	return mockLLMCall(prompt)
}

// callRealLLM calls an OpenAI-compatible API.
func callRealLLM(ctx context.Context, apiKey, baseURL, model, prompt string) (string, error) {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	client := openai.NewClientWithConfig(config)

	// Use provided model or default to GPT-3.5-Turbo
	useModel := model
	if useModel == "" {
		useModel = openai.GPT3Dot5Turbo
	}

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: useModel,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.1, // Low temperature for deterministic analysis
		},
	)

	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}
