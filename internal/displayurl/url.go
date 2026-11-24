// Package displayurl validates external links for display in reports.
// It does not resolve or fetch URLs.
package displayurl

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var (
	httpURL                   = regexp.MustCompile(`^((?i:https?))://([^/?#]+)([/?#].*)?$`)
	authority                 = regexp.MustCompile(`^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9.-]+)(?::([0-9]+))?$`)
	dnsLabel                  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	urlChars                  = regexp.MustCompile(`^([A-Za-z0-9:/?#\[\]@!$&'()*+,;=._~-]|%[0-9A-Fa-f]{2})+$`)
	openProjectAPIWorkPackage = regexp.MustCompile(`^https?://[^/?#]+(?:/[^?#]*)?/api/v3/work_packages(?:[/?#]|$)`)
	numericHostLabel          = regexp.MustCompile(`(?i)^(?:[0-9]+|0x[0-9a-f]*)$`)
)

// Normalize trims surrounding ASCII spaces and lowercases the scheme. A
// rejected URL returns an empty string. Keep this rule aligned with DisplayUrl
// in the OpenProject Ruby consumer.
func Normalize(raw string) string {
	value := strings.Trim(raw, " ")
	parts := httpURL.FindStringSubmatch(value)
	if parts == nil || !urlChars.MatchString(value) {
		return ""
	}
	hostPort := authority.FindStringSubmatch(parts[2])
	if hostPort == nil || !validHost(hostPort[1]) {
		return ""
	}
	if port := hostPort[2]; port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return ""
		}
	}
	return strings.ToLower(parts[1]) + "://" + parts[2] + parts[3]
}

// NormalizeWorkPackage also excludes legacy OpenProject API self links from
// work-package UI links. They can still be retained as source evidence.
func NormalizeWorkPackage(raw string) string {
	value := Normalize(raw)
	if openProjectAPIWorkPackage.MatchString(value) {
		return ""
	}
	return value
}

func validHost(host string) bool {
	if strings.HasPrefix(host, "[") {
		ip, err := netip.ParseAddr(host[1 : len(host)-1])
		return err == nil && ip.Is6()
	}
	host = strings.TrimSuffix(host, ".")
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	// Browsers interpret numeric-looking hosts as IPv4. Require the canonical
	// four-part spelling when the last label would trigger that parser.
	if numericHostLabel.MatchString(labels[len(labels)-1]) {
		ip, err := netip.ParseAddr(host)
		return err == nil && ip.Is4()
	}
	for _, label := range labels {
		if !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}
