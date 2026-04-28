package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type providerValidationError struct {
	message string
}

func (e providerValidationError) Error() string {
	return e.message
}

type openAIModelClient struct {
	httpClient *http.Client
}

func newOpenAIModelClient(httpClient *http.Client) *openAIModelClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &openAIModelClient{httpClient: httpClient}
}

func (c *openAIModelClient) listModels(ctx context.Context, baseURL, apiKey string) ([]discoveredProviderModel, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build openai models request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call openai models api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, providerValidationError{message: "provider api key is invalid"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("provider model discovery failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode openai models response: %w", err)
	}

	models := make([]discoveredProviderModel, 0, len(payload.Data))
	for _, model := range payload.Data {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		models = append(models, discoveredProviderModel{
			ProviderModelID: model.ID,
			DisplayName:     model.ID,
			Metadata: map[string]any{
				"owned_by": model.OwnedBy,
				"created":  model.Created,
			},
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ProviderModelID < models[j].ProviderModelID
	})
	return models, nil
}
