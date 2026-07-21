package apidoc

import "github.com/getkin/kin-openapi/openapi3"

// ── small schema/builder helpers ─────────────────────────────────────────────

func ref(name string) *openapi3.SchemaRef { return openapi3.NewSchemaRef("#/components/schemas/"+name, nil) }

func desc(s *openapi3.Schema, d string) *openapi3.SchemaRef {
	s.Description = d
	return openapi3.NewSchemaRef("", s)
}

func pstr(d string) *openapi3.SchemaRef  { return desc(openapi3.NewStringSchema(), d) }
func pint(d string) *openapi3.SchemaRef  { return desc(openapi3.NewInt64Schema(), d) }
func pnum(d string) *openapi3.SchemaRef  { return desc(openapi3.NewFloat64Schema(), d) }
func pbool(d string) *openapi3.SchemaRef { return desc(openapi3.NewBoolSchema(), d) }
func parr(d string, item *openapi3.SchemaRef) *openapi3.SchemaRef {
	s := openapi3.NewArraySchema()
	s.Items = item
	return desc(s, d)
}

// pbinary / pfiles document multipart file parts (a single file / repeated files).
func pbinary(d string) *openapi3.SchemaRef {
	s := openapi3.NewStringSchema()
	s.Format = "binary"
	return desc(s, d)
}
func pfiles(d string) *openapi3.SchemaRef { return parr(d, pbinary("")) }

// props builds an ordered property map from name/schema pairs.
func props(pairs ...any) openapi3.Schemas {
	m := openapi3.Schemas{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1].(*openapi3.SchemaRef)
	}
	return m
}

// jsonResp is a JSON response referencing a named component schema.
func jsonResp(desc, schema string) *openapi3.ResponseRef {
	r := openapi3.NewResponse().WithDescription(desc)
	if schema != "" {
		r = r.WithJSONSchemaRef(ref(schema))
	}
	return &openapi3.ResponseRef{Value: r}
}

// errResp is the shared {"error": "..."} response.
func errResp(desc string) *openapi3.ResponseRef {
	return &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription(desc).WithJSONSchemaRef(ref("Error"))}
}

// actionBody builds an action-dispatcher request body: an object with an `action`
// enum plus the union of fields the actions use (each action uses a subset — see
// the operation description). Pass actions=nil for a plain (non-dispatch) body.
func actionBody(desc string, actions []string, p openapi3.Schemas) *openapi3.RequestBodyRef {
	s := openapi3.NewObjectSchema()
	s.Properties = p
	if len(actions) > 0 {
		a := openapi3.NewStringSchema()
		a.Description = "The operation to perform."
		for _, v := range actions {
			a.Enum = append(a.Enum, v)
		}
		if s.Properties == nil {
			s.Properties = openapi3.Schemas{}
		}
		s.Properties["action"] = openapi3.NewSchemaRef("", a)
		s.Required = []string{"action"}
	}
	rb := openapi3.NewRequestBody().WithDescription(desc).WithJSONSchemaRef(openapi3.NewSchemaRef("", s))
	rb.Required = true
	return &openapi3.RequestBodyRef{Value: rb}
}

// multipartBody documents a multipart/form-data upload.
func multipartBody(desc string, p openapi3.Schemas) *openapi3.RequestBodyRef {
	s := openapi3.NewObjectSchema()
	s.Properties = p
	mt := openapi3.NewMediaType().WithSchemaRef(openapi3.NewSchemaRef("", s))
	rb := openapi3.NewRequestBody().WithDescription(desc)
	rb.Content = openapi3.Content{"multipart/form-data": mt}
	rb.Required = true
	return &openapi3.RequestBodyRef{Value: rb}
}

// pb accumulates operations onto the document.
type pb struct{ doc *openapi3.T }

type opt struct {
	query []*openapi3.Parameter
	path  []*openapi3.Parameter
	body  *openapi3.RequestBodyRef
	resps []respEntry
}
type respEntry struct {
	code string
	r    *openapi3.ResponseRef
}

func qparam(name, desc string, required bool) *openapi3.Parameter {
	pp := openapi3.NewQueryParameter(name).WithDescription(desc).WithSchema(openapi3.NewStringSchema())
	pp.Required = required
	return pp
}
func pparam(name, desc string) *openapi3.Parameter {
	return openapi3.NewPathParameter(name).WithDescription(desc).WithSchema(openapi3.NewStringSchema())
}

