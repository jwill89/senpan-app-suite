package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/affiliates"
	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/permissions"
)

// Affiliates: partner establishments listed under Senpan Tea House. Each can be
// posted to Discord as an embed through one shared webhook.

var affiliateTags = []string{"Affiliates"}

// affiliateWebhookKey is the setting that holds the webhook every affiliate posts
// to. It isn't among the public settings: only the Affiliates page reads it.
const affiliateWebhookKey = "affiliate_webhook_url"

const noAffiliate = "There is no such affiliate."

var errNoAffiliate = errors.New("no such affiliate")

// AffiliateHour is one opening-hours row.
type AffiliateHour struct {
	Label string `json:"label" maxLength:"100" doc:"What the row covers, such as Mon-Fri; empty for none."`
	Start string `json:"start" maxLength:"10" doc:"When it opens, wall-clock HH:MM in the affiliate's timezone. A row with no start is dropped."`
	End   string `json:"end" maxLength:"10" doc:"When it closes, wall-clock HH:MM; empty for open-ended."`
}

// Affiliate is a partner establishment.
type Affiliate struct {
	ID          int64           `json:"id" doc:"The affiliate's id."`
	Name        string          `json:"name" doc:"The establishment's name."`
	Subtitle    string          `json:"subtitle" doc:"A second line under the name, in any script."`
	Owners      []string        `json:"owners" nullable:"false" doc:"The owners' names."`
	Location    string          `json:"location" doc:"Where it is, as free text."`
	Timezone    string          `json:"timezone" doc:"The IANA timezone its opening hours are in."`
	Hours       []AffiliateHour `json:"hours" nullable:"false" doc:"When it is open."`
	Details     string          `json:"details" doc:"About it, in Markdown."`
	Logo        string          `json:"logo" doc:"The logo: a root-relative path in the image library."`
	Screenshot  string          `json:"screenshot" doc:"A picture of the establishment: a root-relative path in the image library."`
	EmbedColor  string          `json:"embedColor" doc:"The accent of its Discord embed, #rrggbb; empty for the brand color."`
	DiscordLink string          `json:"discordLink" doc:"A Discord invite or link, always http(s)."`
	CarrdLink   string          `json:"carrdLink" doc:"A Carrd or other site link, always http(s)."`
	SortOrder   int64           `json:"sortOrder" doc:"Its place in the manual order, lowest first."`
	CreatedAt   *string         `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// AffiliateInput is an affiliate as a save sends it: all of it, since a save
// replaces it.
type AffiliateInput struct {
	Name        string          `json:"name" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The establishment's name."`
	Subtitle    string          `json:"subtitle" maxLength:"200" doc:"A second line under the name, in any script; empty for none."`
	Owners      []string        `json:"owners" maxItems:"50" nullable:"false" doc:"The owners' names; blank ones are dropped."`
	Location    string          `json:"location" maxLength:"500" doc:"Where it is, as free text."`
	Timezone    string          `json:"timezone" maxLength:"100" doc:"The IANA timezone its opening hours are in; empty for UTC."`
	Hours       []AffiliateHour `json:"hours" maxItems:"50" nullable:"false" doc:"When it is open."`
	Details     string          `json:"details" maxLength:"20000" doc:"About it, in Markdown."`
	Logo        string          `json:"logo" maxLength:"500" doc:"The logo: a root-relative path in the image library; empty for none."`
	Screenshot  string          `json:"screenshot" maxLength:"500" doc:"A picture of the establishment: a root-relative path in the image library; empty for none."`
	EmbedColor  string          `json:"embedColor" maxLength:"7" doc:"The accent of its Discord embed, #rrggbb; empty for the brand color."`
	DiscordLink string          `json:"discordLink" maxLength:"500" doc:"A Discord invite or link; one with no http(s) scheme is saved with https:// before it."`
	CarrdLink   string          `json:"carrdLink" maxLength:"500" doc:"A Carrd or other site link; one with no http(s) scheme is saved with https:// before it."`
}

// AffiliateList is every affiliate.
type AffiliateList struct {
	Affiliates []Affiliate `json:"affiliates" nullable:"false" doc:"Every affiliate, in their manual order and then by name."`
}

