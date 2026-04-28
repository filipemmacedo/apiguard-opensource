package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"apiguard/internal/config"
)

const (
	piiActionObserve = "observe"
	piiActionBlock   = "block"

	piiDirectionIngress = "ingress"
	piiDirectionEgress  = "egress"
)

type piiPolicyRecord struct {
	EntityType string
	Enabled    bool
	Action     string
	UpdatedBy  string
	UpdatedAt  time.Time
}

type piiFindingRecord struct {
	ID          int64
	TenantID    string
	RequestID   string
	Timestamp   time.Time
	Direction   string
	EntityType  string
	Action      string
	Fingerprint string
	Count       int64
}

type piiRequestSummary struct {
	IngressFindingCount int64    `json:"ingress_finding_count"`
	EgressFindingCount  int64    `json:"egress_finding_count"`
	EntityTypes         []string `json:"entity_types,omitempty"`
	Actions             []string `json:"actions,omitempty"`
}

type piiTenantSummary struct {
	FlaggedRequestCount int64    `json:"flagged_request_count"`
	IngressFindingCount int64    `json:"ingress_finding_count"`
	EgressFindingCount  int64    `json:"egress_finding_count"`
	EntityTypes         []string `json:"entity_types,omitempty"`
	LastDetectedAt      *string  `json:"last_detected_at,omitempty"`
}

type piiEntityDefinition struct {
	DisplayName string
	Detect      func(string) []string
	Normalize   func(string) string
}

var (
	emailPIIPattern = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	phonePIIPattern = regexp.MustCompile(`(?:\+|00)?[0-9][0-9\-\s\(\)]{7,}[0-9]`)
	cardPIIPattern  = regexp.MustCompile(`(?:\d[ -]?){13,19}`)
)

var piiEntityDefinitions = map[string]piiEntityDefinition{
	"credit_card": {
		DisplayName: "Credit Card",
		Detect: func(text string) []string {
			matches := cardPIIPattern.FindAllString(text, -1)
			out := make([]string, 0, len(matches))
			for _, match := range matches {
				digits := digitsOnly(match)
				if len(digits) < 13 || len(digits) > 19 {
					continue
				}
				if !passesLuhn(digits) {
					continue
				}
				out = append(out, digits)
			}
			return out
		},
		Normalize: digitsOnly,
	},
	"email_address": {
		DisplayName: "Email Address",
		Detect: func(text string) []string {
			return emailPIIPattern.FindAllString(text, -1)
		},
		Normalize: func(value string) string {
			return strings.ToLower(strings.TrimSpace(value))
		},
	},
	"phone_number": {
		DisplayName: "Phone Number",
		Detect: func(text string) []string {
			matches := phonePIIPattern.FindAllString(text, -1)
			out := make([]string, 0, len(matches))
			for _, match := range matches {
				digits := digitsOnly(match)
				if len(digits) < 10 || len(digits) > 15 {
					continue
				}
				out = append(out, digits)
			}
			return out
		},
		Normalize: digitsOnly,
	},
}

func defaultPIIPolicies(actor string, now time.Time) []piiPolicyRecord {
	entityTypes := supportedPIIEntityTypes()
	records := make([]piiPolicyRecord, 0, len(entityTypes))
	for _, entityType := range entityTypes {
		records = append(records, piiPolicyRecord{
			EntityType: entityType,
			Enabled:    true,
			Action:     piiActionObserve,
			UpdatedBy:  actor,
			UpdatedAt:  now,
		})
	}
	return records
}

func supportedPIIEntityTypes() []string {
	entityTypes := make([]string, 0, len(piiEntityDefinitions))
	for entityType := range piiEntityDefinitions {
		entityTypes = append(entityTypes, entityType)
	}
	sort.Strings(entityTypes)
	return entityTypes
}

func piiDisplayName(entityType string) string {
	definition, ok := piiEntityDefinitions[entityType]
	if !ok {
		return entityType
	}
	return definition.DisplayName
}

func validPIIEntityType(entityType string) bool {
	_, ok := piiEntityDefinitions[strings.TrimSpace(entityType)]
	return ok
}

func validPIIAction(action string) bool {
	switch strings.TrimSpace(action) {
	case piiActionObserve, piiActionBlock:
		return true
	default:
		return false
	}
}