// add registers one operation. auth is one of: "public", "auth", "admin",
// "permission:<key>", "any-bookclub" — it sets the security requirement and is
// echoed into the description.
func (b *pb) add(method, path, tag, summary, auth, descExtra string, o opt) {
	op := openapi3.NewOperation()
	op.Tags = []string{tag}
	op.Summary = summary
	d := "**Auth:** " + auth
	if descExtra != "" {
		d += "\n\n" + descExtra
	}
	op.Description = d
	if auth != "public" {
		op.Security = &openapi3.SecurityRequirements{
			{"cookieAuth": {}}, {"bearerAuth": {}},
		}
	} else {
		op.Security = &openapi3.SecurityRequirements{} // explicitly no auth
	}
	for _, q := range o.query {
		op.AddParameter(q)
	}
	for _, pp := range o.path {
		op.AddParameter(pp)
	}
	if o.body != nil {
		op.RequestBody = o.body
	}
	resps := openapi3.NewResponsesWithCapacity(len(o.resps))
	for _, re := range o.resps {
		resps.Set(re.code, re.r)
	}
	op.Responses = resps

	pi := b.doc.Paths.Value(path)
	if pi == nil {
		pi = &openapi3.PathItem{}
		b.doc.Paths.Set(path, pi)
	}
	pi.SetOperation(method, op)
}

// ok/created/noContent/errs are response-list shorthands.
func ok(schema string) respEntry    { return respEntry{"200", jsonResp("Success", schema)} }
func created(s string) respEntry    { return respEntry{"201", jsonResp("Created", s)} }
func noContent() respEntry          { return respEntry{"204", jsonResp("No Content", "")} }
func r(code, desc string) respEntry { return respEntry{code, errResp(desc)} }

// rawResp is a 200 response with a non-JSON media type (e.g. the generated font
// kit CSS, or font bytes — binary sets the string schema's format accordingly).
func rawResp(desc, mediaType string, binary bool) respEntry {
	s := openapi3.NewStringSchema()
	if binary {
		s.Format = "binary"
	}
	resp := openapi3.NewResponse().WithDescription(desc)
	resp.Content = openapi3.Content{mediaType: openapi3.NewMediaType().WithSchemaRef(openapi3.NewSchemaRef("", s))}
	return respEntry{"200", &openapi3.ResponseRef{Value: resp}}
}

