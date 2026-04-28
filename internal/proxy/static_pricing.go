package proxy

// staticModelPrices maps model IDs to their estimated pricing in USD per 1M tokens.
// These are approximate public prices — update via PR as providers change rates.
var staticModelPrices = map[string]modelPrice{
	// OpenAI
	"gpt-4o":                {InputPer1MUSD: 2.50, OutputPer1MUSD: 10.00},
	"gpt-4o-mini":           {InputPer1MUSD: 0.15, OutputPer1MUSD: 0.60},
	"gpt-4-turbo":           {InputPer1MUSD: 10.00, OutputPer1MUSD: 30.00},
	"gpt-4-turbo-preview":   {InputPer1MUSD: 10.00, OutputPer1MUSD: 30.00},
	"gpt-4":                 {InputPer1MUSD: 30.00, OutputPer1MUSD: 60.00},
	"gpt-3.5-turbo":         {InputPer1MUSD: 0.50, OutputPer1MUSD: 1.50},
	"gpt-3.5-turbo-0125":    {InputPer1MUSD: 0.50, OutputPer1MUSD: 1.50},
	"o1":                    {InputPer1MUSD: 15.00, OutputPer1MUSD: 60.00},
	"o1-mini":               {InputPer1MUSD: 1.10, OutputPer1MUSD: 4.40},
	"o3-mini":               {InputPer1MUSD: 1.10, OutputPer1MUSD: 4.40},

	// Anthropic (via OpenAI-compatible endpoints)
	"claude-3-5-sonnet-20241022": {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00},
	"claude-3-5-sonnet-20240620": {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00},
	"claude-3-5-haiku-20241022":  {InputPer1MUSD: 0.80, OutputPer1MUSD: 4.00},
	"claude-3-opus-20240229":     {InputPer1MUSD: 15.00, OutputPer1MUSD: 75.00},
	"claude-3-haiku-20240307":    {InputPer1MUSD: 0.25, OutputPer1MUSD: 1.25},
	"claude-3-sonnet-20240229":   {InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00},

	// Google (via compatible endpoints)
	"gemini-1.5-pro":   {InputPer1MUSD: 1.25, OutputPer1MUSD: 5.00},
	"gemini-1.5-flash": {InputPer1MUSD: 0.075, OutputPer1MUSD: 0.30},
	"gemini-2.0-flash": {InputPer1MUSD: 0.10, OutputPer1MUSD: 0.40},

	// Mistral
	"mistral-large-latest":  {InputPer1MUSD: 3.00, OutputPer1MUSD: 9.00},
	"mistral-small-latest":  {InputPer1MUSD: 0.20, OutputPer1MUSD: 0.60},
	"codestral-latest":      {InputPer1MUSD: 1.00, OutputPer1MUSD: 3.00},
}

type modelPrice struct {
	InputPer1MUSD  float64
	OutputPer1MUSD float64
}

// staticModelPricing returns pricing for a model from the static table.
// Returns (price, true) if found, (zero, false) if the model is unknown.
func staticModelPricing(modelID string) (modelPrice, bool) {
	p, ok := staticModelPrices[modelID]
	return p, ok
}
