package action

import (
	"fmt"
	"strconv"
	"strings"
)

// parseInt64 parses a single trimmed positive int64 from a raw argument
// string. Empty input returns an error so palette callers can fall back to a
// cursor-context task ID if they choose.
func parseInt64(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("expected a task id")
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid task id %q", raw)
	}
	return n, nil
}

// parseFields splits raw on whitespace and trims each token. Empty tokens are
// dropped. Used by palette callers that accept "id arg1 arg2 ...".
func parseFields(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	return parts
}
