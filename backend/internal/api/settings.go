package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/bookclubs"
	"app-suite/internal/db"
	"app-suite/internal/fonts"
	"app-suite/internal/game"
	"app-suite/internal/origins"
	"app-suite/internal/permissions"
	"app-suite/internal/settings"
	"app-suite/internal/store"
)

// SettingsInput is the site's settings as a save sends them: every one, since a save
// replaces them all.
type SettingsInput struct {
	AppTitle                string     `json:"appTitle" maxLength:"200" doc:"The site's name, shown in the browser tab and the header."`
	DefaultDrawDelay        int        `json:"defaultDrawDelay" minimum:"0" maximum:"60" doc:"Seconds a drawn number waits before players see it, so the caller can announce it first."`
	FrequentWinnerThreshold int        `json:"frequentWinnerThreshold" minimum:"1" maximum:"100" doc:"Wins within frequentWinnerHours that mark a player as a frequent winner."`
	FrequentWinnerHours     int        `json:"frequentWinnerHours" minimum:"1" maximum:"168" doc:"The hours a frequent winner's wins are counted over."`
	HeaderFont              string     `json:"headerFont" maxLength:"100" doc:"The font family headings use: a Google Font or an uploaded font's family."`
	AniListAPIURL           string     `json:"aniListApiUrl" maxLength:"200" doc:"The AniList GraphQL endpoint book clubs look titles up at: https, under anilist.co. Empty for the default."`
	BingoJoinPrompt         string     `json:"bingoJoinPrompt" maxLength:"500" doc:"What the home page asks a player who is joining a board."`
	YoeverCooldownSeconds   int        `json:"yoeverCooldownSeconds" minimum:"0" maximum:"3600" doc:"Seconds a card waits between Yoever reactions."`
	CustomCardCost          int        `json:"customCardCost" minimum:"0" maximum:"1000000000" doc:"What a personal card costs, in gil, shown on the custom card request page."`
	HideBingo               bool       `json:"hideBingo" doc:"Whether bingo is hidden from the public pages."`
	HideCustomCards         bool       `json:"hideCustomCards" doc:"Whether the custom card request is hidden while the rest of bingo stays."`
	TeaRoomOrigins          OriginList `json:"teaRoomOrigins" nullable:"false" doc:"The sites whose pages may read the public tea-room API from a browser, each as an origin such as https://senpan.cafe. https://*.example.com covers every subdomain of example.com. Empty for none."`
	UpdatedAt               string     `json:"updatedAt,omitempty" doc:"When the settings were last saved, as read. Sent with a save, it must still be the latest: a save made from an older copy is answered 409, so a page left open can't undo what was saved since. Left out, the save is not checked."`
}

// OriginList is up to twenty origins, each up to 200 characters.
type OriginList []string

// Schema is an array of at most 20 strings, none null.
func (OriginList) Schema(huma.Registry) *huma.Schema {
	most, longest := 20, 200
	return &huma.Schema{
		Type: huma.TypeArray, MaxItems: &most,
		Items: &huma.Schema{Type: huma.TypeString, MaxLength: &longest},
	}
}

// Settings is the site's settings as anyone reads them, with the uploaded fonts a
// page registers to show them.
type Settings struct {
	SettingsInput
	UploadedFonts []UploadedFont `json:"uploadedFonts" nullable:"false" doc:"The uploaded fonts, each with a serving token, for the header-font picker and the pages that use one."`
}

// UploadedFont is an uploaded font a page can use.
type UploadedFont struct {
	Name   string `json:"name" doc:"The uploaded file's name."`
	Family string `json:"family" doc:"The CSS font family it is served as."`
	Token  string `json:"token" doc:"The token its file is served under (GET /api/fonts/pub/f/{token}); it rotates, so a page uses the one it was given."`
}

