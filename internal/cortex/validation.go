package cortex

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrInvalidInput = errors.New("invalid input")
)

func validateRemember(cmd RememberCommand) error {
	if strings.TrimSpace(cmd.IdempotencyKey) == "" || strings.TrimSpace(cmd.AgentID) == "" {
		return fmt.Errorf("%w: idempotency_key and agent_id are required", ErrInvalidInput)
	}
	if strings.TrimSpace(cmd.MemoryKey) == "" || strings.TrimSpace(cmd.Title) == "" || strings.TrimSpace(cmd.Content) == "" {
		return fmt.Errorf("%w: memory_key, title, and content are required", ErrInvalidInput)
	}
	if !validKind(cmd.Kind) || !validScope(cmd.Scope) {
		return fmt.Errorf("%w: unsupported kind or scope", ErrInvalidInput)
	}
	if (cmd.Scope == ScopeProject || cmd.Scope == ScopeDomain) && strings.TrimSpace(cmd.ScopeKey) == "" {
		return fmt.Errorf("%w: scope_key is required for %s scope", ErrInvalidInput, cmd.Scope)
	}
	return nil
}

func validateRecall(query RecallQuery) error {
	if strings.TrimSpace(query.AgentID) == "" || strings.TrimSpace(query.Text) == "" {
		return fmt.Errorf("%w: agent_id and text are required", ErrInvalidInput)
	}
	if query.Limit < 0 || query.Limit > 50 {
		return fmt.Errorf("%w: limit must be between 1 and 50", ErrInvalidInput)
	}
	return nil
}

func validateReview(cmd ReviewCommand) error {
	if strings.TrimSpace(cmd.IdempotencyKey) == "" || strings.TrimSpace(cmd.MemoryID) == "" || strings.TrimSpace(cmd.ActorID) == "" {
		return fmt.Errorf("%w: idempotency_key, memory_id, and actor_id are required", ErrInvalidInput)
	}
	switch cmd.Decision {
	case ReviewApprove, ReviewPromote, ReviewReject, ReviewSupersede, ReviewArchive:
		return nil
	default:
		return fmt.Errorf("%w: unsupported review decision", ErrInvalidInput)
	}
}

func validateFeedback(cmd FeedbackCommand) error {
	if strings.TrimSpace(cmd.IdempotencyKey) == "" || strings.TrimSpace(cmd.MemoryID) == "" || strings.TrimSpace(cmd.AgentID) == "" {
		return fmt.Errorf("%w: idempotency_key, memory_id, and agent_id are required", ErrInvalidInput)
	}
	switch cmd.Outcome {
	case FeedbackConfirmed, FeedbackContradicted, FeedbackHelpful, FeedbackUnhelpful, FeedbackApplied:
		return nil
	default:
		return fmt.Errorf("%w: unsupported feedback outcome", ErrInvalidInput)
	}
}

func validKind(kind MemoryKind) bool {
	switch kind {
	case KindFact, KindPreference, KindDecision, KindFailedAttempt, KindSolution, KindProjectState:
		return true
	default:
		return false
	}
}

func validScope(scope Scope) bool {
	switch scope {
	case ScopeGlobal, ScopeProject, ScopeDomain, ScopePrivate:
		return true
	default:
		return false
	}
}
