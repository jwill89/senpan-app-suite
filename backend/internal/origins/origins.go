// Package origins is the rule for the sites allowed to read a public API from a
// browser: which entries an admin may list, and whether an Origin matches the list.
package origins

import (
	"net/url"
	"strings"
)

// problem is why an entry is refused, a sentence an admin reads.
type problem string

func (p problem) Error() string { return string(p) }

const (
	errScheme   problem = "An origin starts with http:// or https://."
	errParts    problem = "An origin is a scheme and a host (and a port), with no path, query, fragment or user info."
	errWildcard problem = "A wildcard may only be the whole first label of the host, as in https://*.example.com."
	errHost     problem = "That isn't a valid host."
)

// Normalize returns entry as a canonical allowlist entry: an origin
// (scheme://host[:port], scheme http or https) in lower case, with no path, query,
// fragment or user info and no trailing slash. The host may start with "*." as its
// whole first label (https://*.example.com), standing for every subdomain; a bare
// "*" or any other wildcard is refused.
func Normalize(entry string) (string, error) {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(entry), "://")
	scheme = strings.ToLower(scheme)
	if !ok || (scheme != "http" && scheme != "https") {
		return "", errScheme
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.ContainsAny(rest, "/?#@\\ ") {
		return "", errParts
	}
	host, wild := strings.CutPrefix(rest, "*.")
	if strings.Contains(host, "*") {
		return "", errWildcard
	}
	u, err := url.Parse(scheme + "://" + host)
	if err != nil || u.Hostname() == "" || !strings.EqualFold(u.Host, host) {
		return "", errHost
	}
	canonical := strings.ToLower(u.Host)
	if wild {
		canonical = "*." + canonical
	}
	return scheme + "://" + canonical, nil
}

// Allowed reports whether origin, as a browser sends it in the Origin header, is
// covered by list: it equals an entry, or an entry like "https://*.example.com"
// covers it as a subdomain at any depth (not example.com itself) with the same
// scheme and port. Entries are compared in their normalized form, so a bare "*"
// covers nothing.
func Allowed(list []string, origin string) bool {
	origin = strings.ToLower(strings.TrimSpace(origin))
	scheme, host, ok := strings.Cut(origin, "://")
	if !ok {
		return false
	}
	for _, entry := range list {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == origin {
			return true
		}
		entryScheme, entryHost, ok := strings.Cut(entry, "://")
		if !ok || entryScheme != scheme {
			continue
		}
		if suffix, wild := strings.CutPrefix(entryHost, "*"); wild && strings.HasPrefix(suffix, ".") &&
			strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
			return true
		}
	}
	return false
}