// Integrations is the key the server uses with an outside service: admin-only, since
// it grants access there.
type Integrations struct {
	GoogleFontsAPIKey string `json:"googleFontsApiKey" maxLength:"200" doc:"The Google Fonts API key the header-font picker lists fonts with. Empty for none."`
}

type settingsOutput struct {
	Body Settings
}

type settingsInput struct {
	Body SettingsInput
}

type integrationsOutput struct {
	Body Integrations
}

type integrationsInput struct {
	Body Integrations
}

func (h handlers) registerSettings(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-settings",
		Method:      http.MethodGet,
		Path:        "/settings",
		Summary:     "The site's settings",
		Description: "Public: every page reads them. Nothing secret is among them (see the integrations).",
		Tags:        []string{"Settings"},
		Middlewares: huma.Middlewares{public},
	}, h.getSettings)

	huma.Register(api, huma.Operation{
		OperationID: "update-settings",
		Method:      http.MethodPut,
		Path:        "/settings",
		Summary:     "Replace the site's settings",
		Description: "Every board and console hears of it on its live stream (settings_changed) and reads them " +
			"again; a changed draw delay reaches the Game page too (draw_delay).",
		Tags:        []string{"Settings"},
		Security:    secured,
		Middlewares: huma.Middlewares{h.permitted(api, permissions.SystemSettings)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity},
	}, h.updateSettings)

	huma.Register(api, huma.Operation{
		OperationID: "get-integrations",
		Method:      http.MethodGet,
		Path:        "/settings/integrations",
		Summary:     "The keys the server uses with outside services",
		Tags:        []string{"Settings"},
		Security:    secured,
		Middlewares: huma.Middlewares{h.admin(api)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden},
	}, h.getIntegrations)

	huma.Register(api, huma.Operation{
		OperationID: "update-integrations",
		Method:      http.MethodPut,
		Path:        "/settings/integrations",
		Summary:     "Replace the keys the server uses with outside services",
		Tags:        []string{"Settings"},
		Security:    secured,
		Middlewares: huma.Middlewares{h.admin(api)},
		Errors:      []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity},
	}, h.updateIntegrations)
}

// siteKeys is every key the site's settings are kept under.
var siteKeys = []string{
	settings.AppTitle, settings.DefaultDrawDelay, settings.FrequentWinnerThreshold, settings.FrequentWinnerHours,
	settings.HeaderFont, settings.AniListAPIURL, settings.BingoJoinPrompt, settings.YoeverCooldownSeconds,
	settings.CustomCardCost, settings.HideBingo, settings.HideCustomCards, settings.TeaRoomOrigins, settings.UpdatedAt,
}

// loadSettings reads the site's settings, each its default when never saved or
// saved empty.
func loadSettings(ctx context.Context, q *db.Queries) (SettingsInput, error) {
	rows, err := q.ListSettings(ctx, siteKeys)
	if err != nil {
		return SettingsInput{}, err
	}
	stored := make(map[string]string, len(rows))
	for _, r := range rows {
		stored[r.Key] = r.Value
	}
	text := func(key string) string {
		if v := stored[key]; v != "" {
			return v
		}
		return settings.Defaults[key]
	}
	number := func(key string) int {
		n, err := strconv.Atoi(text(key))
		if err != nil {
			n, _ = strconv.Atoi(settings.Defaults[key])
		}
		return n
	}
	return SettingsInput{
		AppTitle:                text(settings.AppTitle),
		DefaultDrawDelay:        number(settings.DefaultDrawDelay),
		FrequentWinnerThreshold: number(settings.FrequentWinnerThreshold),
		FrequentWinnerHours:     number(settings.FrequentWinnerHours),
		HeaderFont:              text(settings.HeaderFont),
		AniListAPIURL:           text(settings.AniListAPIURL),
		BingoJoinPrompt:         text(settings.BingoJoinPrompt),
		YoeverCooldownSeconds:   number(settings.YoeverCooldownSeconds),
		CustomCardCost:          number(settings.CustomCardCost),
		HideBingo:               text(settings.HideBingo) == "1",
		HideCustomCards:         text(settings.HideCustomCards) == "1",
		TeaRoomOrigins:          OriginList(parseOrigins(stored[settings.TeaRoomOrigins])),
		UpdatedAt:               stored[settings.UpdatedAt],
	}, nil
}

