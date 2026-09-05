package store

import (
	"errors"
	"strings"

	"github.com/agensfield/fulla/internal/fault"
)

// Preserve established archive/bundle failure codes, but keep actionable plugin
// and interaction outcomes instead of misreporting them as bad recovery keys.
func streamFailure(err error, code, message string) error {
	var problem *fault.Error
	if errors.As(err, &problem) && (strings.HasPrefix(problem.Code, "plugin.") || problem.Code == "interaction.required") {
		return problem
	}
	return fault.New(code, message)
}
