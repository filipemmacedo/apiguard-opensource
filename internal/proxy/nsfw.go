package proxy

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const nsfwActionBlock = "block"

var errNSFWBlockedTermNotFound = errors.New("nsfw blocked term not found")

type nsfwBlockedTermRecord struct {
	ID             int64
	Term           string
	NormalizedTerm string
	Enabled        bool
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedBy      string
	UpdatedAt      time.Time
}

type nsfwBlockedTermMatch struct {
	TermID int64
}

type nsfwBlockedTermValidationError struct {
	message string
}

func (e nsfwBlockedTermValidationError) Error() string {
	return e.message
}

func normalizeNSFWTerm(term string) string {
	return strings.Join(strings.Fields(strings.ToLower(term)), " ")
}

func collapseNSFWDisplayTerm(term string) string {
	return strings.Join(strings.Fields(term), " ")
}

func validateNSFWTerm(term string) (string, string, error) {
	displayValue := collapseNSFWDisplayTerm(term)
	normalizedValue := normalizeNSFWTerm(displayValue)
	if normalizedValue == "" {
		return "", "", nsfwBlockedTermValidationError{message: "term is required"}
	}
	return displayValue, normalizedValue, nil
}

func detectNSFWBlockedTermMatches(texts []string, terms []nsfwBlockedTermRecord) []nsfwBlockedTermMatch {
	if len(texts) == 0 || len(terms) == 0 {
		return nil
	}

	matches := map[int64]nsfwBlockedTermMatch{}
	for _, text := range texts {
		normalizedText := normalizeNSFWTerm(text)
		if normalizedText == "" {
			continue
		}
		for _, term := range terms {
			if !term.Enabled || term.ID <= 0 || term.NormalizedTerm == "" {
				continue
			}
			if nsfwTermMatches(normalizedText, term.NormalizedTerm) {
				matches[term.ID] = nsfwBlockedTermMatch{TermID: term.ID}
			}
		}
	}

	if len(matches) == 0 {
		return nil
	}

	out := make([]nsfwBlockedTermMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, match)
	}
	sortNSFWMatches(out)
	return out
}

func nsfwGuardrailOutcomes(matches []nsfwBlockedTermMatch) []guardrailOutcomeRecord {
	if len(matches) == 0 {
		return nil
	}

	outcomes := make([]guardrailOutcomeRecord, 0, len(matches))
	for _, match := range matches {
		outcomes = append(outcomes, guardrailOutcomeRecord{
			GuardrailType:   guardrailTypeNSFWKeyword,
			Action:          nsfwActionBlock,
			MatchedPolicyID: strconv.FormatInt(match.TermID, 10),
		})
	}
	sortGuardrailOutcomes(outcomes)
	return outcomes
}

func sortNSFWMatches(matches []nsfwBlockedTermMatch) {
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].TermID < matches[j].TermID
	})
}

func nsfwTermMatches(normalizedText, normalizedTerm string) bool {
	if normalizedTerm == "" || normalizedText == "" {
		return false
	}
	if strings.Contains(normalizedTerm, " ") {
		return strings.Contains(normalizedText, normalizedTerm)
	}
	return containsWholeNormalizedWord(normalizedText, normalizedTerm)
}

func containsWholeNormalizedWord(text, term string) bool {
	searchStart := 0
	for searchStart <= len(text)-len(term) {
		index := strings.Index(text[searchStart:], term)
		if index < 0 {
			return false
		}
		start := searchStart + index
		end := start + len(term)
		if wholeWordBoundary(text, start, end) {
			return true
		}
		searchStart = end
	}
	return false
}

func wholeWordBoundary(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if isNSFWWordRune(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if isNSFWWordRune(r) {
			return false
		}
	}
	return true
}

func isNSFWWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}
