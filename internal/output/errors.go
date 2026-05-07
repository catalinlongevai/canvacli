package output

import (
	"encoding/json"
	"io"
)

type ErrorEnvelope struct {
	Error       string   `json:"error"`
	Message     string   `json:"message,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Fix         string   `json:"fix,omitempty"`
	WaitSeconds int      `json:"wait_seconds,omitempty"`
	ExitCode    int      `json:"exit_code"`
}

// EmitError prints a structured error envelope and returns the appropriate
// exit code. Fix is selected by code from a static map (never from upstream
// data) — defends against prompt injection.
func EmitError(w io.Writer, code, message string, suggestions []string) int {
	env := ErrorEnvelope{
		Error:       code,
		Message:     message,
		Suggestions: suggestions,
		Fix:         fixForCode(code),
		ExitCode:    exitCodeFor(code),
	}
	b, _ := json.Marshal(env)
	_, _ = w.Write(append(b, '\n'))
	return env.ExitCode
}

func fixForCode(code string) string {
	switch code {
	case "auth_revoked", "auth_required":
		return "canva login"
	case "design_not_found":
		return "canva list --json | jq '.title'"
	case "template_not_found":
		return "canva templates --json"
	case "rate_limited":
		return "retry after wait_seconds (or pass --auto-wait)"
	case "permission_denied":
		return "verify your account has the required scopes via canva whoami"
	default:
		return ""
	}
}

func exitCodeFor(code string) int {
	switch code {
	case "auth_revoked", "auth_required":
		return 2
	case "design_not_found", "template_not_found", "not_found":
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
