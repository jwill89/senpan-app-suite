package server

// DeriveFormatForTest exposes the unexported deriveFormat helper to the
// external server_test package so the format-mapping logic can be unit-tested.
var DeriveFormatForTest = deriveFormat
