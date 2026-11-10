// Package anilist looks titles up on AniList's GraphQL API and maps them to the
// fields of a reading-list item. The endpoint is the caller's: it is configurable,
// so it is held to an allowlist, as is every redirect the lookup follows.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/bookclubs"
)

// userAgent names this app to AniList and its Cloudflare front: a descriptive one is
// good practice for a public API and avoids generic bot filtering.
const userAgent = "SenpanAppSuite (+https://apps.senpan.cafe)"

// Timeout is how long a lookup may take.
const Timeout = 15 * time.Second

// maxBody caps how much of a response is read.
const maxBody = 2 << 20

// Kind classifies why a lookup failed, by what the caller can do about it.
type Kind int

const (
	// Unreachable is AniList answering with a 5xx or something unreadable, or the
	// request never getting an answer (DNS, a refused connection, a timeout).
	Unreachable Kind = iota + 1
	// RateLimited is AniList answering 429; Error.RetryAfter says when to try again.
	RateLimited
	// Refused is any other refusal: a 4xx, or an error in the GraphQL answer. A
	// datacenter address AniList has blocked or a disabled API land here.
	Refused
)

// Error is a failed lookup.
type Error struct {
	Kind Kind
	// Status is AniList's HTTP status, 0 when it never answered.
	Status int
	// RetryAfter is how long AniList asked to wait, for RateLimited.
	RetryAfter time.Duration
	// Message is AniList's own reason, or what went wrong reaching it.
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("anilist returned status %d: %s", e.Status, e.Message)
	}
	return "anilist: " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// HTTPStatus is the HTTP status an API answers when err failed a lookup, and how
// long to wait when that is a 429: a rate limit is 429, a refusal 500, and AniList
// not answering 503. It is never a 4xx or 5xx that Cloudflare would replace with its
// own page without saying which of these it was.
func HTTPStatus(err error) (status int, retryAfter time.Duration) {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return http.StatusInternalServerError, 0
	}
	switch e.Kind {
	case RateLimited:
		return http.StatusTooManyRequests, e.RetryAfter
	case Unreachable:
		return http.StatusServiceUnavailable, 0
	case Refused:
		return http.StatusInternalServerError, 0
	}
	return http.StatusInternalServerError, 0
}

// Client looks titles up. The zero value is not ready: use New.
type Client struct {
	HTTP *http.Client
}

// New is a Client whose redirects are checked against the AniList allowlist, so a
// crafted or compromised endpoint can't bounce the server's request to an internal
// service or a cloud metadata address.
func New() *Client {
	return &Client{HTTP: &http.Client{
		Timeout: Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if !bookclubs.IsAllowedAniListURL(req.URL.String()) {
				return fmt.Errorf("redirect to disallowed host %q blocked", req.URL.Host)
			}
			return nil
		},
	}}
}

// Endpoint is the GraphQL endpoint to use for the configured setting: it, trimmed of
// a trailing slash, when it is an allowed AniList address, else the default. The
// setting is checked when saved, but one written another way must not aim the lookup
// at an internal service.
func Endpoint(configured string) string {
	base := strings.TrimSpace(configured)
	if base == "" || !bookclubs.IsAllowedAniListURL(base) {
		base = bookclubs.DefaultAniListURL
	}
	return strings.TrimRight(base, "/")
}

// Media is the part of an AniList Media object a reading-list item is made from.
type Media struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Description string `json:"description"`
	CoverImage  struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
	Format          string   `json:"format"`
	CountryOfOrigin string   `json:"countryOfOrigin"`
	Genres          []string `json:"genres"`
	Chapters        *int     `json:"chapters"`
	SiteURL         string   `json:"siteUrl"`
}

// mediaFields is the GraphQL selection set for a Media node.
const mediaFields = `id title { romaji english native } description(asHtml: false) ` +
	`coverImage { extraLarge large } format countryOfOrigin genres chapters siteUrl`

// Source is a link to where a title can be read.
type Source struct {
	Title string
	URL   string
}

// Suggestion is a title as a reading-list item could start out.
type Suggestion struct {
	CoverImage string
	Title      string
	Summary    string
	Format     string
	Genres     string
	Chapters   string
	Sources    []Source
}

// Search runs a paged manga search at endpoint.
func (c *Client) Search(ctx context.Context, endpoint, search string) ([]Suggestion, error) {
	query := `query ($search: String) { Page(page: 1, perPage: 10) { media(search: $search, type: MANGA, sort: SEARCH_MATCH) { ` + mediaFields + ` } } }`
	body, err := c.post(ctx, endpoint, query, map[string]any{"search": search})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Page struct {
				Media []Media `json:"media"`
			} `json:"Page"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	if err := decode(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, &Error{Kind: Refused, Message: resp.Errors[0].Message}
	}
	out := make([]Suggestion, 0, len(resp.Data.Page.Media))
	for _, m := range resp.Data.Page.Media {
		out = append(out, ToSuggestion(m))
	}
	return out, nil
}

// ByID fetches one manga title by its AniList id; a title AniList doesn't have is
// an empty result, not an error.
func (c *Client) ByID(ctx context.Context, endpoint string, id int) ([]Suggestion, error) {
	query := `query ($id: Int) { Media(id: $id, type: MANGA) { ` + mediaFields + ` } }`
	body, err := c.post(ctx, endpoint, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Media *Media `json:"Media"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	if err := decode(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, &Error{Kind: Refused, Message: resp.Errors[0].Message}
	}
	if resp.Data.Media == nil {
		return []Suggestion{}, nil
	}
	return []Suggestion{ToSuggestion(*resp.Data.Media)}, nil
}

