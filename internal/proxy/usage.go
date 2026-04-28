package proxy

import "encoding/json"

type Usage struct {
	PromptTokens     *int64
	CompletionTokens *int64
	TotalTokens      *int64
}

func ExtractUsage(responseBody []byte) *Usage {
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return nil
	}

	// Google Gemini: top-level usageMetadata with promptTokenCount / candidatesTokenCount / totalTokenCount
	if meta, ok := payload["usageMetadata"].(map[string]any); ok {
		prompt := toInt64Ptr(meta["promptTokenCount"])
		completion := toInt64Ptr(meta["candidatesTokenCount"])
		total := toInt64Ptr(meta["totalTokenCount"])
		if prompt != nil || completion != nil || total != nil {
			if total == nil && prompt != nil && completion != nil {
				sum := *prompt + *completion
				total = &sum
			}
			return &Usage{
				PromptTokens:     prompt,
				CompletionTokens: completion,
				TotalTokens:      total,
			}
		}
	}

	rawUsage, ok := payload["usage"]
	if !ok {
		return nil
	}

	usageMap, ok := rawUsage.(map[string]any)
	if !ok {
		return nil
	}

	// OpenAI format: prompt_tokens / completion_tokens / total_tokens
	if pt := toInt64Ptr(usageMap["prompt_tokens"]); pt != nil {
		return &Usage{
			PromptTokens:     pt,
			CompletionTokens: toInt64Ptr(usageMap["completion_tokens"]),
			TotalTokens:      toInt64Ptr(usageMap["total_tokens"]),
		}
	}

	// Anthropic format: input_tokens / output_tokens
	input := toInt64Ptr(usageMap["input_tokens"])
	output := toInt64Ptr(usageMap["output_tokens"])
	if input != nil || output != nil {
		var total *int64
		if input != nil && output != nil {
			sum := *input + *output
			total = &sum
		}
		return &Usage{
			PromptTokens:     input,
			CompletionTokens: output,
			TotalTokens:      total,
		}
	}

	return nil
}

func toInt64Ptr(v any) *int64 {
	switch value := v.(type) {
	case float64:
		n := int64(value)
		return &n
	case int:
		n := int64(value)
		return &n
	case int64:
		n := value
		return &n
	default:
		return nil
	}
}
