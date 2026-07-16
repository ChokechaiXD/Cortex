package cortex

func applyFeedback(truth, utility float64, outcome FeedbackOutcome) (float64, float64, EventType) {
	switch outcome {
	case FeedbackConfirmed:
		return clamp(truth + 0.10), utility, EventConfirmed
	case FeedbackContradicted:
		return clamp(truth - 0.20), utility, EventContradicted
	case FeedbackHelpful:
		return truth, clamp(utility + 0.10), EventHelpful
	case FeedbackUnhelpful:
		return truth, clamp(utility - 0.20), EventUnhelpful
	case FeedbackApplied:
		return truth, clamp(utility + 0.03), EventApplied
	default:
		return truth, utility, ""
	}
}

func clamp(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
