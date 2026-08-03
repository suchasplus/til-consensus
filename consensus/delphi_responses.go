package consensus

import (
	"fmt"
	"math"
	"strings"
)

// ValidateDelphiResponses enforces the rating contract at the coordinator
// boundary as well as at provider decoding time. This keeps custom task
// delegates from bypassing the same checks applied by the runtime adapters.
func ValidateDelphiResponses(responses []DelphiResponseDraft, scaleMin, scaleMax int) error {
	if scaleMin >= scaleMax {
		return fmt.Errorf("rating scale min must be less than max")
	}
	if len(responses) == 0 {
		return fmt.Errorf("responses must contain at least one rating")
	}
	seen := make(map[string]struct{}, len(responses))
	for idx, response := range responses {
		statementID := strings.TrimSpace(response.StatementID)
		statement := strings.TrimSpace(response.Statement)
		if statementID == "" && statement == "" {
			return fmt.Errorf("responses[%d] requires statementId or statement", idx)
		}
		if response.Rating == nil {
			return fmt.Errorf("responses[%d].rating is required", idx)
		}
		rating := *response.Rating
		if math.IsNaN(rating) || math.IsInf(rating, 0) || rating < float64(scaleMin) || rating > float64(scaleMax) {
			return fmt.Errorf("responses[%d].rating must be within [%d,%d]", idx, scaleMin, scaleMax)
		}
		key := "id:" + statementID
		if statementID == "" {
			key = "statement:" + normalizedClaimKey(statement)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("responses[%d] duplicates a rating for %q", idx, firstNonEmpty(statementID, statement))
		}
		seen[key] = struct{}{}
	}
	return nil
}