// AffiliateOrder is the affiliates in their new order.
type AffiliateOrder struct {
	OrderedIDs []int64 `json:"orderedIds" minItems:"1" maxItems:"1000" uniqueItems:"true" nullable:"false" doc:"The affiliates' ids, top first."`
}

type affiliateIDInput struct {
	ID int64 `path:"id" doc:"The affiliate's id."`
}

type affiliateCreateInput struct {
	Body AffiliateInput
}

type affiliateSaveInput struct {
	ID   int64 `path:"id" doc:"The affiliate's id."`
	Body AffiliateInput
}

type affiliateOrderInput struct {
	Body AffiliateOrder
}

type affiliateWebhookInput struct {
	Body DiscordWebhook
}

type affiliateOutput struct{ Body Affiliate }

type affiliateCreatedOutput struct {
	Status int
	Body   Affiliate
}

type affiliateListOutput struct{ Body AffiliateList }

type affiliateWebhookOutput struct{ Body DiscordWebhook }

func (h handlers) registerAffiliates(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.TeahouseAffiliates)}
	// The festival map and stamp rally editors pick an affiliate to name who runs a
	// pitch, so they read the list too.
	readers := huma.Middlewares{h.permitted(api, permissions.TeahouseAffiliates, permissions.FestivalMap, permissions.FestivalStampRally)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}

	huma.Register(api, huma.Operation{
		OperationID: "list-affiliates", Method: http.MethodGet, Path: "/affiliates",
		Summary: "Every affiliate", Tags: affiliateTags, Security: secured, Middlewares: readers, Errors: guarded,
	}, h.listAffiliates)
	huma.Register(api, huma.Operation{
		OperationID: "create-affiliate", Method: http.MethodPost, Path: "/affiliates",
		Summary: "Make an affiliate", Tags: affiliateTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: page, Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createAffiliate)
	huma.Register(api, huma.Operation{
		OperationID: "update-affiliate", Method: http.MethodPut, Path: "/affiliates/{id}",
		Summary: "Replace an affiliate", Tags: affiliateTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updateAffiliate)
	huma.Register(api, huma.Operation{
		OperationID: "delete-affiliate", Method: http.MethodDelete, Path: "/affiliates/{id}",
		Summary: "Delete an affiliate", Tags: affiliateTags, DefaultStatus: http.StatusNoContent,
		Description: "A pitch occupant that named this affiliate and nothing else takes its name as its title.",
		Security:    secured, Middlewares: page, Errors: append(guarded, http.StatusNotFound),
	}, h.deleteAffiliate)
	huma.Register(api, huma.Operation{
		OperationID: "reorder-affiliates", Method: http.MethodPost, Path: "/affiliates/reorder",
		Summary: "Put the affiliates in a new order", Tags: affiliateTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.reorderAffiliates)
	huma.Register(api, huma.Operation{
		OperationID: "get-affiliate-webhook", Method: http.MethodGet, Path: "/affiliates/webhook",
		Summary: "The Discord webhook affiliates are posted to", Tags: affiliateTags,
		Security: secured, Middlewares: page, Errors: guarded,
	}, h.getAffiliateWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "set-affiliate-webhook", Method: http.MethodPut, Path: "/affiliates/webhook",
		Summary: "Set the Discord webhook affiliates are posted to", Tags: affiliateTags,
		Security: secured, Middlewares: page, Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.setAffiliateWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "post-affiliate", Method: http.MethodPost, Path: "/affiliates/{id}/post",
		Summary: "Post an affiliate to Discord", Tags: affiliateTags, Security: secured, Middlewares: page,
		Errors: append(append(guarded, http.StatusNotFound), discordErrors...),
	}, h.postAffiliate)
}

// decodeList reads a JSON array column; one that doesn't read is an empty list.
func decodeList[T any](stored string) []T {
	list := []T{}
	if err := json.Unmarshal([]byte(stored), &list); err != nil || list == nil {
		return []T{}
	}
	return list
}

func toAffiliate(a *db.Affiliate) Affiliate {
	return Affiliate{
		ID: a.ID, Name: a.Name, Subtitle: a.Subtitle, Owners: decodeList[string](a.Owners),
		Location: a.Location, Timezone: a.Timezone, Hours: decodeList[AffiliateHour](a.Hours),
		Details: a.Details, Logo: a.Logo, Screenshot: a.Screenshot, EmbedColor: a.EmbedColor,
		DiscordLink: a.DiscordLink, CarrdLink: a.CarrdLink, SortOrder: a.SortOrder, CreatedAt: a.CreatedAt,
	}
}

func toHours(in []AffiliateHour) []affiliates.Hour {
	out := make([]affiliates.Hour, 0, len(in))
	for _, h := range in {
		out = append(out, affiliates.Hour{Label: h.Label, Start: h.Start, End: h.End})
	}
	return out
}

func (a Affiliate) profile() affiliates.Profile {
	return affiliates.Profile{
		Name: a.Name, Location: a.Location, Timezone: a.Timezone, Hours: toHours(a.Hours), Details: a.Details,
		Logo: a.Logo, Screenshot: a.Screenshot, EmbedColor: a.EmbedColor,
		DiscordLink: a.DiscordLink, CarrdLink: a.CarrdLink,
	}
}

// sanitized is the input as it is stored: trimmed, blank owners and start-less hours
// dropped, and the two links given a safe scheme (they are hrefs on the public
// festival map).
func (in AffiliateInput) sanitized() AffiliateInput {
	hours := make([]AffiliateHour, 0, len(in.Hours))
	for _, h := range affiliates.SanitizeHours(toHours(in.Hours)) {
		hours = append(hours, AffiliateHour{Label: h.Label, Start: h.Start, End: h.End})
	}
	return AffiliateInput{
		Name:        strings.TrimSpace(in.Name),
		Subtitle:    strings.TrimSpace(in.Subtitle),
		Owners:      affiliates.SanitizeOwners(in.Owners),
		Location:    strings.TrimSpace(in.Location),
		Timezone:    strings.TrimSpace(in.Timezone),
		Hours:       hours,
		Details:     in.Details,
		Logo:        strings.TrimSpace(in.Logo),
		Screenshot:  strings.TrimSpace(in.Screenshot),
		EmbedColor:  strings.TrimSpace(in.EmbedColor),
		DiscordLink: affiliates.NormalizeExternalURL(in.DiscordLink),
		CarrdLink:   affiliates.NormalizeExternalURL(in.CarrdLink),
	}
}

// lists is the owners and hours as the JSON the columns hold.
func (in AffiliateInput) lists() (owners, hours string, err error) {
	o, err := json.Marshal(in.Owners)
	if err != nil {
		return "", "", err
	}
	hs, err := json.Marshal(in.Hours)
	if err != nil {
		return "", "", err
	}
	return string(o), string(hs), nil
}

func (h handlers) listAffiliates(ctx context.Context, _ *struct{}) (*affiliateListOutput, error) {
	rows, err := h.read().ListAffiliates(ctx)
	if err != nil {
		return nil, h.fail("list affiliates", err)
	}
	out := AffiliateList{Affiliates: make([]Affiliate, 0, len(rows))}
	for _, r := range rows {
		out.Affiliates = append(out.Affiliates, toAffiliate(r))
	}
	return &affiliateListOutput{Body: out}, nil
}

func (h handlers) createAffiliate(ctx context.Context, in *affiliateCreateInput) (*affiliateCreatedOutput, error) {
	body := in.Body.sanitized()
	owners, hours, err := body.lists()
	if err != nil {
		return nil, h.fail("encode an affiliate", err)
	}
	r, err := h.write().CreateAffiliate(ctx, db.CreateAffiliateParams{
		Name: body.Name, Subtitle: body.Subtitle, Owners: owners, Location: body.Location, Timezone: body.Timezone,
		Hours: hours, Details: body.Details, Logo: body.Logo, Screenshot: body.Screenshot,
		EmbedColor: body.EmbedColor, DiscordLink: body.DiscordLink, CarrdLink: body.CarrdLink,
	})
	if err != nil {
		return nil, h.fail("create an affiliate", err)
	}
	return &affiliateCreatedOutput{Status: http.StatusCreated, Body: toAffiliate(r)}, nil
}

func (h handlers) updateAffiliate(ctx context.Context, in *affiliateSaveInput) (*affiliateOutput, error) {
	body := in.Body.sanitized()
	owners, hours, err := body.lists()
	if err != nil {
		return nil, h.fail("encode an affiliate", err)
	}
	r, err := h.write().UpdateAffiliate(ctx, db.UpdateAffiliateParams{
		Name: body.Name, Subtitle: body.Subtitle, Owners: owners, Location: body.Location, Timezone: body.Timezone,
		Hours: hours, Details: body.Details, Logo: body.Logo, Screenshot: body.Screenshot,
		EmbedColor: body.EmbedColor, DiscordLink: body.DiscordLink, CarrdLink: body.CarrdLink, ID: in.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noAffiliate)
	}
	if err != nil {
		return nil, h.fail("update an affiliate", err)
	}
	return &affiliateOutput{Body: toAffiliate(r)}, nil
}

func (h handlers) deleteAffiliate(ctx context.Context, in *affiliateIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	var deleted int64
	err := h.transact(ctx, func(q *db.Queries) error {
		if err := q.NameOccupantsAfterAffiliate(ctx, in.ID); err != nil {
			return err
		}
		n, err := q.DeleteAffiliate(ctx, in.ID)
		deleted = n
		return err
	})
	if err != nil {
		return nil, h.fail("delete an affiliate", err)
	}
	if deleted == 0 {
		return nil, huma.Error404NotFound(noAffiliate)
	}
	return nil, nil
}

func (h handlers) reorderAffiliates(ctx context.Context, in *affiliateOrderInput) (*affiliateListOutput, error) {
	order := in.Body.OrderedIDs
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		for i, id := range order {
			n, err := q.SetAffiliateOrder(ctx, db.SetAffiliateOrderParams{SortOrder: int64(i), ID: id})
			if err != nil {
				return err
			}
			if n == 0 {
				return missingError{err: errNoAffiliate, index: i}
			}
		}
		return nil
	})
	var miss missingError
	switch {
	case errors.As(err, &miss):
		return nil, unknown("body.orderedIds["+strconv.Itoa(miss.index)+"]", order[miss.index], "affiliate")
	case err != nil:
		return nil, h.fail("reorder affiliates", err)
	}
	return h.listAffiliates(ctx, nil)
}

