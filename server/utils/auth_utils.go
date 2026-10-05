package utils

import (
	"os"
	"strings"
)

// IsEmailAllowed reports whether the given email may sign in, based on the
// comma-separated AUTH_ALLOWED_EMAILS env variable. Entries are either full
// addresses ("me@example.com") or domains prefixed with "@" ("@example.com").
// If AUTH_ALLOWED_EMAILS is unset or empty, every email is allowed.
func IsEmailAllowed(email string) bool {
	allowList := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_EMAILS"))
	if allowList == "" {
		return true
	}

	email = NormalizeEmail(email)
	if email == "" {
		return false
	}

	for _, entry := range strings.Split(allowList, ",") {
		entry = NormalizeEmail(entry)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "@") {
			if strings.HasSuffix(email, entry) {
				return true
			}
		} else if email == entry {
			return true
		}
	}

	return false
}
