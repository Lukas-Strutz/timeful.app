package utils

import "testing"

func TestIsEmailAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList string
		email     string
		want      bool
	}{
		{"empty allow list allows everyone", "", "anyone@example.com", true},
		{"exact match", "me@example.com", "me@example.com", true},
		{"match is case and whitespace insensitive", " Me@Example.com ", "  ME@example.COM", true},
		{"other address is rejected", "me@example.com", "you@example.com", false},
		{"one of several entries", "a@x.com, me@example.com ,b@y.com", "me@example.com", true},
		{"domain entry", "@example.com", "anyone@example.com", true},
		{"domain entry does not match other domains", "@example.com", "me@notexample.org", false},
		{"domain entry does not match look-alike suffix", "@example.com", "me@badexample.com", false},
		{"empty email is rejected", "me@example.com", "", false},
		{"blank entries are ignored", " , ,me@example.com", "you@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_ALLOWED_EMAILS", tt.allowList)
			if got := IsEmailAllowed(tt.email); got != tt.want {
				t.Errorf("IsEmailAllowed(%q) with %q = %v, want %v", tt.email, tt.allowList, got, tt.want)
			}
		})
	}
}