// affiliateWebhook is the saved webhook, "" for none.
func (h handlers) affiliateWebhook(ctx context.Context) (string, error) {
	url, err := h.read().GetSetting(ctx, affiliateWebhookKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(url), err
}

func (h handlers) getAffiliateWebhook(ctx context.Context, _ *struct{}) (*affiliateWebhookOutput, error) {
	url, err := h.affiliateWebhook(ctx)
	if err != nil {
		return nil, h.fail("read the affiliate webhook", err)
	}
	return &affiliateWebhookOutput{Body: DiscordWebhook{WebhookURL: url}}, nil
}

func (h handlers) setAffiliateWebhook(ctx context.Context, in *affiliateWebhookInput) (*affiliateWebhookOutput, error) {
	body := DiscordWebhook{WebhookURL: strings.TrimSpace(in.Body.WebhookURL)}
	if err := body.check(); err != nil {
		return nil, err
	}
	if err := h.write().SetSetting(ctx, db.SetSettingParams{Key: affiliateWebhookKey, Value: body.WebhookURL}); err != nil {
		return nil, h.fail("save the affiliate webhook", err)
	}
	return &affiliateWebhookOutput{Body: body}, nil
}

func (h handlers) postAffiliate(ctx context.Context, in *affiliateIDInput) (*affiliateOutput, error) {
	row, err := h.read().GetAffiliateByID(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noAffiliate)
	}
	if err != nil {
		return nil, h.fail("read an affiliate", err)
	}
	webhook, err := h.affiliateWebhook(ctx)
	if err != nil {
		return nil, h.fail("read the affiliate webhook", err)
	}
	if webhook == "" {
		return nil, noWebhook("the affiliates")
	}
	a := toAffiliate(row)
	embed := affiliates.Embed(a.profile(), requestOf(ctx).Site, time.Now())
	if err := h.Discord.PostEmbed(ctx, discord.Target{Kind: "affiliate", Name: a.Name}, webhook, embed); err != nil {
		return nil, h.discordProblem(err, "post affiliate "+strconv.FormatInt(a.ID, 10))
	}
	return &affiliateOutput{Body: a}, nil
}
