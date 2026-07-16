package launcher

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"cortex.local/cortex/internal/config"
)

func validateDashboardURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawPath != "" ||
		parsed.Fragment != "" || config.ValidateListen(parsed.Host) != nil || !validDashboardPath(parsed) {
		return fmt.Errorf("dashboard URL must be a local Cortex HTTP address")
	}
	return nil
}

func validDashboardPath(parsed *url.URL) bool {
	if parsed.Path == "/" {
		return parsed.RawQuery == ""
	}
	if parsed.Path != "/ui/session" {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 1 || len(query["code"]) != 1 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Get("code"))
	return err == nil && len(raw) == 32
}
