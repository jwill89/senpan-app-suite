package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func botReq(headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestVerifiedBot(t *testing.T) {
	// No Cloudflare signal → not a verified bot.
	if _, ok := verifiedBot(botReq(nil)); ok {
		t.Error("no header should not be a verified bot")
	}
	if _, ok := verifiedBot(botReq(map[string]string{"X-Verified-Bot": "false"})); ok {
		t.Error("x-verified-bot:false should not be a verified bot")
	}

	// Custom transform-rule header (works on any plan) → verified; named by UA,
	// which is trustworthy now that Cloudflare vouched for the source.
	if name, ok := verifiedBot(botReq(map[string]string{
		"X-Verified-Bot": "true", "User-Agent": "Googlebot/2.1",
	})); !ok || name != "Googlebot/2.1" {
		t.Errorf("x-verified-bot true: got (%q, %v); want (Googlebot/2.1, true)", name, ok)
	}

	// Native managed-transform headers (Bot Management) → category preferred.
	if name, ok := verifiedBot(botReq(map[string]string{
		"Cf-Verified-Bot": "true", "Cf-Verified-Bot-Category": "Search Engine Crawler", "User-Agent": "x",
	})); !ok || name != "Search Engine Crawler" {
		t.Errorf("cf category: got (%q, %v); want (Search Engine Crawler, true)", name, ok)
	}

	// Verified but no category and no UA → a generic label, still flagged.
	if name, ok := verifiedBot(botReq(map[string]string{"Cf-Verified-Bot": "1"})); !ok || name != "verified" {
		t.Errorf("verified no name: got (%q, %v); want (verified, true)", name, ok)
	}
}

func TestTruthyHeader(t *testing.T) {
	for _, v := range []string{"true", "1", "yes", "TRUE", " Yes "} {
		if !truthyHeader(botReq(map[string]string{"X-T": v}), "X-T") {
			t.Errorf("truthyHeader(%q) = false; want true", v)
		}
	}
	for _, v := range []string{"", "false", "0", "no", "maybe"} {
		if truthyHeader(botReq(map[string]string{"X-T": v}), "X-T") {
			t.Errorf("truthyHeader(%q) = true; want false", v)
		}
	}
}
