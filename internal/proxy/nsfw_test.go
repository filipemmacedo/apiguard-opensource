package proxy

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"apiguard/internal/config"
)

func TestNSFWTermValidationAndMatching(t *testing.T) {
	t.Parallel()

	displayValue, normalizedValue, err := validateNSFWTerm("  Adult   Content  ")
	if err != nil {
		t.Fatalf("validateNSFWTerm: %v", err)
	}
	if displayValue != "Adult Content" {
		t.Fatalf("expected collapsed display term, got %q", displayValue)
	}
	if normalizedValue != "adult content" {
		t.Fatalf("expected normalized term, got %q", normalizedValue)
	}

	matches := detectNSFWBlockedTermMatches([]string{
		"This ADULT   content should be blocked.",
		"cartoon partials should stay allowed",
	}, []nsfwBlockedTermRecord{
		{ID: 1, Term: "Adult Content", NormalizedTerm: "adult content", Enabled: true},
		{ID: 2, Term: "art", NormalizedTerm: "art", Enabled: true},
	})
	if len(matches) != 1 {
		t.Fatalf("expected 1 NSFW match, got %d", len(matches))
	}
	if matches[0].TermID != 1 {
		t.Fatalf("expected match for term 1, got %d", matches[0].TermID)
	}

	outcomes := nsfwGuardrailOutcomes(matches)
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 guardrail outcome, got %d", len(outcomes))
	}
	if outcomes[0].GuardrailType != guardrailTypeNSFWKeyword {
		t.Fatalf("unexpected guardrail type: %q", outcomes[0].GuardrailType)
	}
	if outcomes[0].MatchedPolicyID != "1" {
		t.Fatalf("unexpected matched policy id: %q", outcomes[0].MatchedPolicyID)
	}
}

func TestManagedNSFWTermsRejectDuplicateNormalizedTerm(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	if _, err := server.createManagedNSFWBlockedTerm("Adult Content", true, "tester"); err != nil {
		t.Fatalf("createManagedNSFWBlockedTerm: %v", err)
	}

	_, err := server.createManagedNSFWBlockedTerm(" adult   content ", true, "tester")
	if err == nil {
		t.Fatal("expected duplicate normalized term error")
	}
	var validationErr nsfwBlockedTermValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T", err)
	}
	if validationErr.Error() != "duplicate normalized nsfw term" {
		t.Fatalf("unexpected validation error: %q", validationErr.Error())
	}
}

func TestManagedNSFWTermPersistenceLifecycle(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, config.Config{
		UpstreamTimeout: time.Second,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	created, err := server.createManagedNSFWBlockedTerm("Explicit Phrase", true, "tester")
	if err != nil {
		t.Fatalf("createManagedNSFWBlockedTerm: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("expected persisted id, got %d", created.ID)
	}

	updated, err := server.updateManagedNSFWBlockedTerm(created.ID, "Explicit   Phrase", false, "tester")
	if err != nil {
		t.Fatalf("updateManagedNSFWBlockedTerm: %v", err)
	}
	if updated.Term != "Explicit Phrase" {
		t.Fatalf("expected collapsed updated term, got %q", updated.Term)
	}
	if updated.Enabled {
		t.Fatal("expected updated term to be disabled")
	}

	records, err := server.management.listNSFWBlockedTerms()
	if err != nil {
		t.Fatalf("listNSFWBlockedTerms: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 persisted term, got %d", len(records))
	}
	if records[0].NormalizedTerm != "explicit phrase" {
		t.Fatalf("unexpected normalized term: %q", records[0].NormalizedTerm)
	}
	if records[0].Enabled {
		t.Fatal("expected disabled persisted term")
	}

	if err := server.deleteManagedNSFWBlockedTerm(created.ID); err != nil {
		t.Fatalf("deleteManagedNSFWBlockedTerm: %v", err)
	}
	records, err = server.management.listNSFWBlockedTerms()
	if err != nil {
		t.Fatalf("listNSFWBlockedTerms after delete: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 persisted terms after delete, got %d", len(records))
	}
}
