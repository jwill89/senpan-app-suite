package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/auth"
	"app-suite/internal/db"
	"app-suite/internal/discord"
)

// Signing in with Discord (OAuth 2.0's authorization code grant with PKCE, asking only
// for identify), and the links an admin hands out: one that signs someone up with their
// Discord account, and one that links a Discord account to an account that exists. Nobody
// signs up with Discord without a link, and a Discord account signs in only to the
// account it is linked to.
//
// The start, a link and Discord's callback are pages a browser opens rather than calls,
// so each answers 302 whatever happens: a failure goes to the sign-in page with a reason
// it shows (?error=), since a tab can't show a problem body. Their inputs carry no
// constraint and skip huma's checks, so nothing is refused with a 422 first.

// discordCallback is the path below Prefix that Discord sends the browser back to. The
// Discord application lists it, under each address the site runs on, exactly as it is
// here (https://apps.senpan.cafe/api/v1/auth/discord/callback).
const discordCallback = "/auth/discord/callback"

// How long things last.
const (
	// discordStateLifetime is how long a sign-in may take at Discord: the state and
	// verifier of an attempt given up on stop answering after it.
	discordStateLifetime = 10 * time.Minute
	// signUpLinkLifetime is how long an admin's sign-up link works, and
	// linkLinkLifetime a link that links a Discord account to an account, which is
	// as good as that account's password while it waits.
	signUpLinkLifetime = 7 * 24 * time.Hour
	linkLinkLifetime   = 24 * time.Hour
)

// The kinds of link (discord_invites.kind).
const (
	inviteSignUp = "register"
	inviteLink   = "link"
)

// Where the browser lands when a Discord sign-in doesn't finish: the sign-in page, with
// a reason it knows.
const (
	signInPage = "/admin/login"
	// discordDenied: declined on Discord's page.
	discordDenied = signInPage + "?error=discord_denied"
	// discordFailed: anything else, an expired or replayed start included.
	discordFailed = signInPage + "?error=discord_failed"
	// discordBusy: held back by the per-address limit.
	discordBusy = signInPage + "?error=discord_busy"
	// discordUnavailable: the server has no Discord application configured.
	discordUnavailable = signInPage + "?error=discord_unavailable"
	// discordNotLinked: no account is linked to that Discord account.
	discordNotLinked = signInPage + "?error=discord_not_linked"
	// discordInactive: the account isn't active.
	discordInactive = signInPage + "?error=discord_inactive"
	// discordLinkExpired: the admin's link is unknown, used or expired.
	discordLinkExpired = signInPage + "?error=discord_link_expired"
	// discordTaken: the Discord account is linked to another account already.
	discordTaken = signInPage + "?error=discord_taken"
)

// The admin area, where a finished sign-in lands unless the start named a page.
const adminArea = "/admin"

var (
	errLinkUsed     = errors.New("the link was used or has expired")
	errDiscordTaken = errors.New("the Discord account is linked to another account")
)

// DiscordAccount is the Discord account linked to an account, as of its last sign-in.
type DiscordAccount struct {
	Username    string `json:"username" doc:"Its Discord username, as of its last sign-in here."`
	DisplayName string `json:"displayName" doc:"The name it shows on Discord, as of its last sign-in here; empty when it has none."`
	LinkedAt    string `json:"linkedAt" doc:"When it was linked, RFC 3339 in UTC."`
}

// DiscordInvite is a link an admin just made: the only time it is shown.
type DiscordInvite struct {
	URL       string `json:"url" doc:"The link. Shown this once; it works once, for whoever opens it, so send it only to the person it is for."`
	ExpiresAt string `json:"expiresAt" doc:"When it stops working, RFC 3339 in UTC."`
}

type discordInviteOutput struct{ Body DiscordInvite }

type createDiscordInviteInput struct {
	Body struct {
		UserID *int64 `json:"userId,omitempty" minimum:"1" doc:"The account the link links a Discord account to; it replaces a link made for it before, and works for a day. Left out, the link signs someone up instead: it makes an active account with no permissions, named after their Discord account, and works for a week."`
	}
}