func derivePIIFingerprintKey(cfg config.Config) string {
	base := strings.TrimSpace(cfg.SecretMasterKey)
	if base == "" {
		base = "api-guard-pii-fallback|" + strings.TrimSpace(cfg.ListenAddr) + "|" + strings.TrimSpace(cfg.OpenAIBaseURL)
	}
	sum := sha256.Sum256([]byte(base))
	return string(sum[:])
}

func fingerprintPIIValue(secret, entityType, normalizedValue string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(entityType))
	mac.Write([]byte{0})
	mac.Write([]byte(normalizedValue))
	return hex.EncodeToString(mac.Sum(nil))
}

func detectPIIFindings(texts []string, policyByEntityType map[string]piiPolicyRecord, direction, fingerprintKey string) []piiFindingRecord {
	if len(texts) == 0 || len(policyByEntityType) == 0 {
		return nil
	}

	aggregated := map[string]*piiFindingRecord{}
	for _, entityType := range supportedPIIEntityTypes() {
		policy, ok := policyByEntityType[entityType]
		if !ok || !policy.Enabled || !validPIIAction(policy.Action) {
			continue
		}
		definition := piiEntityDefinitions[entityType]
		for _, text := range texts {
			if strings.TrimSpace(text) == "" {
				continue
			}
			for _, rawMatch := range definition.Detect(text) {
				normalized := definition.Normalize(rawMatch)
				if strings.TrimSpace(normalized) == "" {
					continue
				}
				fingerprint := fingerprintPIIValue(fingerprintKey, entityType, normalized)
				key := direction + "|" + entityType + "|" + policy.Action + "|" + fingerprint
				record, exists := aggregated[key]
				if !exists {
					record = &piiFindingRecord{
						Direction:   direction,
						EntityType:  entityType,
						Action:      policy.Action,
						Fingerprint: fingerprint,
					}
					aggregated[key] = record
				}
				record.Count++
			}
		}
	}

	out := make([]piiFindingRecord, 0, len(aggregated))
	for _, record := range aggregated {
		out = append(out, *record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Direction == out[j].Direction {
			if out[i].EntityType == out[j].EntityType {
				return out[i].Fingerprint < out[j].Fingerprint
			}
			return out[i].EntityType < out[j].EntityType
		}
		return out[i].Direction < out[j].Direction
	})
	return out
}

func hasBlockingPIIFinding(findings []piiFindingRecord) bool {
	for _, finding := range findings {
		if finding.Action == piiActionBlock && finding.Direction == piiDirectionIngress {
			return true
		}
	}
	return false
}

func piiFindingLogFields(findings []piiFindingRecord) (int64, int64, []string) {
	var ingressCount int64
	var egressCount int64
	entityTypes := map[string]struct{}{}

	for _, finding := range findings {
		entityTypes[finding.EntityType] = struct{}{}
		switch finding.Direction {
		case piiDirectionIngress:
			ingressCount += finding.Count
		case piiDirectionEgress:
			egressCount += finding.Count
		}
	}

	return ingressCount, egressCount, mapKeysSorted(entityTypes)
}

func extractRequestPIITexts(body []byte) []string {
	return extractRequestTextValues(body)
}

func extractRequestTextValues(body []byte) []string {
	var payload struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	out := make([]string, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		out = appendTextValues(out, message.Content)
	}
	return out
}

func extractResponsePIITexts(body []byte) []string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	var out []string
	if errorPayload, ok := payload["error"].(map[string]any); ok {
		if message, ok := errorPayload["message"].(string); ok {
			out = append(out, message)
		}
	}
	if choices, ok := payload["choices"].([]any); ok {
		for _, choiceValue := range choices {
			choice, ok := choiceValue.(map[string]any)
			if !ok {
				continue
			}
			if message, ok := choice["message"].(map[string]any); ok {
				out = appendTextValues(out, message["content"])
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				out = appendTextValues(out, delta["content"])
			}
		}
	}
	if output, ok := payload["output"].([]any); ok {
		for _, item := range output {
			typed, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = appendTextValues(out, typed["content"])
		}
	}
	return out
}

func appendTextValues(target []string, value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			target = append(target, typed)
		}
	case []any:
		for _, item := range typed {
			target = appendTextValues(target, item)
		}
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			target = append(target, text)
		}
		if content, ok := typed["content"]; ok {
			target = appendTextValues(target, content)
		}
	}
	return target
}

func digitsOnly(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func passesLuhn(value string) bool {
	sum := 0
	double := false
	for i := len(value) - 1; i >= 0; i-- {
		digit := int(value[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum > 0 && sum%10 == 0
}
