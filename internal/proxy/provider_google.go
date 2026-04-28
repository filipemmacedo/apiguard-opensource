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

type googleModelClient struct {
	httpClient *http.Client
}

func (c *googleModelClient) listModels(ctx context.Context, baseURL, apiKey string) ([]discoveredProviderModel, error) {
	var allModels []discoveredProviderModel
	pageToken := ""

	for {
		endpoint := strings.TrimRight(baseURL, "/") + "/v1beta/models?pageSize=100&key=" + apiKey
		if pageToken != "" {
			endpoint += "&pageToken=" + pageToken
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build google models request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call google models api: %w", err)
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
			Models []struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
				Version     string `json:"version"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode google models response: %w", err)
		}

		for _, model := range payload.Models {
			name := model.Name
			// Strip "models/" prefix from Google's resource name to get the model ID.
			modelID := strings.TrimPrefix(name, "models/")
			if strings.TrimSpace(modelID) == "" {
				continue
			}
			displayName := model.DisplayName
			if displayName == "" {
				displayName = modelID
			}
			allModels = append(allModels, discoveredProviderModel{
				ProviderModelID: modelID,
				DisplayName:     displayName,
				Metadata: map[string]any{
					"description": model.Description,
					"version":     model.Version,
				},
			})
		}

		if payload.NextPageToken == "" {
			break
		}
		pageToken = payload.NextPageToken
	}

	sort.Slice(allModels, func(i, j int) bool {
		return allModels[i].ProviderModelID < allModels[j].ProviderModelID
	})
	return allModels, nil
}