func buildPaths(doc *openapi3.T) {
	b := &pb{doc: doc}

	// ── System ──────────────────────────────────────────────────────────────
	b.add("GET", "/api/version", "System", "Backend version", "public", "", opt{
		resps: []respEntry{{"200", jsonResp("The backend semver", "")}}})
	b.add("GET", "/api/config", "System", "Client bootstrap config", "public",
		"Returns the non-secret Cloudflare Turnstile site key (empty when disabled).", opt{
			resps: []respEntry{{"200", jsonResp("Config", "")}}})

	// ── Auth ────────────────────────────────────────────────────────────────
	b.add("GET", "/api/auth", "Auth", "Current auth status + user", "public", "", opt{
		resps: []respEntry{ok("AuthCheckResponse")}})
	b.add("POST", "/api/auth", "Auth", "Log in or out", "public",
		"`login` (default) verifies credentials (IP rate-limited, Turnstile-gated when configured) and sets the session cookie; `logout` destroys the session.",
		opt{
			body: actionBody("Login or logout.", []string{"login", "logout"}, props(
				"username", pstr("Account username (login)."),
				"password", pstr("Account password (login)."),
				"turnstile_token", pstr("Cloudflare Turnstile token, when the bot check is enabled."),
			)),
			resps: []respEntry{
				{"200", jsonResp("Logged in (LoginResponse) or logged out (LogoutResponse)", "LoginResponse")},
				r("400", "Invalid JSON"), r("401", "Invalid username or password"),
				r("403", "Account pending activation / bot check failed"), r("429", "Too many attempts"),
			}})
	b.add("POST", "/api/register", "Auth", "Register a (pending) account", "public",
		"Hidden registration: creates an inactive, non-admin account an admin must activate. IP rate-limited and Turnstile-gated when configured.",
		opt{
			body: actionBody("New account credentials.", nil, props(
				"username", pstr("1–32 chars; may not be the reserved `admin`."),
				"password", pstr("At least 8 characters."),
				"turnstile_token", pstr("Turnstile token, when enabled."),
			)),
			resps: []respEntry{ok("RegisterResponse"), r("400", "Validation failed"), r("409", "Username taken"), r("429", "Too many attempts")}})

	// ── Users & Account ───────────────────────────────────────────────────────
	b.add("GET", "/api/users", "Users & Account", "List accounts", "admin", "", opt{
		resps: []respEntry{ok("UsersResponse"), r("401", "Unauthorized")}})
	b.add("PATCH", "/api/users/{id}", "Users & Account", "Update an account", "admin",
		"Partial update — any supplied field is applied: `active`, `admin`, `permissions` (page-permission keys), `password` (min 8). The reserved `admin` account is protected (its `active`/`admin`/`password` can't be changed here; `permissions` may).",
		opt{
			path: []*openapi3.Parameter{pparam("id", "Target user id.")},
			body: actionBody("Account fields to update.", nil, props(
				"active", pbool("Activate/deactivate."), "admin", pbool("Grant/revoke admin."),
				"permissions", parr("Page-permission keys.", pstr("")),
				"password", pstr("New password (min 8)."),
			)),
			resps: []respEntry{ok("OKResponse"), r("400", "Validation failed"), r("401", "Unauthorized"), r("403", "Protected account"), r("404", "User not found")}})
	b.add("DELETE", "/api/users/{id}", "Users & Account", "Delete an account", "admin",
		"The reserved `admin` account is protected and cannot be deleted.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Target user id.")},
			resps: []respEntry{noContent(), r("401", "Unauthorized"), r("403", "Protected account"), r("404", "User not found")}})
	b.add("POST", "/api/account/change-password", "Users & Account", "Change own password", "auth", "", opt{
		body: actionBody("Self-service password change.", nil, props(
			"current_password", pstr("Current password."), "new_password", pstr("New password (min 8)."))),
		resps: []respEntry{ok("OKResponse"), r("400", "Short password"), r("401", "Current password incorrect")}})
	b.add("GET", "/api/account/token", "Users & Account", "Token metadata", "auth",
		"Personal access token metadata (never the secret). Shape is the `TokenInfo` schema.", opt{
			resps: []respEntry{ok("TokenInfo"), r("401", "Unauthorized")}})
	b.add("POST", "/api/account/token", "Users & Account", "Generate a token", "auth",
		"Mints a `pat_…` (plaintext returned once), replacing any existing token.", opt{
			resps: []respEntry{ok("AccountTokenGenerateResponse"), r("401", "Unauthorized")}})
	b.add("DELETE", "/api/account/token", "Users & Account", "Revoke a token", "auth",
		"Deletes the account's token, reporting whether a row was removed.", opt{
			resps: []respEntry{ok("TokenRevokeResponse"), r("401", "Unauthorized")}})
	// Passkeys (WebAuthn). The begin endpoints return standard WebAuthn options
	// (PublicKeyCredentialCreation/RequestOptions) rather than a model schema.
	b.add("POST", "/api/auth/passkey/begin", "Users & Account", "Begin passkey login", "public",
		"Starts a usernameless (discoverable) WebAuthn login; returns the credential request options (challenge).", opt{
			resps: []respEntry{r("200", "PublicKey credential request options"), r("500", "Could not start")}})
	b.add("POST", "/api/auth/passkey/finish", "Users & Account", "Finish passkey login", "public",
		"Verifies the WebAuthn assertion and, on success, establishes the session (as a password login does).", opt{
			resps: []respEntry{ok("LoginResponse"), r("400", "No login in progress"), r("401", "Passkey login failed"), r("429", "Too many attempts")}})
	b.add("POST", "/api/account/passkeys/register/begin", "Users & Account", "Begin passkey registration", "auth",
		"Starts registering a new passkey for the logged-in account; returns the credential creation options.", opt{
			resps: []respEntry{r("200", "PublicKey credential creation options"), r("401", "Unauthorized")}})
	b.add("POST", "/api/account/passkeys/register/finish", "Users & Account", "Finish passkey registration", "auth",
		"Verifies the attestation and stores the credential. The passkey label comes from the `name` query.", opt{
			query: []*openapi3.Parameter{qparam("name", "Friendly passkey label.", false)},
			resps: []respEntry{ok("PasskeysResponse"), r("400", "Registration failed"), r("401", "Unauthorized"), r("409", "Already registered")}})
	b.add("GET", "/api/account/passkeys", "Users & Account", "List own passkeys", "auth",
		"The logged-in account's passkeys (metadata only).", opt{
			resps: []respEntry{ok("PasskeysResponse"), r("401", "Unauthorized")}})
	b.add("DELETE", "/api/account/passkeys/{id}", "Users & Account", "Delete a passkey", "auth", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Passkey row id.")},
		resps: []respEntry{ok("PasskeysResponse"), r("401", "Unauthorized"), r("404", "Passkey not found")}})

	// ── Bingo: board / cards / game ───────────────────────────────────────────
	b.add("GET", "/api/board", "Bingo", "Player board (card + game)", "public", "", opt{
		query: []*openapi3.Parameter{qparam("id", "6-char card ID (required).", true), qparam("preview", "Any non-empty value returns only the card.", false)},
		resps: []respEntry{{"200", jsonResp("BoardResponse (full) or CardResponse (preview)", "BoardResponse")}, r("400", "Board ID required"), r("404", "Board not found")}})
	b.add("GET", "/api/cards", "Bingo", "List cards", "permission:bingo-cards", "", opt{
		resps: []respEntry{ok("CardsListResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/cards", "Bingo", "Create a named card", "permission:bingo-cards",
		"Generates one card, optionally assigned to `player_name`.", opt{
			body:  actionBody("New named card.", nil, props("player_name", pstr("Player name to assign (optional)."))),
			resps: []respEntry{created("GenerateSingleCardResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/cards/generate", "Bingo", "Bulk-generate cards", "permission:bingo-cards",
		"Generates `count` random cards (clamped 1–500).", opt{
			body:  actionBody("Bulk generate.", nil, props("count", pint("Number of cards to generate (1–500)."))),
			resps: []respEntry{created("GenerateCardsResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/cards/request", "Bingo", "Submit a custom card request", "public",
		"Public Personal Card Request: submit a hand-built bingo card with a chosen 6-char ID, character name, and world. Validates the board and rejects a taken ID or a duplicate board; on success the card is stored pending staff approval (not yet playable). Rate-limited; Cloudflare Turnstile when configured.", opt{
			body: actionBody("Custom card request.", nil, props(
				"character_name", pstr("Requester's character name."),
				"world", pstr("Requester's home world."),
				"card_id", pstr("Chosen 6-character alphanumeric card ID."),
				"board_data", parr("5×5 grid of numbers (row-major; centre 0 = FREE).", parr("", pint("Cell number."))),
				"turnstile_token", pstr("Cloudflare Turnstile token (when enabled)."))),
			resps: []respEntry{created("CardRequestResponse"), r("400", "Invalid card or fields"), r("409", "ID taken or duplicate card"), r("429", "Too many requests")}})
	b.add("DELETE", "/api/cards/all", "Bingo", "Delete all cards", "permission:bingo-cards",
		"Removes every non-Protected card, reporting how many were deleted. Protected cards are spared.", opt{
			resps: []respEntry{ok("DeletedCountResponse"), r("401", "Unauthorized")}})
	b.add("DELETE", "/api/cards/{id}", "Bingo", "Delete a card", "permission:bingo-cards", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Card id.")},
		resps: []respEntry{noContent(), r("401", "Unauthorized")}})
	b.add("PATCH", "/api/cards/{id}", "Bingo", "Update a card's player", "permission:bingo-cards",
		"Updates the assigned player name and details.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Card id.")},
			body:  actionBody("Card player fields.", nil, props("player_name", pstr("Player name."), "details", pstr("Cardholder details."))),
			resps: []respEntry{ok("OKResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/cards/{id}/approve", "Bingo", "Approve a pending custom card", "permission:bingo-cards",
		"Approves a pending custom-card request: it becomes playable and is automatically marked Protected.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Card id.")},
			resps: []respEntry{ok("OKResponse"), r("401", "Unauthorized"), r("404", "No pending custom card with that ID")}})
	b.add("POST", "/api/cards/{id}/protect", "Bingo", "Set a card's Protected flag", "permission:bingo-cards",
		"Marks or unmarks a card as Protected. Protected cards are spared by Delete All (still deletable individually).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Card id.")},
			body:  actionBody("Protected flag.", nil, props("protected", pbool("Whether the card is Protected."))),
			resps: []respEntry{ok("OKResponse"), r("401", "Unauthorized"), r("404", "Card not found")}})
	b.add("GET", "/api/game", "Bingo", "Current game state", "public", "", opt{
		resps: []respEntry{ok("GameStateResponse")}})
	b.add("POST", "/api/game/start", "Bingo", "Start a game", "permission:bingo-game", "", opt{
		body: actionBody("Game start.", nil, props(
			"pattern_ids", parr("Win pattern ids (≥1).", pint("")),
			"auto", pbool("Start with the automatic-draw loop running."),
			"auto_interval", pint("Seconds between automatic draws (\"Time Between Calls\")."))),
		resps: []respEntry{ok("GameStateResponse"), r("400", "No pattern selected"), r("401", "Unauthorized")}})
	b.add("POST", "/api/game/draw", "Bingo", "Draw a number", "permission:bingo-game",
		"Draws the next number; `delay` (0–60s) delays the player broadcast.", opt{
			body:  actionBody("Draw.", nil, props("delay", pint("Player broadcast delay seconds (0–60)."))),
			resps: []respEntry{ok("DrawResult"), r("400", "No active game / all drawn"), r("401", "Unauthorized")}})
	b.add("POST", "/api/game/end", "Bingo", "End the game", "permission:bingo-game",
		"Ends the active game, logging the confirmed valid winners.", opt{
			body:  actionBody("End.", nil, props("valid_winner_ids", parr("Confirmed winner card ids.", pstr("")))),
			resps: []respEntry{ok("EndGameResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/game/halftime", "Bingo", "Answer the half-time prompt", "permission:bingo-game",
		"Records the admin's half-time decision: `minigame` true alerts players about a mini-game (held until the triggering number has reached them, and auto stays paused); false declines it and resumes auto if it was paused for the prompt. An empty body defaults to true.", opt{
			body:  actionBody("Half-time choice.", nil, props("minigame", pbool("Alert players about a mini-game (true) or decline and resume auto (false)."))),
			resps: []respEntry{ok("OKResponse"), r("401", "Unauthorized")}})
	b.add("POST", "/api/game/yoever", "Bingo", "Trigger \"It's Yoever\"", "public",
		"Broadcasts the \"It's Yoever\" reaction (sound + a bouncing image labelled with the player's name) to every connected client. Public, but each board is throttled to one trigger per `yoever_cooldown_seconds` and an admin can switch the reaction off per game.", opt{
			body: actionBody("Trigger.", nil, props("card_id", pstr("Triggering board id (required)."))),
			resps: []respEntry{
				ok("YoeverResponse"),
				r("400", "Missing board id"),
				r("403", "Reaction switched off"),
				r("404", "Board not found"),
				r("409", "No active game"),
				r("429", "On cooldown (Retry-After)")}})
	b.add("PATCH", "/api/game", "Bingo", "Update game controls", "permission:bingo-game",
		"Partial update: `delay` (0–60) persists the shared draw delay; `details` sets the markdown game details; `yoever_enabled` toggles the \"It's Yoever\" reaction; `auto_enabled` switches the automatic-draw loop on/off and `auto_interval` adjusts the seconds between auto draws (live, never written back to a preset). Any combination may be supplied.", opt{
			body: actionBody("Game controls.", nil, props(
				"delay", pint("Shared draw delay seconds (0–60)."),
				"details", pstr("Markdown game details."),
				"yoever_enabled", pbool("Switch the \"It's Yoever\" reaction on/off."),
				"auto_enabled", pbool("Switch the automatic-draw loop on/off."),
				"auto_interval", pint("Seconds between automatic draws (\"Time Between Calls\")."))),
			resps: []respEntry{ok("OKResponse"), r("400", "Draw delay out of range"), r("401", "Unauthorized")}})

	// ── Bingo: patterns / categories / presets / styles ───────────────────────
	b.add("GET", "/api/patterns", "Bingo", "List patterns + categories", "permission:bingo-patterns", "", opt{resps: []respEntry{ok("PatternsResponse")}})
	b.add("POST", "/api/patterns", "Bingo", "Create a pattern", "permission:bingo-patterns",
		"Creates a win pattern (rejects a duplicate 5×5 grid).", opt{
			body: actionBody("New pattern.", nil, props(
				"name", pstr("Pattern name (required)."),
				"pattern_data", parr("5×5 boolean grid (required).", parr("", pbool(""))),
				"category_id", pint("Owning category id."))),
			resps: []respEntry{created("PatternCreateResponse"), r("400", "Invalid"), r("409", "Duplicate pattern")}})
	b.add("POST", "/api/patterns/reorder", "Bingo", "Reorder patterns in a category", "permission:bingo-patterns",
		"Persists a new drag-and-drop order for a category's patterns; returns the fresh patterns + categories snapshot.", opt{
			body: actionBody("Bulk reorder.", nil, props(
				"category_id", pint("Category whose patterns are being ordered."),
				"ordered_ids", parr("Pattern ids in the new order.", pint("")))),
			resps: []respEntry{ok("PatternsResponse"), r("400", "Invalid")}})
	b.add("PATCH", "/api/patterns/{id}", "Bingo", "Update a pattern", "permission:bingo-patterns",
		"Partial update: `name` renames, `category_id` moves it to a category, `direction` (up|down) reorders within its category. A `direction` move returns the fresh PatternsResponse; a pure rename/move returns OKResponse.", opt{
			path: []*openapi3.Parameter{pparam("id", "Pattern id.")},
			body: actionBody("Pattern fields.", nil, props(
				"name", pstr("New pattern name."),
				"category_id", pint("Move to this category id."),
				"direction", pstr("Reorder within category: up|down."))),
			resps: []respEntry{{"200", jsonResp("PatternsResponse (direction move) or OKResponse", "OKResponse")}, r("400", "Invalid")}})
	b.add("DELETE", "/api/patterns/{id}", "Bingo", "Delete a pattern", "permission:bingo-patterns", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Pattern id.")},
		resps: []respEntry{noContent(), r("400", "Invalid")}})
	b.add("GET", "/api/pattern-categories", "Bingo", "List categories", "permission:bingo-patterns", "", opt{resps: []respEntry{ok("CategoriesResponse")}})
	b.add("POST", "/api/pattern-categories", "Bingo", "Create a category", "permission:bingo-patterns", "", opt{
		body:  actionBody("New category.", nil, props("name", pstr("Category name (required)."))),
		resps: []respEntry{created("CategoryCreateResponse"), r("400", "Invalid")}})
	b.add("POST", "/api/pattern-categories/reorder", "Bingo", "Reorder categories", "permission:bingo-patterns",
		"Persists a new order for all categories; returns the fresh categories list.", opt{
			body:  actionBody("Bulk reorder.", nil, props("ordered_ids", parr("Category ids in the new order.", pint("")))),
			resps: []respEntry{ok("CategoriesResponse"), r("400", "Invalid")}})
	b.add("PATCH", "/api/pattern-categories/{id}", "Bingo", "Update a category", "permission:bingo-patterns",
		"Partial update: `name` renames, `direction` (up|down) reorders. A `direction` move returns the fresh CategoriesResponse; a pure rename returns OKResponse.", opt{
			path: []*openapi3.Parameter{pparam("id", "Category id.")},
			body: actionBody("Category fields.", nil, props(
				"name", pstr("New category name."),
				"direction", pstr("Reorder: up|down."))),
			resps: []respEntry{{"200", jsonResp("CategoriesResponse (direction move) or OKResponse", "OKResponse")}, r("400", "Invalid")}})
	b.add("DELETE", "/api/pattern-categories/{id}", "Bingo", "Delete a category", "permission:bingo-patterns",
		"Refuses to delete the last remaining category (409).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Category id.")},
			resps: []respEntry{noContent(), r("409", "Cannot delete the last category")}})
	presetFields := func() openapi3.Schemas {
		return props("name", pstr("Preset name (required)."),
			"pattern_ids", parr("Win pattern ids (≥1).", pint("")), "game_details", pstr("Markdown details."),
			"auto", pbool("Pre-select the automatic-draw toggle when applied."),
			"auto_interval", pint("Seconds between automatic draws (\"Time Between Calls\")."))
	}
	b.add("GET", "/api/presets", "Bingo", "List presets", "permission:bingo-presets", "", opt{resps: []respEntry{ok("PresetsResponse")}})
	b.add("POST", "/api/presets", "Bingo", "Create a preset", "permission:bingo-presets", "", opt{
		body:  actionBody("Preset fields.", nil, presetFields()),
		resps: []respEntry{created("PresetCreateResponse"), r("400", "Invalid")}})
	b.add("PUT", "/api/presets/{id}", "Bingo", "Replace a preset", "permission:bingo-presets", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Preset id.")},
		body:  actionBody("Preset fields.", nil, presetFields()),
		resps: []respEntry{ok("OKResponse"), r("400", "Invalid")}})
	b.add("DELETE", "/api/presets/{id}", "Bingo", "Delete a preset", "permission:bingo-presets", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Preset id.")},
		resps: []respEntry{noContent()}})
	b.add("GET", "/api/styles", "Bingo", "List styles", "permission:system-themes", "", opt{resps: []respEntry{ok("StylesResponse")}})
	b.add("POST", "/api/styles", "Bingo", "Create a theme", "permission:system-themes", "", opt{
		body: actionBody("New theme.", nil, props(
			"name", pstr("Style name (required)."),
			"tokens", desc(openapi3.NewObjectSchema(), "Design-token overrides (name→CSS value)."),
			"board_flourish", pstr("Board flourish path."), "number_flourish", pstr("Number flourish path."),
			"is_public", pbool("Whether the theme is selectable in the client-side picker (default false = admin-only)."))),
		resps: []respEntry{created("StyleCreateResponse"), r("400", "Invalid")}})
	b.add("POST", "/api/styles/deactivate", "Bingo", "Deactivate the active theme", "permission:system-themes",
		"Clears the active style and broadcasts an empty style_update, reverting every client to the default look.", opt{
			resps: []respEntry{ok("OKResponse")}})
	b.add("GET", "/api/styles/active", "Bingo", "Active theme CSS", "public", "", opt{resps: []respEntry{ok("ActiveCSSResponse")}})
	b.add("GET", "/api/styles/public", "Bingo", "List public themes", "public",
		"The Public themes an end user may pick for themselves (id + name only). Private themes are never listed.", opt{
			resps: []respEntry{ok("PublicStylesResponse")}})
	b.add("GET", "/api/styles/public/{id}", "Bingo", "Public theme CSS", "public",
		"Generated CSS + flourishes of a single Public theme. 404 for a Private or missing theme, so Private CSS can't be fetched by id.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Style id.")},
			resps: []respEntry{ok("ActiveCSSResponse"), r("404", "Theme not found")}})
	b.add("GET", "/api/styles/{id}", "Bingo", "Get a theme", "permission:system-themes", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Style id.")},
		resps: []respEntry{ok("StyleGetResponse"), r("404", "Style not found")}})
	b.add("PUT", "/api/styles/{id}", "Bingo", "Replace a theme", "permission:system-themes",
		"Full replace. If this is the active style, the regenerated CSS is broadcast (style_update).", opt{
			path: []*openapi3.Parameter{pparam("id", "Style id.")},
			body: actionBody("Theme fields.", nil, props(
				"name", pstr("Style name (required)."),
				"tokens", desc(openapi3.NewObjectSchema(), "Design-token overrides (name→CSS value)."),
				"board_flourish", pstr("Board flourish path."), "number_flourish", pstr("Number flourish path."),
				"is_public", pbool("Whether the theme is selectable in the client-side picker."))),
			resps: []respEntry{ok("OKResponse"), r("400", "Invalid")}})
	b.add("DELETE", "/api/styles/{id}", "Bingo", "Delete a theme", "permission:system-themes",
		"If the deleted style was active, the active setting is cleared and an empty style_update reverts clients.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Style id.")},
			resps: []respEntry{noContent()}})
	b.add("POST", "/api/styles/{id}/activate", "Bingo", "Activate a theme", "permission:system-themes",
		"Makes the style active and broadcasts its CSS (style_update).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Style id.")},
			resps: []respEntry{ok("OKResponse")}})

	buildFeaturePaths(b)
}
