package domain

import (
	"fmt"
	"strings"
)

const DefaultScreenshotSort = "-uploaded_at"

var allowedScreenshotSortFields = map[string]struct{}{
	"view_name":   {},
	"uploaded_at": {},
}

// NormalizeScreenshotSort validates a sort expression and returns it
// in the canonical "+field"/"-field" form.
func NormalizeScreenshotSort(sort string) (string, error) {
	if sort == "" {
		return DefaultScreenshotSort, nil
	}

	direction := "+"
	field := sort
	switch sort[0] {
	case '+', ' ':
		field = sort[1:]
	case '-':
		direction = "-"
		field = sort[1:]
	}

	field = strings.ToLower(strings.TrimSpace(field))
	if _, ok := allowedScreenshotSortFields[field]; !ok {
		return "", fmt.Errorf("invalid sort field: %q", field)
	}

	return direction + field, nil
}