type redirectOutput struct {
	Status   int
	Location string `header:"Location" doc:"Where the browser goes next."`
}

type startDiscordSignInInput struct {
	Next string `query:"next" doc:"A path on this site to land on once signed in. Anything else is ignored, and the admin area is used."`
}

type openDiscordInviteInput struct {
	Token string `path:"token" doc:"The link's secret."`
}

type finishDiscordSignInInput struct {
	Code  string `query:"code" doc:"The authorization code Discord issued."`
	State string `query:"state" doc:"The state this sign-in's start sent to Discord."`
	Error string `query:"error" doc:"Discord's reason, when the sign-in was declined."`
}

func (h handlers) registerDiscordSignIn(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "start-discord-sign-in", Method: http.MethodGet, Path: "/auth/discord/start",
		Summary: "Start signing in with Discord", Tags: signInTags,
		Description: "A page the browser opens, not a call. It answers 302 to Discord's consent page, or to " +
			"`/admin/login?error=discord_unavailable` when the server has no Discord application. Thirty starts " +
			"(and links opened) from one address within 10 minutes hold it back for the rest of the 10 minutes, " +
			"answering 302 to `/admin/login?error=discord_busy`.",
		DefaultStatus: http.StatusFound, SkipValidateParams: true,
	}, h.startDiscordSignIn)
	huma.Register(api, huma.Operation{
		OperationID: "open-discord-invite", Method: http.MethodGet, Path: "/auth/discord/invite/{token}",
		Summary: "Open an admin's Discord link", Tags: signInTags,
		Description: "A page the browser opens, not a call: the link an admin made to sign someone up with Discord, " +
			"or to link a Discord account to an account. It answers 302 to Discord's consent page, the link's secret " +
			"kept in the session rather than the address bar, or to `/admin/login?error=` with `discord_link_expired` " +
			"(unknown, used or expired), `discord_busy` (the start's limit, which this shares) or `discord_unavailable`. " +
			"The link is used up only when the sign-in finishes.",
		DefaultStatus: http.StatusFound, SkipValidateParams: true,
	}, h.openDiscordInvite)
	huma.Register(api, huma.Operation{
		OperationID: "finish-discord-sign-in", Method: http.MethodGet, Path: discordCallback,
		Summary: "Finish signing in with Discord", Tags: signInTags,
		Description: "Where Discord sends the browser back. Signed in, it answers 302 to the page the start named, or " +
			"the admin area. Otherwise it answers 302 to `/admin/login?error=` with `discord_denied` (declined at Discord), " +
			"`discord_not_linked` (no account is linked to that Discord account), `discord_inactive` (the account isn't " +
			"active; a link to it is still made), `discord_link_expired` (the admin's link was used or expired meanwhile), " +
			"`discord_taken` (the Discord account is linked to another account, and the link is left unused) or " +
			"`discord_failed`. The start's state answers once, for ten minutes.",
		DefaultStatus: http.StatusFound, SkipValidateParams: true,
	}, h.finishDiscordSignIn)

	adminSession := huma.Middlewares{h.admin(api), h.session(api)}
	huma.Register(api, huma.Operation{
		OperationID: "create-discord-invite", Method: http.MethodPost, Path: "/discord-invites",
		Summary: "Make a link to sign up or link a Discord account", Tags: userTags, DefaultStatus: http.StatusCreated,
		Description: "The answer is the only time the link is shown. Only the admin account itself may make a link " +
			"to it (403). 503 when the server has no Discord application.",
		Security: secured, Middlewares: adminSession,
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable},
	}, h.createDiscordInvite)
	huma.Register(api, huma.Operation{
		OperationID: "unlink-discord", Method: http.MethodDelete, Path: "/users/{id}/discord",
		Summary: "Unlink an account's Discord account", Tags: userTags, DefaultStatus: http.StatusNoContent,
		Description: "Only the admin account itself may unlink its own (403). Refused (409) when Discord is the " +
			"account's only way to sign in: set a password for it first.",
		Security: secured, Middlewares: adminSession,
		Errors: []int{http.StatusForbidden, http.StatusNotFound, http.StatusConflict},
	}, h.unlinkDiscord)
}