type graphQLError struct {
	Message string `json:"message"`
}

func decode(body []byte, into any) error {
	if err := json.Unmarshal(body, into); err != nil {
		return &Error{Kind: Unreachable, Message: "its answer couldn't be read", Err: err}
	}
	return nil
}

// post runs a GraphQL query and returns the response body, capped. ctx is the
// caller's request, so a lookup they abandon stops too.
func (c *Client) post(ctx context.Context, endpoint, query string, variables map[string]any) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, &Error{Kind: Refused, Message: "the query couldn't be encoded", Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Kind: Refused, Message: "the request couldn't be built", Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	slog.Debug("anilist request", "endpoint", endpoint, "bytes", len(payload))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, transportError(err, c.HTTP.Timeout)
	}
	// Read below: closing it has nothing to report.
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	slog.Debug("anilist response", "status", resp.StatusCode, "bytes", len(body))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	// AniList sends its GraphQL error shape on a failure too (a 403 when the API is
	// disabled for stability): its message is the real reason.
	msg := errorMessage(body)
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	e := &Error{Kind: Refused, Status: resp.StatusCode, Message: msg}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		e.Kind = RateLimited
		e.RetryAfter = retryAfter(resp.Header.Get("Retry-After"))
	case resp.StatusCode >= 500:
		e.Kind = Unreachable
	}
	return nil, e
}

// transportError says why AniList couldn't be reached: a timeout, a refused or reset
// connection, DNS or TLS. A datacenter address meeting AniList's Cloudflare front
// commonly sees a timeout or a reset rather than a clean status.
func transportError(err error, timeout time.Duration) *Error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		if urlErr.Timeout() {
			return &Error{Kind: Unreachable, Message: fmt.Sprintf("it did not respond within %s", timeout), Err: err}
		}
		return &Error{Kind: Unreachable, Message: fmt.Sprintf("it could not be reached (%v)", urlErr.Err), Err: err}
	}
	return &Error{Kind: Unreachable, Message: fmt.Sprintf("the request failed (%v)", err), Err: err}
}

// retryAfter reads a Retry-After header in seconds; 0 when it is absent or isn't.
func retryAfter(header string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

// errorMessage is the first GraphQL error message in a response body, "" when the
// body isn't that shape.
func errorMessage(body []byte) string {
	var resp struct {
		Errors []graphQLError `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Errors) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Errors[0].Message)
}

// ToSuggestion maps a Media to a reading-list item: the best available title, the
// description without its HTML, a friendly format derived from format and country of
// origin, and AniList's own page as the first source.
func ToSuggestion(m Media) Suggestion {
	cover := m.CoverImage.ExtraLarge
	if cover == "" {
		cover = m.CoverImage.Large
	}
	title := m.Title.English
	if title == "" {
		title = m.Title.Romaji
	}
	if title == "" {
		title = m.Title.Native
	}
	chapters := ""
	if m.Chapters != nil && *m.Chapters > 0 {
		chapters = strconv.Itoa(*m.Chapters)
	}
	sources := []Source{}
	if m.SiteURL != "" {
		sources = append(sources, Source{Title: "AniList", URL: m.SiteURL})
	}
	return Suggestion{
		CoverImage: cover,
		Title:      strings.TrimSpace(title),
		Summary:    StripHTML(m.Description),
		Format:     DeriveFormat(m.Format, m.CountryOfOrigin),
		Genres:     strings.Join(m.Genres, ", "),
		Chapters:   chapters,
		Sources:    sources,
	}
}

// DeriveFormat turns AniList's format and country of origin into a friendlier label
// (Manga, Manhwa, Manhua, Danmei, Novel); the curator can change it.
func DeriveFormat(format, country string) string {
	switch format {
	case "NOVEL":
		if country == "CN" || country == "TW" {
			return "Danmei"
		}
		return "Novel"
	case "ONE_SHOT":
		return "One-shot"
	}
	switch country {
	case "KR":
		return "Manhwa"
	case "CN", "TW":
		return "Manhua"
	case "JP":
		return "Manga"
	}
	if format == "" {
		return ""
	}
	// The raw enum, title-cased ("MANGA" -> "Manga").
	return strings.ToUpper(format[:1]) + strings.ToLower(format[1:])
}

var (
	brTag    = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	blankRun = regexp.MustCompile(`\n{3,}`)
)

// StripHTML removes the tags AniList descriptions use (<br>, <i>, <b>), unescapes
// entities, and collapses runs of blank lines, so the summary reads as plain text.
// A <br> becomes a line break so paragraphs survive.
func StripHTML(s string) string {
	s = brTag.ReplaceAllString(s, "\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = blankRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
