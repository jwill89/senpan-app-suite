package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// Public self-service sign-up for a Stamp Rally, plus the "I lost my link" lookup.
//
// A rally carrying the public_signup opt-in is listed here and anyone may issue
// themselves a card, rather than waiting for staff to hand one out. Signing up for a
// rally that has an open Garapon linked to it also issues that drawing link, on its
// OWN token: the card link can be shared, the drawing link spends draws, and they
// must not be the same secret.
//
// A participant is recorded as a character name plus a home world, in two fields,
// the way every other system in the app records one (model.ParticipantLabel).
//
// These paths are singular ("stamp-signup", "stamp-lookup") like the existing public
// "stamp-card/{token}". They deliberately do NOT sit under the plural "stamp-rallies"
// prefix, which adminMutationResource treats as an admin mutation; they broadcast
// their own invalidation instead.

// maxParticipantNameLen bounds a submitted name or world, counted in runes so a
// multi-byte name isn't cut short by a byte count.
const maxParticipantNameLen = 60

// handleStampSignupList returns the rallies currently open to public sign-up.
//
//	Endpoint:  GET /api/stamp-signup
//	Auth:      public
//	Response:  200 {"rallies": [SignupRally]}
func (s *Server) handleStampSignupList(w http.ResponseWriter, r *http.Request) {
	rallies, err := s.store.ListSignupRallies()
	if err != nil {
		writeInternalError(w, "list signup rallies", err)
		return
	}
	// The store filters on status + the opt-in; the availability window is
	// time-relative, so it is applied here as on the other public paths.
	now := time.Now().UTC()
	open := make([]model.SignupRally, 0, len(rallies))
	for _, ry := range rallies {
		if withinWindow(ry.AvailableFrom, ry.AvailableTo, now) {
			open = append(open, ry)
		}
	}
	writeJSON(w, http.StatusOK, model.SignupRalliesResponse{Rallies: open})
}

// stampSignupRequest is the JSON body for POST /api/stamp-signup/{id}.
type stampSignupRequest struct {
	Name string `json:"name"`
	// Home world, picked from a list rather than typed. Optional on the wire so an
	// older client still works, but the form always sends it.
	World          string `json:"world"`
	TurnstileToken string `json:"turnstile_token"` // Cloudflare Turnstile token (when enabled)
}

