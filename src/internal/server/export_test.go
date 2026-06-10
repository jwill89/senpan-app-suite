package server

// DeriveFormatForTest exposes the unexported deriveFormat helper to the
// external server_test package so the format-mapping logic can be unit-tested.
var DeriveFormatForTest = deriveFormat

// PostDueEventsForTest runs a single event-scheduler sweep (posting any due,
// unposted events) so the external test package can exercise the background
// poster deterministically without waiting on the ticker.
func (s *Server) PostDueEventsForTest() { s.postDueEvents() }
