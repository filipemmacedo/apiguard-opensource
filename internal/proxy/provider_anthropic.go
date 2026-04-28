package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type anthropicModelClient struct {
	httpClient *http.Client
}

func (c *anthropicModelClient) listModels(ctx context.Context, baseURL, apiKey string) ([]discoveredProviderModel, error) {
	var allModels []discoveredProviderModel
	hasMore := true
	afterID := ""

	for hasMore {
		endpoint := strings.TrimRight(baseURL, "/") + "/v1/models?limit=100"
		if afterID != "" {
			endpoint += "&after_id=" + afterID
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build anthropic models request: %w", err)
		}
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call anthropic models api: %w", err)
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
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Type        string `json:"type"`
				CreatedAt   string `json:"created_at"`
			} `json:"data"`
			HasMore bool    `json:"has_more"`
			LastID  *string `json:"last_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode anthropic models response: %w", err)
		}

		for _, model := range payload.Data {
			if strings.TrimSpace(model.ID) == "" {
				continue
			}
			displayName := model.DisplayName
			if displayName == "" {
				displayName = model.ID
			}
			allModels = append(allModels, discoveredProviderModel{
				ProviderModelID: model.ID,
				DisplayName:     displayName,
				Metadata: map[string]any{
					"type":       model.Type,
					"created_at": model.CreatedAt,
				},
			})
		}

		hasMore = payload.HasMore
		if payload.LastID != nil {
			afterID = *payload.LastID
		} else {
			hasMore = false
		}
	}

	sort.Slice(allModels, func(i, j int) bool {
		return allModels[i].ProviderModelID < allModels[j].ProviderModelID
	})
	return allModels, nil
}