// handleStampSignup issues the caller their own card for a rally.
//
//	Endpoint:  POST /api/stamp-signup/{id}
//	Auth:      public
//	Request:   {"name": "Firstname Lastname", "world": "Gilgamesh"}
//	Response:  201 StampSignupResponse - the card token, plus the garapon token when
//	           the rally has an open linked Garapon (a separate token)
func (s *Server) handleStampSignup(w http.ResponseWriter, r *http.Request) {
	// Public endpoint: throttle per IP so a bot can't fill a rally's card table with
	// junk names. Every attempt counts against it, as on the raffle sign-up path.
	ip := clientIP(r)
	if s.rallyLimiter.isLimited(ip) {
		slog.Warn("stamp rally sign-up rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many sign-ups. Please try again later.")
		return
	}
	s.rallyLimiter.recordFailure(ip)

	rallyID, ok := pathInt64(w, r, "id", "stamp rally")
	if !ok {
		return
	}
	req, err := readJSON[stampSignupRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), req.TurnstileToken, ip) {
		slog.Warn("turnstile verification failed (stamp rally sign-up)", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot check failed. Please try again.")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Enter your character name")
		return
	}
	if utf8.RuneCountInString(name) > maxParticipantNameLen {
		writeError(w, http.StatusBadRequest, "That name is too long")
		return
	}
	// The world arrives as its own field now, the way every other system stores it.
	// A client that still sends "Name @ World" in one string is split here rather
	// than being stored as a name nothing else can match.
	world := strings.TrimSpace(req.World)
	if world == "" {
		name, world = model.SplitParticipantLabel(name)
	}
	if utf8.RuneCountInString(world) > maxParticipantNameLen {
		writeError(w, http.StatusBadRequest, "That world name is too long")
		return
	}

	rally, err := s.store.GetStampRally(rallyID)
	if err != nil {
		writeInternalError(w, "get rally for signup", err)
		return
	}
	// A rally that exists but hasn't opted in is reported as missing rather than
	// forbidden: this endpoint must not confirm the existence of rallies whose cards
	// staff hand out privately.
	if rally == nil || !rally.PublicSignup {
		writeError(w, http.StatusNotFound, "That stamp rally isn't open for sign-ups")
		return
	}
	if !rallyOpen(rally, time.Now().UTC()) {
		writeError(w, http.StatusBadRequest, "This stamp rally is no longer accepting sign-ups")
		return
	}

	// A linked, still-open garapon means this sign-up issues a drawing link too.
	// Best-effort, as on the admin path: if the lookup itself fails, the participant
	// still gets their stamp card rather than an error.
	// The draw allowance comes from the garapon itself: nobody is present to choose
	// one, so the admin sets it once per event (Default draws) and every self-issued
	// link carries it.
	var garaponID *int64
	var garaponTitle string
	garaponDraws := 1
	if g, gErr := s.store.OpenGaraponForRally(rallyID); gErr != nil {
		slog.Error("stamp signup: look up linked garapon", "rally_id", rallyID, "error", gErr)
	} else if g != nil {
		garaponID, garaponTitle = &g.ID, g.Title
		garaponDraws = clampDefaultDraws(g.DefaultDraws)
	}

	card, garaponToken, err := s.store.SignUpForRally(rallyID, name, world, garaponID, garaponDraws)
	if errors.Is(err, store.ErrParticipantNameTaken) {
		writeError(w, http.StatusConflict,
			`Someone has already signed up for this rally under that name. `+
				`If that was you, use "Find my links" to get your link back.`)
		return
	}
	if err != nil {
		writeInternalError(w, "sign up for rally", err)
		return
	}

	resp := model.StampSignupResponse{
		ParticipantName: card.ParticipantName,
		World:           card.World,
		RallyTitle:      rally.Title,
		CardToken:       card.Token,
	}
	if garaponID != nil {
		// Its own token, not the card's - see Store.SignUpForRally. This is the one
		// moment the drawing link is handed out, so the client stores it locally;
		// no name-keyed lookup will return it again.
		resp.GaraponToken = garaponToken
		resp.GaraponTitle = garaponTitle
	}
	writeJSON(w, http.StatusCreated, resp)

	slog.Info("stamp rally sign-up", "rally_id", rallyID, "rally", rally.Title,
		"participant", card.ParticipantName, "garapon", garaponTitle, "garapon_draws", garaponDraws)

	// Public path, so it is excluded from the adminMutationResource middleware.
	// Broadcast explicitly so an admin watching the rally (or the garapon) sees the
	// new card and drawing link appear live.
	s.broadcastResourceChanged("stamp-rallies")
	if garaponID != nil {
		s.broadcastResourceChanged("garapons")
	}
}

// stampLookupRequest is the JSON body for POST /api/stamp-lookup.
type stampLookupRequest struct {
	Name string `json:"name"`
	// Picked from the same world list the sign-up form uses. Optional: a client
	// that still sends one composed "Name @ World" string is split server-side.
	World string `json:"world"`
}

// handleStampLookup returns the cards - and paired drawing links - held under an
// exact participant name, for someone who lost their link.
//
//	Endpoint:  POST /api/stamp-lookup
//	Auth:      public
//	Request:   {"name": "Firstname Lastname @ World"}
//	Response:  200 {"entries": [...]}, empty when nothing matches
//
// POST rather than GET so the name never lands in a URL, a query string or the
// access log. The match is the WHOLE name (case-insensitively): no prefix or
// substring search, so the endpoint can't be walked to discover who signed up. A
// miss returns an empty list rather than a 404 - "no such participant" and "that
// participant holds no cards on an open rally" are deliberately indistinguishable.
func (s *Server) handleStampLookup(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.rallyLimiter.isLimited(ip) {
		slog.Warn("stamp rally lookup rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many lookups. Please try again later.")
		return
	}
	s.rallyLimiter.recordFailure(ip)

	req, err := readJSON[stampLookupRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Enter the name you signed up with")
		return
	}
	world := strings.TrimSpace(req.World)
	if world == "" {
		name, world = model.SplitParticipantLabel(name)
	}
	entries, err := s.store.LookupParticipantCards(name, world)
	if err != nil {
		writeInternalError(w, "look up participant cards", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StampLookupResponse{Entries: entries})
}
