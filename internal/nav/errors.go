package nav

import (
	"errors"
	"fmt"
	"net/http"
)

// Error 是领域层返回给 HTTP 层的结构化错误。
type Error struct {
	Status    int
	Code      string
	Message   string
	Conflicts []Conflict
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func NotFound(what string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: what + " not found"}
}

func Invalid(message string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_board", Message: message}
}

func InvalidWithConflicts(message string, conflicts []Conflict) *Error {
	return &Error{
		Status:    http.StatusUnprocessableEntity,
		Code:      "grid_conflict",
		Message:   message,
		Conflicts: conflicts,
	}
}

func RevisionMismatch(expected, got int64) *Error {
	return &Error{
		Status:  http.StatusConflict,
		Code:    "revision_mismatch",
		Message: fmt.Sprintf("board revision is %d, client sent %d", expected, got),
	}
}

func BadRequest(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

// AsError 把任意 error 归一成 *Error（ValidationError 会转成 422）。
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var valErr *ValidationError
	if errors.As(err, &valErr) {
		if len(valErr.Conflicts) > 0 {
			return InvalidWithConflicts(valErr.Message, valErr.Conflicts)
		}
		return Invalid(valErr.Message)
	}
	return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
}