func (h handlers) startDiscordSignIn(ctx context.Context, in *startDiscordSignInInput) (*redirectOutput, error) {
	if !h.DiscordOAuth.Enabled() {
		return redirectTo(discordUnavailable), nil
	}
	if !h.countDiscordStart(ctx) {
		return redirectTo(discordBusy), nil
	}
	return redirectTo(h.toDiscord(ctx, in.Next, 0)), nil
}

func (h handlers) openDiscordInvite(ctx context.Context, in *openDiscordInviteInput) (*redirectOutput, error) {
	if !h.DiscordOAuth.Enabled() {
		return redirectTo(discordUnavailable), nil
	}
	// Counted before the link is looked up, so its secret can't be guessed at speed.
	if !h.countDiscordStart(ctx) {
		return redirectTo(discordBusy), nil
	}
	invite, err := h.read().GetDiscordInvite(ctx, auth.HashToken(in.Token))
	if errors.Is(err, sql.ErrNoRows) {
		return redirectTo(discordLinkExpired), nil
	}
	if err != nil {
		h.Logger.Error("read a Discord link", "error", err)
		return redirectTo(discordFailed), nil
	}
	return redirectTo(h.toDiscord(ctx, "", invite.ID)), nil
}

// countDiscordStart counts a start (or a link opened) against the client's address, and
// reports whether it may go ahead. Each leaves a state in the session store until it
// expires, so every one counts, made or not.
func (h handlers) countDiscordStart(ctx context.Context) bool {
	ip := requestOf(ctx).IP
	if wait := retryAfter(h.DiscordLimit, ip); wait > 0 {
		h.Logger.Warn("discord sign-in held back", "ip", ip)
		return false
	}
	if h.DiscordLimit != nil {
		h.DiscordLimit.Fail(ip)
	}
	return true
}

// toDiscord begins a sign-in on the request's session and answers Discord's consent
// page. next is kept for after the callback when it is a path on this site; inviteID is
// the admin's link it began from, 0 for none. Whatever an earlier attempt kept is
// replaced, so an attempt given up on can't decide how this one ends.
func (h handlers) toDiscord(ctx context.Context, next string, inviteID int64) string {
	state, verifier := discord.NewState(), discord.NewVerifier()
	s := h.Sessions
	s.Put(ctx, auth.SessionDiscordState, state)
	s.Put(ctx, auth.SessionDiscordUntil, time.Now().Add(discordStateLifetime).Unix())
	s.Put(ctx, auth.SessionDiscordVerifier, verifier)
	s.Remove(ctx, auth.SessionDiscordNext)
	s.Remove(ctx, auth.SessionDiscordInvite)
	if sitePath(next) {
		s.Put(ctx, auth.SessionDiscordNext, next)
	}
	if inviteID != 0 {
		s.Put(ctx, auth.SessionDiscordInvite, inviteID)
	}
	return h.DiscordOAuth.AuthCodeURL(discordRedirectURL(ctx), state, verifier)
}

func (h handlers) finishDiscordSignIn(ctx context.Context, in *finishDiscordSignInInput) (*redirectOutput, error) {
	// What the start kept answers once, whatever happens next.
	s := h.Sessions
	want := s.PopString(ctx, auth.SessionDiscordState)
	until := s.GetInt64(ctx, auth.SessionDiscordUntil)
	s.Remove(ctx, auth.SessionDiscordUntil)
	verifier := s.PopString(ctx, auth.SessionDiscordVerifier)
	next := s.PopString(ctx, auth.SessionDiscordNext)
	inviteID := s.GetInt64(ctx, auth.SessionDiscordInvite)
	s.Remove(ctx, auth.SessionDiscordInvite)

	if in.Error != "" {
		return redirectTo(discordDenied), nil
	}
	if !h.DiscordOAuth.Enabled() || verifier == "" || !stateMatches(want, in.State, until) {
		return redirectTo(discordFailed), nil
	}
	who, err := h.DiscordOAuth.Identify(ctx, discordRedirectURL(ctx), in.Code, verifier)
	if err != nil {
		h.Logger.Warn("discord sign-in failed", "error", err)
		return redirectTo(discordFailed), nil
	}
	h.Logger.Debug("discord sign-in", "discord_id", who.ID, "discord_username", who.Username)
	if inviteID != 0 {
		return redirectTo(h.finishDiscordInvite(ctx, inviteID, who)), nil
	}
	return redirectTo(h.signInWithDiscord(ctx, who, next)), nil
}

