package logging

import (
	"log/slog"
	"regexp"
	"strings"
)

// Redacted replaces a credential that would otherwise reach the log output.
const Redacted = "[redacted]"

// credentialPatterns match the shapes a credential takes inside driver error
// text: DSN userinfo, and key=value pairs whose key names a secret.
//
// Sanitizing is necessary because the alternative -- trusting three third-party
// drivers never to embed a credential in an error string -- is not a property
// this project can verify or keep verified.
var credentialPatterns = []*regexp.Regexp{
	// scheme://user:secret@host
	regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:/@\s]+):([^@\s]+)@`),
	// password=secret, passwd: secret, token="secret", secret=...
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|auth)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;)]+)`),
}

// SanitizeString removes credentials from arbitrary text.
func SanitizeString(s string) string {
	for _, re := range credentialPatterns {
		s = re.ReplaceAllString(s, replacementFor(re))
	}
	return s
}

func replacementFor(re *regexp.Regexp) string {
	if strings.Contains(re.String(), "://") {
		return "$1:" + Redacted + "@"
	}
	return "$1=" + Redacted
}

// SanitizeError renders an error with credentials removed.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return SanitizeString(err.Error())
}

// Error returns a log attribute carrying a sanitized error.
//
// Use this instead of slog.Any("error", err) wherever the error may have come
// from a driver, so that FR-030 holds by construction rather than by each call
// site remembering.
func Error(err error) slog.Attr {
	return slog.String("error", SanitizeError(err))
}
