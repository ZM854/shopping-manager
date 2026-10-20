package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func httpSettings(cfg *Config) error {
	frontend, _ := url.Parse(cfg.FrontendURL)
	cfg.AllowedOrigins = []string{frontend.Scheme + "://" + frontend.Host}
	for _, value := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			strings.Trim(u.Path, "/") != "" || (u.Scheme != "http" && u.Scheme != "https") ||
			(u.Scheme == "http" && cfg.AppENV != "dev") {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS must contain explicit HTTP(S) origins; HTTPS is required outside dev")
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, u.Scheme+"://"+u.Host)
	}
	for _, value := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, addrErr := netip.ParseAddr(value)
		_, prefixErr := netip.ParsePrefix(value)
		if addrErr != nil && prefixErr != nil {
			return fmt.Errorf("TRUSTED_PROXIES must contain IP addresses or CIDR networks")
		}
		if prefix, err := netip.ParsePrefix(value); err == nil && prefix.Bits() == 0 {
			return fmt.Errorf("TRUSTED_PROXIES must not trust all addresses")
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, value)
	}
	for _, entry := range []struct {
		name     string
		target   *int
		fallback int
	}{
		{"AUTH_LOGIN_LIMIT", &cfg.AuthLoginLimit, 30},
		{"AUTH_REGISTRATION_LIMIT", &cfg.AuthRegistrationLimit, 10},
		{"AUTH_RESEND_LIMIT", &cfg.AuthResendLimit, 10},
	} {
		*entry.target = entry.fallback
		if value := os.Getenv(entry.name); value != "" {
			limit, err := strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 10000 {
				return fmt.Errorf("%s must be between 1 and 10000", entry.name)
			}
			*entry.target = limit
		}
	}
	return nil
}