// signInWithDiscord signs in the account linked to who and answers where to land.
func (h handlers) signInWithDiscord(ctx context.Context, who discord.Identity, next string) string {
	id, err := h.read().GetDiscordLinkByDiscordID(ctx, who.ID)
	if errors.Is(err, sql.ErrNoRows) {
		h.Logger.Info("discord sign-in with no account linked")
		return discordNotLinked
	}
	if err != nil {
		h.Logger.Error("read a Discord link", "error", err)
		return discordFailed
	}
	// The names can change on Discord; the Users page shows the latest.
	if err := h.write().RefreshDiscordNames(ctx, db.RefreshDiscordNamesParams{
		Username: who.Username, DisplayName: who.DisplayName, DiscordID: who.ID,
	}); err != nil {
		h.Logger.Error("note a Discord account's names", "error", err, "user_id", id)
	}
	u, err := h.user(ctx, id)
	if err != nil {
		return discordFailed
	}
	if !u.IsActive {
		return discordInactive
	}
	if err := h.startSession(ctx, u, requestOf(ctx).IP); err != nil {
		h.Logger.Error("sign in with Discord", "error", err, "user_id", u.ID)
		return discordFailed
	}
	h.Logger.Info("signed in with discord", "user_id", u.ID)
	if sitePath(next) {
		return next
	}
	return adminArea
}

// finishDiscordInvite carries out the admin's link the sign-in began from, for who, and
// answers where to land: signed in to the account it made or linked, when that account
// is active.
func (h handlers) finishDiscordInvite(ctx context.Context, inviteID int64, who discord.Identity) string {
	invite, err := h.read().GetDiscordInviteByID(ctx, inviteID)
	if errors.Is(err, sql.ErrNoRows) {
		return discordLinkExpired
	}
	if err != nil {
		h.Logger.Error("read a Discord link", "error", err)
		return discordFailed
	}
	// Detached: an account made, or a link used, must not stop half-way.
	detached := context.WithoutCancel(ctx)
	var id int64
	switch {
	case invite.Kind == inviteSignUp:
		id, err = h.signUpWithDiscord(detached, invite.ID, who)
	case invite.Kind == inviteLink && invite.UserID.Valid:
		id, err = invite.UserID.Int64, h.linkDiscord(detached, invite.ID, invite.UserID.Int64, who)
	default:
		err = errLinkUsed
	}
	switch {
	case errors.Is(err, errLinkUsed):
		return discordLinkExpired
	case errors.Is(err, errDiscordTaken):
		h.Logger.Info("discord link refused: the Discord account is linked to another account")
		return discordTaken
	case err != nil:
		h.Logger.Error("use a Discord link", "error", err, "kind", invite.Kind)
		return discordFailed
	}
	ResourceChanged(h.Live, "users")
	if invite.Kind == inviteSignUp {
		h.Logger.Info("account made with a discord sign-up link", "user_id", id)
	} else {
		h.Logger.Info("discord account linked", "user_id", id)
	}
	u, err := h.user(detached, id)
	if err != nil {
		return discordFailed
	}
	if !u.IsActive {
		return discordInactive
	}
	if err := h.startSession(ctx, u, requestOf(ctx).IP); err != nil {
		h.Logger.Error("sign in with Discord", "error", err, "user_id", u.ID)
		return discordFailed
	}
	return adminArea
}

