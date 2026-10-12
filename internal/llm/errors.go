package llm

import "fmt"

// httpStatusError formats a non-200 LLM HTTP response. Auth errors keep the
// literal "HTTP 401" / "HTTP 403" prefix so session.statusFromError can parse them.
func httpStatusError(statusCode int, body string) error {
	switch statusCode {
	case 401:
		return fmt.Errorf("HTTP 401: authentication failed — check your API key or provider credentials (%s)", body)
	case 403:
		return fmt.Errorf("HTTP 403: access denied — verify your API key permissions or model access (%s)", body)
	default:
		return fmt.Errorf("HTTP %d: %s", statusCode, body)
	}
}
