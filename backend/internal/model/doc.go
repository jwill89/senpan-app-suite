// Package model defines the domain types shared across the application, along
// with the typed request/response envelopes for the HTTP API. These types carry
// no logic and are used as data-transfer objects between the store, service, and
// server layers. All struct tags are for JSON serialization to/from the API and
// database. The response envelopes are the single source of truth for BOTH the
// JSON wire shapes AND the generated frontend types (tygo) AND the OpenAPI
// schemas, so handlers return them instead of ad-hoc map[string]any.
package model