// signUpWithDiscord uses the sign-up link inviteID: it makes an active account with no
// permissions, named after who's Discord username, links who to it and answers its id.
// A Discord account linked already is refused, and the link is left unused.
func (h handlers) signUpWithDiscord(ctx context.Context, inviteID int64, who discord.Identity) (int64, error) {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := q.GetDiscordLinkByDiscordID(ctx, who.ID); err == nil {
		return 0, errDiscordTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if n, err := q.UseDiscordInvite(ctx, inviteID); err != nil {
		return 0, err
	} else if n == 0 {
		return 0, errLinkUsed
	}
	id, err := createDiscordUser(ctx, q, who.Username)
	if err != nil {
		return 0, err
	}
	if err := q.LinkDiscord(ctx, db.LinkDiscordParams{
		UserID: id, DiscordID: who.ID, Username: who.Username, DisplayName: who.DisplayName,
	}); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// linkDiscord uses the link inviteID to link who to account id, replacing a Discord
// account it was linked to before. A Discord account linked to another account is
// refused, and the link is left unused.
func (h handlers) linkDiscord(ctx context.Context, inviteID, id int64, who discord.Identity) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if owner, err := q.GetDiscordLinkByDiscordID(ctx, who.ID); err == nil && owner != id {
		return errDiscordTaken
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if n, err := q.UseDiscordInvite(ctx, inviteID); err != nil {
		return err
	} else if n == 0 {
		return errLinkUsed
	}
	err = q.LinkDiscord(ctx, db.LinkDiscordParams{
		UserID: id, DiscordID: who.ID, Username: who.Username, DisplayName: who.DisplayName,
	})
	if uniqueViolation(err) {
		return errDiscordTaken
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// maxNameTries bounds the names tried for an account made with Discord.
const maxNameTries = 50

// createDiscordUser makes the account for a Discord sign-up, named after its Discord
// username, or that name with -2, -3 and so on when it is taken or reserved, and
// answers its id.
func createDiscordUser(ctx context.Context, q *db.Queries, discordName string) (int64, error) {
	base := cutName(strings.TrimSpace(discordName), auth.MaxUsernameLength)
	if base == "" {
		base = "discord"
	}
	for i := 1; i <= maxNameTries; i++ {
		name := base
		if i > 1 {
			suffix := "-" + strconv.Itoa(i)
			name = cutName(base, auth.MaxUsernameLength-len(suffix)) + suffix
		}
		if strings.EqualFold(name, auth.ReservedUsername) {
			continue
		}
		id, err := q.CreateDiscordUser(ctx, name)
		if uniqueViolation(err) {
			continue
		}
		return id, err
	}
	return 0, errors.New("no free username for the Discord account")
}

func (h handlers) createDiscordInvite(ctx context.Context, in *createDiscordInviteInput) (*discordInviteOutput, error) {
	if !h.DiscordOAuth.Enabled() {
		return nil, huma.Error503ServiceUnavailable("Discord sign-in isn't set up on this server.")
	}
	me := Who(ctx)
	kind, lifetime := inviteSignUp, signUpLinkLifetime
	var forID sql.NullInt64
	if in.Body.UserID != nil {
		target, err := h.user(ctx, *in.Body.UserID)
		if err != nil {
			return nil, err
		}
		if target.Username == auth.ReservedUsername && me.ID != target.ID {
			return nil, huma.Error403Forbidden("Only the admin account can make a link to itself.")
		}
		kind, lifetime = inviteLink, linkLinkLifetime
		forID = sql.NullInt64{Int64: target.ID, Valid: true}
	}
	token, hash, err := auth.NewLinkToken()
	if err != nil {
		return nil, h.fail("make a Discord link", err)
	}
	expires := time.Now().UTC().Add(lifetime).Format(time.RFC3339)
	if err := h.keepDiscordInvite(ctx, hash, kind, forID, expires); err != nil {
		return nil, h.fail("keep a Discord link", err)
	}
	if forID.Valid {
		h.Logger.Info("discord link made", "kind", kind, "by", me.ID, "user_id", forID.Int64)
	} else {
		h.Logger.Info("discord link made", "kind", kind, "by", me.ID)
	}
	return &discordInviteOutput{Body: DiscordInvite{
		URL:       requestOf(ctx).Site + Prefix + "/auth/discord/invite/" + token,
		ExpiresAt: expires,
	}}, nil
}

// keepDiscordInvite stores a link, clearing the expired ones and, for an account's
// own link, the one it replaces.
func (h handlers) keepDiscordInvite(ctx context.Context, hash, kind string, forID sql.NullInt64, expires string) error {
	tx, q, err := h.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := q.DeleteExpiredDiscordInvites(ctx); err != nil {
		return err
	}
	if forID.Valid {
		if err := q.DeleteDiscordInvitesFor(ctx, forID); err != nil {
			return err
		}
	}
	if err := q.CreateDiscordInvite(ctx, db.CreateDiscordInviteParams{
		TokenHash: hash, Kind: kind, UserID: forID, ExpiresAt: expires,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h handlers) unlinkDiscord(ctx context.Context, in *userIDInput) (*struct{}, error) {
	target, err := h.user(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if target.Username == auth.ReservedUsername && Who(ctx).ID != target.ID {
		return nil, huma.Error403Forbidden("Only the admin account can unlink its own Discord account.")
	}
	tx, q, err := h.begin(ctx)
	if err != nil {
		return nil, h.fail("unlink a Discord account", err)
	}
	defer func() { _ = tx.Rollback() }()
	n, err := q.UnlinkDiscord(ctx, target.ID)
	if err != nil {
		return nil, h.fail("unlink a Discord account", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("This account has no Discord account linked.")
	}
	if !target.HasPassword {
		if last, err := lastWayIn(ctx, q, target.ID); err != nil {
			return nil, h.fail("count an account's ways to sign in", err)
		} else if last {
			return nil, huma.Error409Conflict("Discord is this account's only way to sign in. Set a password for it first.")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, h.fail("unlink a Discord account", err)
	}
	h.Logger.Info("discord account unlinked", "user_id", target.ID, "by", Who(ctx).ID)
	return nil, nil
}

// discordLinks is every account's Discord account, by account id.
func (h handlers) discordLinks(ctx context.Context) (map[int64]*DiscordAccount, error) {
	rows, err := h.read().ListDiscordLinks(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*DiscordAccount, len(rows))
	for _, r := range rows {
		out[r.UserID] = discordAccountOf(r)
	}
	return out, nil
}

// discordLink is account id's Discord account, or nil when it has none.
func (h handlers) discordLink(ctx context.Context, id int64) (*DiscordAccount, error) {
	row, err := h.read().GetDiscordLink(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return discordAccountOf(row), nil
}

func discordAccountOf(r *db.UserDiscord) *DiscordAccount {
	return &DiscordAccount{Username: r.Username, DisplayName: r.DisplayName, LinkedAt: r.LinkedAt}
}

// discordRedirectURL is the callback's full address on the site the request came to,
// which Discord must list exactly.
func discordRedirectURL(ctx context.Context) string {
	return requestOf(ctx).Site + Prefix + discordCallback
}

func redirectTo(location string) *redirectOutput {
	return &redirectOutput{Status: http.StatusFound, Location: location}
}

// stateMatches reports whether got is the state the session kept, until it expires.
// The comparison takes as long however much of got matches.
func stateMatches(want, got string, until int64) bool {
	return want != "" && time.Now().Unix() <= until && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// sitePath reports whether p is a path on this site, so a crafted start can't send a
// signed-in browser to another host.
func sitePath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return false
	}
	// A control character would survive into the Location header, where a browser
	// drops it and reads what is left as another host; a backslash reads as a slash.
	return !strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f || r == '\\' })
}

// cutRunes is s cut to at most n bytes on a character boundary.
func cutName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
