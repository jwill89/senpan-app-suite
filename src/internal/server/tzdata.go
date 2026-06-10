package server

// Embed the IANA timezone database into the binary so time.LoadLocation works
// regardless of whether the host OS ships zoneinfo (notably Windows). Book club
// event scheduling relies on resolving admin-supplied IANA timezones to compute
// absolute post/start instants and Discord timestamps.
import _ "time/tzdata"
