package proxy

import "testing"

func TestExtractUsagePresent(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"abc","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	usage := ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}

	if usage.PromptTokens == nil || *usage.PromptTokens != 10 {
		t.Fatalf("unexpected prompt_tokens: %#v", usage.PromptTokens)
	}
	if usage.CompletionTokens == nil || *usage.CompletionTokens != 20 {
		t.Fatalf("unexpected completion_tokens: %#v", usage.CompletionTokens)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != 30 {
		t.Fatalf("unexpected total_tokens: %#v", usage.TotalTokens)
	}
}

func TestExtractUsageAbsent(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"abc"}`)
	usage := ExtractUsage(body)
	if usage != nil {
		t.Fatalf("expected nil usage, got %#v", usage)
	}
}

func TestExtractUsageAnthropic(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"msg_01","usage":{"input_tokens":15,"output_tokens":25}}`)
	usage := ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}
	if usage.PromptTokens == nil || *usage.PromptTokens != 15 {
		t.Fatalf("unexpected prompt_tokens: %#v", usage.PromptTokens)
	}
	if usage.CompletionTokens == nil || *usage.CompletionTokens != 25 {
		t.Fatalf("unexpected completion_tokens: %#v", usage.CompletionTokens)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != 40 {
		t.Fatalf("unexpected total_tokens: %#v", usage.TotalTokens)
	}
}

func TestExtractUsageGoogleGemini(t *testing.T) {
	t.Parallel()

	body := []byte(`{"candidates":[{}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"totalTokenCount":20}}`)
	usage := ExtractUsage(body)
	if usage == nil {
		t.Fatal("expected usage, got nil")
	}
	if usage.PromptTokens == nil || *usage.PromptTokens != 12 {
		t.Fatalf("unexpected prompt_tokens: %#v", usage.PromptTokens)
	}
	if usage.CompletionTokens == nil || *usage.CompletionTokens != 8 {
		t.Fatalf("unexpected completion_tokens: %#v", usage.CompletionTokens)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != 20 {
		t.Fatalf("unexpected total_tokens: %#v", usage.TotalTokens)
	}
}
