package server

// DeriveFormatForTest exposes the unexported deriveFormat helper to the
// external server_test package so the format-mapping logic can be unit-tested.
var DeriveFormatForTest = deriveFormat

// The following expose pure, unexported helpers to the external server_test
// package so their logic can be unit-tested without spinning up the HTTP server.
var (
	// AdminMutationResourceForTest maps a POST path to its invalidation resource
	// key — the core of the live-admin invalidation feature, including the
	// deliberate exclusion of the public ".../enter" and ".../draw" paths.
	AdminMutationResourceForTest = adminMutationResource
	// SafeFontNameForTest validates/normalizes an uploaded font filename.
	SafeFontNameForTest = safeFontName
	// SanitizeGaraponPrizesForTest trims/normalizes incoming garapon prize rows.
	SanitizeGaraponPrizesForTest = sanitizeGaraponPrizes
	// IsDiscordSnowflakeForTest reports whether a string is a Discord snowflake ID.
	IsDiscordSnowflakeForTest = isDiscordSnowflake
	// ParseRaffleTimeForTest parses a raffle availability timestamp to a UTC instant.
	ParseRaffleTimeForTest = parseRaffleTime
	// NormalizeFontOriginForTest validates/normalizes a font-allowlist origin.
	NormalizeFontOriginForTest = normalizeFontOrigin
	// FontTokenBucketForTest returns the token time bucket for an instant.
	FontTokenBucketForTest = fontTokenBucket
	// FontFileTokenForTest derives a font's serving token for a given bucket, so
	// the expiry window (current + previous bucket) can be exercised directly.
	FontFileTokenForTest = (*Server).fontFileToken
)

// WebRootForTest exposes the server's webRoot so external tests can seed
// upload files mid-test (e.g. adding a font after the env is built).
func (s *Server) WebRootForTest() string { return s.webRoot }