// parseOrigins reads the stored origins list; one that doesn't read as a list
// allows nothing.
func parseOrigins(stored string) []string {
	list := []string{}
	if err := json.Unmarshal([]byte(stored), &list); err != nil || list == nil {
		return []string{}
	}
	return list
}

// TeaRoomOriginAllowed reports whether origin may read the public tea-room API from a
// browser, by the saved allowlist.
func TeaRoomOriginAllowed(ctx context.Context, st *store.Store, origin string) bool {
	stored, err := db.New(st.ReadDB()).GetSetting(ctx, settings.TeaRoomOrigins)
	if err != nil {
		return false
	}
	return origins.Allowed(parseOrigins(stored), origin)
}

func (h handlers) uploadedFonts(ctx context.Context) []UploadedFont {
	list := []UploadedFont{}
	if h.UploadedFonts == nil {
		return list
	}
	for _, f := range h.UploadedFonts(ctx) {
		list = append(list, UploadedFont{Name: f.Name, Family: f.Family, Token: f.Token})
	}
	return list
}

func (h handlers) getSettings(ctx context.Context, _ *struct{}) (*settingsOutput, error) {
	s, err := loadSettings(ctx, h.read())
	if err != nil {
		return nil, h.fail("load settings", err)
	}
	return &settingsOutput{Body: Settings{SettingsInput: s, UploadedFonts: h.uploadedFonts(ctx)}}, nil
}

// check applies the rules the schema can't express, each as a 422 naming its field.
func (in SettingsInput) check() []error {
	var problems []error
	if fonts.NameUnsafe(in.HeaderFont) {
		problems = append(problems, &huma.ErrorDetail{Location: "body.headerFont", Value: in.HeaderFont,
			Message: "A font name may not contain quotes, backslashes, control characters, or any of { } ; < >."})
	}
	if in.AniListAPIURL != "" && !bookclubs.IsAllowedAniListURL(in.AniListAPIURL) {
		problems = append(problems, &huma.ErrorDetail{Location: "body.aniListApiUrl", Value: in.AniListAPIURL,
			Message: "The AniList API URL must be an https URL under anilist.co."})
	}
	for i, entry := range in.TeaRoomOrigins {
		if _, err := origins.Normalize(entry); err != nil {
			problems = append(problems, &huma.ErrorDetail{Location: "body.teaRoomOrigins[" + strconv.Itoa(i) + "]", Value: entry, Message: err.Error()})
		}
	}
	if in.UpdatedAt != "" {
		if _, err := time.Parse(time.RFC3339, in.UpdatedAt); err != nil {
			problems = append(problems, &huma.ErrorDetail{Location: "body.updatedAt", Value: in.UpdatedAt,
				Message: "updatedAt must be the time the settings were read, as they gave it."})
		}
	}
	return problems
}

