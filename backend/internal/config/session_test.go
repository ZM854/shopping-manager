package config

import "testing"

func TestSessionURLConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, env, api, frontend string
		valid, secure            bool
	}{
		{"dev HTTP", "dev", "http://localhost/api", "http://localhost", true, false},
		{"production HTTPS", "production", "https://app.example/api", "https://app.example", true, true},
		{"unset environment", "", "http://localhost/api", "http://localhost", false, false},
		{"production HTTP", "production", "http://app.example/api", "http://app.example", false, false},
		{"mixed HTTPS", "dev", "https://app.example/api", "http://app.example", false, false},
		{"mixed HTTP", "dev", "http://app.example/api", "https://app.example", false, false},
		{"relative", "dev", "/api", "http://localhost", false, false},
		{"credentials", "dev", "https://private:secret@app.example/api", "https://app.example", false, false},
		{"fragment", "dev", "https://app.example/api", "https://app.example/#private", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secure, err := validateSessionURLs(tc.env, tc.api, tc.frontend)
			if (err == nil) != tc.valid || secure != tc.secure {
				t.Fatal("Invalid session URL policy")
			}
		})
	}
}
