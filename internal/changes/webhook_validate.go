package changes

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const maxWebhookRedirects = 0 // no redirects on delivery client

// ValidateWebhookTarget checks destination URL safety. demo profile may allow http.
func ValidateWebhookTarget(targetURL string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("target_url required")
	}
	u, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid target_url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("target_url must include scheme and host")
	}
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_PROFILE")))
	if profile == "" {
		profile = strings.ToLower(strings.TrimSpace(os.Getenv("PROFILE")))
	}
	allowHTTP := profile == "demo" || profile == "memory"
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return fmt.Errorf("webhook target must use https outside demo")
		}
	default:
		return fmt.Errorf("webhook scheme %q not allowed", scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook host required")
	}
	if isBlockedHost(host) {
		return fmt.Errorf("webhook target host %q is not allowed", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("webhook target DNS lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("webhook target resolves to blocked address %s", ip.String())
		}
	}
	return nil
}

func isBlockedHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if h == "metadata.google.internal" || strings.HasSuffix(h, ".internal") {
		return true
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	// AWS/GCP/Azure metadata
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
	}
	return false
}