// normalizedOrigins is entries in their canonical form, each once.
func normalizedOrigins(entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		n, err := origins.Normalize(e)
		if err == nil && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// nextUpdatedAt is a save's updatedAt: now, to the second, but always later than the
// last save's, so two saves within one second still tell an older copy apart.
func nextUpdatedAt(last string, now time.Time) string {
	next := now.UTC().Truncate(time.Second)
	if prev, err := time.Parse(time.RFC3339, last); err == nil && !next.After(prev) {
		next = prev.Add(time.Second)
	}
	return next.Format(time.RFC3339)
}

// errStale is a save made from an older copy of the settings.
var errStale = errors.New("stale")

func (h handlers) updateSettings(ctx context.Context, in *settingsInput) (*settingsOutput, error) {
	body := in.Body
	if problems := body.check(); len(problems) > 0 {
		return nil, huma.Error422UnprocessableEntity("Some settings can't be saved as they are.", problems...)
	}
	allowed := normalizedOrigins(body.TeaRoomOrigins)
	allowedJSON, err := json.Marshal(allowed)
	if err != nil {
		return nil, h.fail("encode the tea-room origins", err)
	}
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	values := map[string]string{
		settings.AppTitle:                strings.TrimSpace(body.AppTitle),
		settings.DefaultDrawDelay:        strconv.Itoa(body.DefaultDrawDelay),
		settings.FrequentWinnerThreshold: strconv.Itoa(body.FrequentWinnerThreshold),
		settings.FrequentWinnerHours:     strconv.Itoa(body.FrequentWinnerHours),
		settings.HeaderFont:              strings.TrimSpace(body.HeaderFont),
		settings.AniListAPIURL:           strings.TrimSpace(body.AniListAPIURL),
		settings.BingoJoinPrompt:         body.BingoJoinPrompt,
		settings.YoeverCooldownSeconds:   strconv.Itoa(body.YoeverCooldownSeconds),
		settings.CustomCardCost:          strconv.Itoa(body.CustomCardCost),
		settings.HideBingo:               flag(body.HideBingo),
		settings.HideCustomCards:         flag(body.HideCustomCards),
		settings.TeaRoomOrigins:          string(allowedJSON),
	}

	// One transaction: the check against the latest save and every value land
	// together, so two saves can't interleave and a client gone part-way leaves
	// nothing half-saved. It runs detached from the request for the same reason.
	ctx = context.WithoutCancel(ctx)
	var before SettingsInput
	err = h.transact(ctx, func(q *db.Queries) error {
		var err error
		if before, err = loadSettings(ctx, q); err != nil {
			return err
		}
		if body.UpdatedAt != "" && before.UpdatedAt != "" && body.UpdatedAt != before.UpdatedAt {
			return errStale
		}
		values[settings.UpdatedAt] = nextUpdatedAt(before.UpdatedAt, time.Now())
		for _, key := range siteKeys {
			if err := q.SetSetting(ctx, db.SetSettingParams{Key: key, Value: values[key]}); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errStale) {
		return nil, huma.Error409Conflict("The settings were saved by someone else since this page loaded them. Reload to see theirs, then save again.")
	}
	if err != nil {
		return nil, h.fail("save settings", err)
	}

	after, err := loadSettings(ctx, h.read())
	if err != nil {
		return nil, h.fail("load settings", err)
	}
	out := Settings{SettingsInput: after, UploadedFonts: h.uploadedFonts(ctx)}
	h.publish(&SettingsChangedEvent{}, everyone)
	if h.Live != nil && after.DefaultDrawDelay != before.DefaultDrawDelay {
		game.DrawDelayChanged(h.Live, after.DefaultDrawDelay)
	}
	return &settingsOutput{Body: out}, nil
}

func (h handlers) getIntegrations(ctx context.Context, _ *struct{}) (*integrationsOutput, error) {
	key, err := h.read().GetSetting(ctx, settings.GoogleFontsAPIKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, h.fail("load integrations", err)
	}
	return &integrationsOutput{Body: Integrations{GoogleFontsAPIKey: key}}, nil
}

func (h handlers) updateIntegrations(ctx context.Context, in *integrationsInput) (*integrationsOutput, error) {
	err := h.write().SetSetting(context.WithoutCancel(ctx), db.SetSettingParams{
		Key: settings.GoogleFontsAPIKey, Value: strings.TrimSpace(in.Body.GoogleFontsAPIKey),
	})
	if err != nil {
		return nil, h.fail("save integrations", err)
	}
	ResourceChanged(h.Live, "integrations")
	return h.getIntegrations(ctx, nil)
}

// transact runs f in one write transaction, committing when it returns nil.
func (h handlers) transact(ctx context.Context, f func(*db.Queries) error) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	if err := f(q); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}
