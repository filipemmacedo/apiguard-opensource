package proxy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type providerConfig struct {
	DefaultBaseURL string
	DisplayName    string
}

type modelLister interface {
	listModels(ctx context.Context, baseURL, apiKey string) ([]discoveredProviderModel, error)
}

var providerRegistry = map[string]providerConfig{
	"openai": {
		DefaultBaseURL: "https://api.openai.com",
		DisplayName:    "OpenAI",
	},
	"anthropic": {
		DefaultBaseURL: "https://api.anthropic.com",
		DisplayName:    "Anthropic",
	},
	"google": {
		DefaultBaseURL: "https://generativelanguage.googleapis.com",
		DisplayName:    "Google",
	},
}

func newModelLister(providerType string, httpClient *http.Client) modelLister {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	switch providerType {
	case "anthropic":
		return &anthropicModelClient{httpClient: httpClient}
	case "google":
		return &googleModelClient{httpClient: httpClient}
	default:
		return &openAIModelClient{httpClient: httpClient}
	}
}

func defaultBaseURLForProvider(providerType string) string {
	if cfg, ok := providerRegistry[providerType]; ok {
		return cfg.DefaultBaseURL
	}
	return ""
}

func supportedProviderTypes() []string {
	out := make([]string, 0, len(providerRegistry))
	for k := range providerRegistry {
		out = append(out, k)
	}
	return out
}

func validateProviderType(providerType string) error {
	if _, ok := providerRegistry[providerType]; !ok {
		return fmt.Errorf("unsupported provider type %q", providerType)
	}
	return nil
}
