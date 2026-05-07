package api

type APIError struct {
	Code        string `json:"error"`
	Message     string `json:"message"`
	HTTPStatus  int    `json:"-"`
	WaitSeconds int    `json:"wait_seconds,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func (e *APIError) ExitCode() int {
	switch e.Code {
	case "auth_revoked", "auth_required":
		return 2
	case "not_found", "design_not_found", "template_not_found":
		return 3
	case "network", "api_unavailable":
		return 4
	case "validation", "bad_request":
		return 5
	case "rate_limited":
		return 6
	case "permission_denied", "scope_insufficient":
		return 7
	default:
		return 1
	}
}

func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

var (
	ErrAuthRevoked      = &APIError{Code: "auth_revoked"}
	ErrNotFound         = &APIError{Code: "not_found"}
	ErrRateLimited      = &APIError{Code: "rate_limited"}
	ErrValidation       = &APIError{Code: "validation"}
	ErrPermissionDenied = &APIError{Code: "permission_denied"}
)
