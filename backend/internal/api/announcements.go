package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/announcements"
	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/permissions"
	"app-suite/internal/schedule"
)

// Announcements: Discord embeds that staff write, post by hand or schedule, to the
// webhook of the announcement's type, optionally tagging a role. Types and roles are
// the lists the announcement picks from.

var announcementTags = []string{"Announcements"}

const (
	noSuchAnnouncement = "There is no such announcement."
	noSuchType         = "There is no such announcement type."
	noSuchRole         = "There is no such announcement role."
)

// AnnouncementType is a named Discord destination.
type AnnouncementType struct {
	ID         int64   `json:"id" doc:"The type's id."`
	Name       string  `json:"name" doc:"What staff call the destination, such as the channel's name."`
	WebhookURL string  `json:"webhookUrl" doc:"The Discord webhook of the channel its announcements post to; empty for none. A webhook grants posting access to a channel, so only staff with the page see it."`
	CreatedAt  *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// AnnouncementTypeInput is a type as a save sends it.
type AnnouncementTypeInput struct {
	Name       string `json:"name" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"What staff call the destination."`
	WebhookURL string `json:"webhookUrl" maxLength:"500" doc:"The Discord webhook of the channel: https, on Discord, under /api/webhooks/. Empty for none."`
}

// AnnouncementTypeList is every type.
type AnnouncementTypeList struct {
	Types []AnnouncementType `json:"types" nullable:"false" doc:"Every type, newest first."`
}

// AnnouncementRole is a Discord role an announcement can tag.
type AnnouncementRole struct {
	ID        int64   `json:"id" doc:"The role's id here, which an announcement's mention names as role:<id>."`
	Name      string  `json:"name" doc:"What staff call the role."`
	RoleID    string  `json:"roleId" doc:"The role's Discord id, digits only; kept as text since it is too big for a JavaScript number."`
	CreatedAt *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// AnnouncementRoleInput is a role as a save sends it.
type AnnouncementRoleInput struct {
	Name   string `json:"name" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"What staff call the role."`
	RoleID string `json:"roleId" minLength:"1" maxLength:"30" doc:"The role's Discord id, digits only."`
}

// AnnouncementRoleList is every role.
type AnnouncementRoleList struct {
	Roles []AnnouncementRole `json:"roles" nullable:"false" doc:"Every role, newest first."`
}

// AnnouncementButton is a link button under the embed.
type AnnouncementButton struct {
	Label string `json:"label" maxLength:"80" doc:"The button's text."`
	Emoji string `json:"emoji" maxLength:"100" doc:"A Unicode emoji, or a custom one as <:name:id> or <a:name:id>; empty for none."`
	URL   string `json:"url" maxLength:"2000" doc:"Where it goes: an http or https address."`
}

// Announcement is a message staff post to Discord.
type Announcement struct {
	ID                  int64                `json:"id" doc:"The announcement's id."`
	TypeID              int64                `json:"typeId" doc:"The type it posts as, which names the webhook."`
	TypeName            string               `json:"typeName" doc:"The type's name; empty if the type is gone."`
	Title               string               `json:"title" doc:"The embed's title."`
	Details             string               `json:"details" doc:"The body, in Markdown."`
	Image               string               `json:"image" doc:"A full-width picture at the bottom of the embed; empty for none."`
	Thumbnail           string               `json:"thumbnail" doc:"A small picture at the embed's top right; empty for none."`
	Color               string               `json:"color" doc:"The embed's accent, #rrggbb; empty for the brand color."`
	Location            string               `json:"location" doc:"Where it happens, as free text; empty for none."`
	StartLocal          string               `json:"startLocal" doc:"When the event starts, wall-clock YYYY-MM-DDTHH:MM in timezone; empty for none."`
	EndLocal            string               `json:"endLocal" doc:"When the event ends, wall-clock in timezone; empty for none."`
	StartAt             *string              `json:"startAt" nullable:"true" doc:"The event's start as an instant, RFC 3339 in UTC; null for none."`
	EndAt               *string              `json:"endAt" nullable:"true" doc:"The event's end as an instant, RFC 3339 in UTC; null for none."`
	StartFormat         string               `json:"startFormat" doc:"The Discord timestamp style of the start: t, T, d, D, f, F or R."`
	EndFormat           string               `json:"endFormat" doc:"The Discord timestamp style of the end: t, T, d, D, f, F or R."`
	DynamicDates        bool                 `json:"dynamicDates" doc:"Whether each post moves the event onto the day it is posted, keeping the times of day, so a recurring day-of post always shows its own occurrence."`
	ScheduleKind        string               `json:"scheduleKind" doc:"How it posts on its own: once, daily, weekly or monthly; empty for by hand only."`
	Timezone            string               `json:"timezone" doc:"The IANA timezone every wall-clock time is in."`
	OnceLocal           string               `json:"onceLocal" doc:"For a once schedule, when it posts, wall-clock in timezone."`
	ScheduleMinutes     int64                `json:"scheduleMinutes" doc:"For a recurring schedule, the time of day it posts, in minutes after midnight in timezone."`
	ScheduleWeekdays    string               `json:"scheduleWeekdays" doc:"For a weekly or monthly schedule, the weekdays as comma-separated numbers, 0 for Sunday to 6 for Saturday. A monthly schedule uses the first."`
	ScheduleWeekOfMonth int64                `json:"scheduleWeekOfMonth" doc:"For a monthly schedule, which weekday of the month: 1 to 5, or -1 for the last."`
	NextPostAt          *string              `json:"nextPostAt" nullable:"true" doc:"When it posts next, RFC 3339 in UTC; null when nothing is scheduled."`
	SkipCount           int64                `json:"skipCount" doc:"How many upcoming occurrences to pass over before posting again; 0 posts as scheduled. A save never changes it."`
	Active              bool                 `json:"active" doc:"Whether its schedule is live."`
	LastPostedAt        *string              `json:"lastPostedAt" nullable:"true" doc:"When it was last posted, by the schedule or by hand, RFC 3339 in UTC; null if never."`
	Buttons             []AnnouncementButton `json:"buttons" nullable:"false" doc:"The link buttons under the embed."`
	Mention             string               `json:"mention" doc:"Who the post tags in its text, above the embed: empty for nobody, everyone, or role:<id> for a managed role."`
	CreatedAt           *string              `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// AnnouncementInput is an announcement as a save sends it: all of its editable
// fields. Whether it is live and when it posts next follow from the schedule.
type AnnouncementInput struct {
	TypeID              int64                `json:"typeId" minimum:"1" doc:"The type it posts as."`
	Title               string               `json:"title" minLength:"1" maxLength:"256" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The embed's title."`
	Details             string               `json:"details" maxLength:"20000" doc:"The body, in Markdown. Required."`
	Image               string               `json:"image" maxLength:"2000" doc:"A full-width picture at the bottom of the embed; empty for none."`
	Thumbnail           string               `json:"thumbnail" maxLength:"2000" doc:"A small picture at the embed's top right; empty for none."`
	Color               string               `json:"color" maxLength:"7" doc:"The embed's accent, #rrggbb; empty for the brand color."`
	Location            string               `json:"location" maxLength:"500" doc:"Where it happens, as free text; empty for none."`
	StartLocal          string               `json:"startLocal" maxLength:"19" doc:"When the event starts, wall-clock YYYY-MM-DDTHH:MM in timezone; empty for none."`
	EndLocal            string               `json:"endLocal" maxLength:"19" doc:"When the event ends, wall-clock in timezone; empty for none."`
	StartFormat         string               `json:"startFormat" maxLength:"1" doc:"The Discord timestamp style of the start: t, T, d, D, f, F or R; empty for F."`
	EndFormat           string               `json:"endFormat" maxLength:"1" doc:"The Discord timestamp style of the end; empty for t."`
	DynamicDates        bool                 `json:"dynamicDates" doc:"Whether each post moves the event onto the day it is posted, keeping the times of day."`
	ScheduleKind        string               `json:"scheduleKind" maxLength:"10" doc:"How it posts on its own: once, daily, weekly or monthly; empty for by hand only."`
	Timezone            string               `json:"timezone" maxLength:"100" doc:"The IANA timezone every wall-clock time is in. Required when there is any time."`
	OnceLocal           string               `json:"onceLocal" maxLength:"19" doc:"For a once schedule, when it posts, wall-clock in timezone. A date already past arms a new announcement to post on the next sweep; an edit arms one only for a date in the future, so fixing a typo never posts it again."`
	ScheduleMinutes     int64                `json:"scheduleMinutes" minimum:"0" maximum:"1439" doc:"For a recurring schedule, the time of day it posts, in minutes after midnight in timezone."`
	ScheduleWeekdays    string               `json:"scheduleWeekdays" maxLength:"20" doc:"For a weekly or monthly schedule, the weekdays as comma-separated numbers, 0 for Sunday to 6 for Saturday."`
	ScheduleWeekOfMonth int64                `json:"scheduleWeekOfMonth" minimum:"-1" maximum:"5" doc:"For a monthly schedule, which weekday of the month: 1 to 5, or -1 for the last."`
	Buttons             []AnnouncementButton `json:"buttons" maxItems:"25" nullable:"false" doc:"The link buttons under the embed. A button needs a label and an http(s) address, and only the first five that do are kept."`
	Mention             string               `json:"mention" maxLength:"30" doc:"Who the post tags in its text, above the embed: empty for nobody, everyone, or role:<id> for a managed role."`
}

// AnnouncementList is every announcement.
type AnnouncementList struct {
	Announcements []Announcement `json:"announcements" nullable:"false" doc:"Every announcement, in the staff's order."`
}

// AnnouncementOrder is the announcements in their new order.
type AnnouncementOrder struct {
	OrderedIDs []int64 `json:"orderedIds" minItems:"1" maxItems:"1000" uniqueItems:"true" nullable:"false" doc:"The announcements' ids, top first."`
}

// AnnouncementSkip says how many upcoming occurrences to pass over.
type AnnouncementSkip struct {
	Count *int64 `json:"count,omitempty" required:"false" minimum:"0" maximum:"52" doc:"How many upcoming occurrences to skip, one each time the schedule comes due. 0 clears a pending skip; left out, it is 1."`
}

type announcementTypeIDInput struct {
	ID int64 `path:"id" doc:"The type's id."`
}

type announcementTypeCreateInput struct {
	Body AnnouncementTypeInput
}

type announcementTypeSaveInput struct {
	ID   int64 `path:"id" doc:"The type's id."`
	Body AnnouncementTypeInput
}

type announcementRoleIDInput struct {
	ID int64 `path:"id" doc:"The role's id."`
}

type announcementRoleCreateInput struct {
	Body AnnouncementRoleInput
}

type announcementRoleSaveInput struct {
	ID   int64 `path:"id" doc:"The role's id."`
	Body AnnouncementRoleInput
}

type announcementIDInput struct {
	ID int64 `path:"id" doc:"The announcement's id."`
}

type announcementCreateInput struct {
	Body AnnouncementInput
}

type announcementSaveInput struct {
	ID   int64 `path:"id" doc:"The announcement's id."`
	Body AnnouncementInput
}

type announcementOrderInput struct {
	Body AnnouncementOrder
}

type announcementSkipInput struct {
	ID   int64 `path:"id" doc:"The announcement's id."`
	Body AnnouncementSkip
}

type announcementTypeOutput struct{ Body AnnouncementType }

type announcementTypeCreatedOutput struct {
	Status int
	Body   AnnouncementType
}

type announcementTypeListOutput struct{ Body AnnouncementTypeList }

type announcementRoleOutput struct{ Body AnnouncementRole }

type announcementRoleCreatedOutput struct {
	Status int
	Body   AnnouncementRole
}

type announcementRoleListOutput struct{ Body AnnouncementRoleList }

type announcementOutput struct{ Body Announcement }

type announcementCreatedOutput struct {
	Status int
	Body   Announcement
}

type announcementListOutput struct{ Body AnnouncementList }

func (h handlers) registerAnnouncements(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.TeahouseAnnouncements)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	invalid := slices.Concat(guarded, []int{http.StatusUnprocessableEntity})
	missing := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-announcement-types", Method: http.MethodGet, Path: "/announcement-types",
		Summary: "Every announcement type", Tags: announcementTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listAnnouncementTypes)
	huma.Register(api, huma.Operation{
		OperationID: "create-announcement-type", Method: http.MethodPost, Path: "/announcement-types",
		Summary: "Make an announcement type", Tags: announcementTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: page, Errors: invalid,
	}, h.createAnnouncementType)
	huma.Register(api, huma.Operation{
		OperationID: "update-announcement-type", Method: http.MethodPut, Path: "/announcement-types/{id}",
		Summary: "Replace an announcement type", Tags: announcementTags, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusUnprocessableEntity),
	}, h.updateAnnouncementType)
	huma.Register(api, huma.Operation{
		OperationID: "delete-announcement-type", Method: http.MethodDelete, Path: "/announcement-types/{id}",
		Summary: "Delete an announcement type", Description: "A type announcements still use can't be deleted (409).",
		Tags: announcementTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusConflict),
	}, h.deleteAnnouncementType)

	huma.Register(api, huma.Operation{
		OperationID: "list-announcement-roles", Method: http.MethodGet, Path: "/announcement-roles",
		Summary: "Every taggable role", Tags: announcementTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listAnnouncementRoles)
	huma.Register(api, huma.Operation{
		OperationID: "create-announcement-role", Method: http.MethodPost, Path: "/announcement-roles",
		Summary: "Make a taggable role", Tags: announcementTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: page, Errors: invalid,
	}, h.createAnnouncementRole)
	huma.Register(api, huma.Operation{
		OperationID: "update-announcement-role", Method: http.MethodPut, Path: "/announcement-roles/{id}",
		Summary: "Replace a taggable role", Tags: announcementTags, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusUnprocessableEntity),
	}, h.updateAnnouncementRole)
	huma.Register(api, huma.Operation{
		OperationID: "delete-announcement-role", Method: http.MethodDelete, Path: "/announcement-roles/{id}",
		Summary: "Delete a taggable role", Description: "A role announcements still tag can't be deleted (409).",
		Tags: announcementTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusConflict),
	}, h.deleteAnnouncementRole)

	huma.Register(api, huma.Operation{
		OperationID: "list-announcements", Method: http.MethodGet, Path: "/announcements",
		Summary: "Every announcement", Tags: announcementTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listAnnouncements)
	huma.Register(api, huma.Operation{
		OperationID: "create-announcement", Method: http.MethodPost, Path: "/announcements",
		Summary: "Make an announcement", Tags: announcementTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: page, Errors: invalid,
	}, h.createAnnouncement)
	huma.Register(api, huma.Operation{
		OperationID: "update-announcement", Method: http.MethodPut, Path: "/announcements/{id}",
		Summary: "Replace an announcement",
		Description: "How many occurrences are skipped and when it was last posted are kept. A one-time announcement that " +
			"has already gone out is not armed again unless its date moves to the future.",
		Tags: announcementTags, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusUnprocessableEntity),
	}, h.updateAnnouncement)
	huma.Register(api, huma.Operation{
		OperationID: "delete-announcement", Method: http.MethodDelete, Path: "/announcements/{id}",
		Summary: "Delete an announcement", Description: "Its pictures stay in the image library.",
		Tags: announcementTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: missing,
	}, h.deleteAnnouncement)
	huma.Register(api, huma.Operation{
		OperationID: "reorder-announcements", Method: http.MethodPost, Path: "/announcements/reorder",
		Summary: "Put the announcements in order", Description: "Answers every announcement, in their new order.",
		Tags: announcementTags, Security: secured, Middlewares: page, Errors: invalid,
	}, h.reorderAnnouncements)
	huma.Register(api, huma.Operation{
		OperationID: "send-announcement", Method: http.MethodPost, Path: "/announcements/{id}/send",
		Summary:     "Post an announcement to Discord now",
		Description: "Posts to its type's webhook and stamps it as posted, without moving its schedule.",
		Tags:        announcementTags, Security: secured, Middlewares: page,
		Errors: append(missing, discordErrors...),
	}, h.sendAnnouncement)
	huma.Register(api, huma.Operation{
		OperationID: "skip-announcement", Method: http.MethodPost, Path: "/announcements/{id}/skip",
		Summary: "Skip an announcement's upcoming occurrences",
		Description: "Sets how many occurrences to pass over; the schedule uses one each time it comes due. " +
			"Only a scheduled announcement can be skipped (409).",
		Tags: announcementTags, Security: secured, Middlewares: page,
		Errors: append(missing, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.skipAnnouncement)
}

func toAnnouncementType(t *db.AnnouncementType) AnnouncementType {
	return AnnouncementType{ID: t.ID, Name: t.Name, WebhookURL: t.WebhookUrl, CreatedAt: t.CreatedAt}
}

func toAnnouncementRole(r *db.AnnouncementRole) AnnouncementRole {
	return AnnouncementRole{ID: r.ID, Name: r.Name, RoleID: r.RoleID, CreatedAt: r.CreatedAt}
}

func toAnnouncement(a *db.Announcement, typeName string) Announcement {
	return Announcement{
		ID: a.ID, TypeID: a.TypeID, TypeName: typeName, Title: a.Title, Details: a.Details, Image: a.Image,
		Thumbnail: a.Thumbnail, Color: a.Color, Location: a.Location, StartLocal: a.StartLocal, EndLocal: a.EndLocal,
		StartAt: a.StartAt, EndAt: a.EndAt, StartFormat: a.StartFormat, EndFormat: a.EndFormat,
		DynamicDates: a.DynamicDates == 1, ScheduleKind: a.ScheduleKind, Timezone: a.Timezone, OnceLocal: a.OnceLocal,
		ScheduleMinutes: a.ScheduleMinutes, ScheduleWeekdays: a.ScheduleWeekdays, ScheduleWeekOfMonth: a.ScheduleWeekOfMonth,
		NextPostAt: a.NextPostAt, SkipCount: a.SkipCount, Active: a.Active == 1, LastPostedAt: a.LastPostedAt,
		Buttons: decodeList[AnnouncementButton](a.Buttons), Mention: a.Mention, CreatedAt: a.CreatedAt,
	}
}

// nilIfEmpty is s as a nullable column: nil for none.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func announcementButtons(in []AnnouncementButton) []announcements.Button {
	out := make([]announcements.Button, 0, len(in))
	for _, b := range in {
		out = append(out, announcements.Button{Label: b.Label, Emoji: b.Emoji, URL: b.URL})
	}
	return out
}

// announcementOf is a stored announcement as the rules and the message read it.
func announcementOf(a *db.Announcement) announcements.Announcement {
	buttons := decodeList[AnnouncementButton](a.Buttons)
	return announcements.Announcement{
		Title: a.Title, Details: a.Details, Image: a.Image, Thumbnail: a.Thumbnail, Color: a.Color,
		Location: a.Location, StartLocal: a.StartLocal, EndLocal: a.EndLocal,
		StartAt: deref(a.StartAt), EndAt: deref(a.EndAt), StartFormat: a.StartFormat, EndFormat: a.EndFormat,
		DynamicDates: a.DynamicDates == 1, Timezone: a.Timezone, Buttons: announcementButtons(buttons), Mention: a.Mention,
	}
}

// AnnouncementRule is the recurrence a stored announcement's schedule describes.
func AnnouncementRule(a *db.Announcement) schedule.Rule {
	return schedule.Rule{
		Kind: a.ScheduleKind, Timezone: a.Timezone, Minutes: int(a.ScheduleMinutes),
		Weekdays: a.ScheduleWeekdays, WeekOfMonth: int(a.ScheduleWeekOfMonth),
	}
}

// AnnouncementPayload is the Discord message for a stored announcement, as of now.
// The role it tags is looked up through q; one that is gone posts without a tag.
func AnnouncementPayload(ctx context.Context, q *db.Queries, a *db.Announcement, now time.Time) discord.Payload {
	roleID := ""
	if _, role, _ := announcements.ParseMention(a.Mention); role > 0 {
		if r, err := q.GetAnnouncementRole(ctx, role); err == nil {
			roleID = r.RoleID
		}
	}
	return announcements.Message(announcementOf(a), roleID, now)
}

func (h handlers) typeName(ctx context.Context, id int64) string {
	t, err := h.read().GetAnnouncementType(ctx, id)
	if err != nil {
		return ""
	}
	return t.Name
}

// -- Types ---------------------------------------------------------------------

func (h handlers) listAnnouncementTypes(ctx context.Context, _ *struct{}) (*announcementTypeListOutput, error) {
	rows, err := h.read().ListAnnouncementTypes(ctx)
	if err != nil {
		return nil, h.fail("list announcement types", err)
	}
	out := AnnouncementTypeList{Types: make([]AnnouncementType, 0, len(rows))}
	for _, r := range rows {
		out.Types = append(out.Types, toAnnouncementType(r))
	}
	return &announcementTypeListOutput{Body: out}, nil
}

func (in AnnouncementTypeInput) cleaned() (AnnouncementTypeInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.WebhookURL = strings.TrimSpace(in.WebhookURL)
	return in, DiscordWebhook{WebhookURL: in.WebhookURL}.check()
}

func (h handlers) createAnnouncementType(ctx context.Context, in *announcementTypeCreateInput) (*announcementTypeCreatedOutput, error) {
	body, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	made, err := h.write().CreateAnnouncementType(ctx, db.CreateAnnouncementTypeParams{Name: body.Name, WebhookUrl: body.WebhookURL})
	if err != nil {
		return nil, h.fail("create an announcement type", err)
	}
	return &announcementTypeCreatedOutput{Status: http.StatusCreated, Body: toAnnouncementType(made)}, nil
}

func (h handlers) updateAnnouncementType(ctx context.Context, in *announcementTypeSaveInput) (*announcementTypeOutput, error) {
	body, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	saved, err := h.write().UpdateAnnouncementType(ctx, db.UpdateAnnouncementTypeParams{Name: body.Name, WebhookUrl: body.WebhookURL, ID: in.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchType)
	}
	if err != nil {
		return nil, h.fail("update an announcement type", err)
	}
	return &announcementTypeOutput{Body: toAnnouncementType(saved)}, nil
}

// errInUse says a type or role is still used, so the transaction that met it rolls back.
var errInUse = errors.New("in use")

func (h handlers) deleteAnnouncementType(ctx context.Context, in *announcementTypeIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	var deleted, used int64
	err := h.transact(ctx, func(q *db.Queries) error {
		var err error
		if used, err = q.CountAnnouncementsByType(ctx, in.ID); err != nil {
			return err
		}
		if used > 0 {
			return errInUse
		}
		deleted, err = q.DeleteAnnouncementType(ctx, in.ID)
		return err
	})
	switch {
	case errors.Is(err, errInUse):
		return nil, huma.Error409Conflict("This type is used by " + strconv.FormatInt(used, 10) + " announcement(s). Move or delete them first.")
	case err != nil:
		return nil, h.fail("delete an announcement type", err)
	case deleted == 0:
		return nil, huma.Error404NotFound(noSuchType)
	}
	return nil, nil
}

// -- Roles ---------------------------------------------------------------------

func (h handlers) listAnnouncementRoles(ctx context.Context, _ *struct{}) (*announcementRoleListOutput, error) {
	rows, err := h.read().ListAnnouncementRoles(ctx)
	if err != nil {
		return nil, h.fail("list announcement roles", err)
	}
	out := AnnouncementRoleList{Roles: make([]AnnouncementRole, 0, len(rows))}
	for _, r := range rows {
		out.Roles = append(out.Roles, toAnnouncementRole(r))
	}
	return &announcementRoleListOutput{Body: out}, nil
}

func (in AnnouncementRoleInput) cleaned() (AnnouncementRoleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.RoleID = strings.TrimSpace(in.RoleID)
	if !announcements.IsSnowflake(in.RoleID) {
		return in, huma.Error422UnprocessableEntity("The role can't be saved as it is.", &huma.ErrorDetail{
			Location: "body.roleId", Value: in.RoleID, Message: "A Discord role id is digits only.",
		})
	}
	return in, nil
}

func (h handlers) createAnnouncementRole(ctx context.Context, in *announcementRoleCreateInput) (*announcementRoleCreatedOutput, error) {
	body, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	made, err := h.write().CreateAnnouncementRole(ctx, db.CreateAnnouncementRoleParams{Name: body.Name, RoleID: body.RoleID})
	if err != nil {
		return nil, h.fail("create an announcement role", err)
	}
	return &announcementRoleCreatedOutput{Status: http.StatusCreated, Body: toAnnouncementRole(made)}, nil
}

func (h handlers) updateAnnouncementRole(ctx context.Context, in *announcementRoleSaveInput) (*announcementRoleOutput, error) {
	body, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	saved, err := h.write().UpdateAnnouncementRole(ctx, db.UpdateAnnouncementRoleParams{Name: body.Name, RoleID: body.RoleID, ID: in.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchRole)
	}
	if err != nil {
		return nil, h.fail("update an announcement role", err)
	}
	return &announcementRoleOutput{Body: toAnnouncementRole(saved)}, nil
}

func (h handlers) deleteAnnouncementRole(ctx context.Context, in *announcementRoleIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	var deleted, used int64
	err := h.transact(ctx, func(q *db.Queries) error {
		var err error
		if used, err = q.CountAnnouncementsByMention(ctx, announcements.RoleMention(in.ID)); err != nil {
			return err
		}
		if used > 0 {
			return errInUse
		}
		deleted, err = q.DeleteAnnouncementRole(ctx, in.ID)
		return err
	})
	switch {
	case errors.Is(err, errInUse):
		return nil, huma.Error409Conflict("This role is tagged by " + strconv.FormatInt(used, 10) + " announcement(s). Change their tag first.")
	case err != nil:
		return nil, h.fail("delete an announcement role", err)
	case deleted == 0:
		return nil, huma.Error404NotFound(noSuchRole)
	}
	return nil, nil
}

// -- Announcements -------------------------------------------------------------

func (h handlers) listAnnouncements(ctx context.Context, _ *struct{}) (*announcementListOutput, error) {
	list, err := h.announcementList(ctx)
	if err != nil {
		return nil, h.fail("list announcements", err)
	}
	return &announcementListOutput{Body: list}, nil
}

func (h handlers) announcementList(ctx context.Context) (AnnouncementList, error) {
	q := h.read()
	rows, err := q.ListAnnouncements(ctx)
	if err != nil {
		return AnnouncementList{}, err
	}
	types, err := q.ListAnnouncementTypes(ctx)
	if err != nil {
		return AnnouncementList{}, err
	}
	names := make(map[int64]string, len(types))
	for _, t := range types {
		names[t.ID] = t.Name
	}
	out := AnnouncementList{Announcements: make([]Announcement, 0, len(rows))}
	for _, r := range rows {
		out.Announcements = append(out.Announcements, toAnnouncement(r, names[r.TypeID]))
	}
	return out, nil
}

// resolve checks an announcement input against the rules and against the type and
// role it names. Every problem is a 422 naming its field.
func (h handlers) resolve(ctx context.Context, in AnnouncementInput, existing *announcements.Cursor) (announcements.Resolved, error) {
	r, problems := announcements.Resolve(announcements.Fields{
		Title: in.Title, Details: in.Details, Image: in.Image, Thumbnail: in.Thumbnail, Color: in.Color,
		Location: in.Location, StartFormat: in.StartFormat, EndFormat: in.EndFormat,
		StartLocal: in.StartLocal, EndLocal: in.EndLocal, OnceLocal: in.OnceLocal,
		Buttons: announcementButtons(in.Buttons), Mention: in.Mention, ScheduleKind: in.ScheduleKind,
		Timezone: in.Timezone, ScheduleMinutes: int(in.ScheduleMinutes), ScheduleWeekdays: in.ScheduleWeekdays,
		ScheduleWeekOfMonth: int(in.ScheduleWeekOfMonth),
	}, existing, time.Now())

	var errs []error
	for _, p := range problems {
		errs = append(errs, &huma.ErrorDetail{Location: "body." + p.Field, Value: p.Value, Message: p.Message})
	}
	q := h.read()
	if _, err := q.GetAnnouncementType(ctx, in.TypeID); errors.Is(err, sql.ErrNoRows) {
		errs = append(errs, &huma.ErrorDetail{Location: "body.typeId", Value: in.TypeID, Message: noSuchType})
	} else if err != nil {
		return r, h.fail("read an announcement type", err)
	}
	if _, role, ok := announcements.ParseMention(r.Mention); ok && role > 0 {
		if _, err := q.GetAnnouncementRole(ctx, role); errors.Is(err, sql.ErrNoRows) {
			errs = append(errs, &huma.ErrorDetail{Location: "body.mention", Value: r.Mention, Message: "The role to tag no longer exists."})
		} else if err != nil {
			return r, h.fail("read an announcement role", err)
		}
	}
	if len(errs) > 0 {
		return r, huma.Error422UnprocessableEntity("The announcement can't be saved as it is.", errs...)
	}
	return r, nil
}

// encodeButtons is the buttons as the JSON the column holds.
func encodeButtons(in []announcements.Button) (string, error) {
	buttons := make([]AnnouncementButton, 0, len(in))
	for _, b := range in {
		buttons = append(buttons, AnnouncementButton{Label: b.Label, Emoji: b.Emoji, URL: b.URL})
	}
	b, err := json.Marshal(buttons)
	return string(b), err
}

func (h handlers) createAnnouncement(ctx context.Context, in *announcementCreateInput) (*announcementCreatedOutput, error) {
	r, err := h.resolve(ctx, in.Body, nil)
	if err != nil {
		return nil, err
	}
	buttons, err := encodeButtons(r.Buttons)
	if err != nil {
		return nil, h.fail("encode announcement buttons", err)
	}
	made, err := h.write().CreateAnnouncement(context.WithoutCancel(ctx), db.CreateAnnouncementParams{
		TypeID: in.Body.TypeID, Title: r.Title, Details: r.Details, Image: r.Image, Thumbnail: r.Thumbnail, Color: r.Color,
		Location: r.Location, StartLocal: r.StartLocal, EndLocal: r.EndLocal, StartAt: nilIfEmpty(r.StartAt),
		EndAt: nilIfEmpty(r.EndAt), StartFormat: r.StartFormat, EndFormat: r.EndFormat,
		DynamicDates: boolInt(in.Body.DynamicDates), ScheduleKind: r.ScheduleKind, Timezone: r.Timezone,
		OnceLocal: r.OnceLocal, ScheduleMinutes: int64(r.ScheduleMinutes), ScheduleWeekdays: r.ScheduleWeekdays,
		ScheduleWeekOfMonth: int64(r.ScheduleWeekOfMonth), NextPostAt: nilIfEmpty(r.NextPostAt),
		Active: boolInt(r.Active), Buttons: buttons, Mention: r.Mention,
	})
	if err != nil {
		return nil, h.fail("create an announcement", err)
	}
	return &announcementCreatedOutput{Status: http.StatusCreated, Body: toAnnouncement(made, h.typeName(ctx, made.TypeID))}, nil
}

func (h handlers) updateAnnouncement(ctx context.Context, in *announcementSaveInput) (*announcementOutput, error) {
	existing, err := h.read().GetAnnouncement(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchAnnouncement)
	}
	if err != nil {
		return nil, h.fail("read an announcement", err)
	}
	r, err := h.resolve(ctx, in.Body, &announcements.Cursor{NextPostAt: deref(existing.NextPostAt), Active: existing.Active == 1})
	if err != nil {
		return nil, err
	}
	buttons, err := encodeButtons(r.Buttons)
	if err != nil {
		return nil, h.fail("encode announcement buttons", err)
	}
	saved, err := h.write().UpdateAnnouncement(context.WithoutCancel(ctx), db.UpdateAnnouncementParams{
		TypeID: in.Body.TypeID, Title: r.Title, Details: r.Details, Image: r.Image, Thumbnail: r.Thumbnail, Color: r.Color,
		Location: r.Location, StartLocal: r.StartLocal, EndLocal: r.EndLocal, StartAt: nilIfEmpty(r.StartAt),
		EndAt: nilIfEmpty(r.EndAt), StartFormat: r.StartFormat, EndFormat: r.EndFormat,
		DynamicDates: boolInt(in.Body.DynamicDates), ScheduleKind: r.ScheduleKind, Timezone: r.Timezone,
		OnceLocal: r.OnceLocal, ScheduleMinutes: int64(r.ScheduleMinutes), ScheduleWeekdays: r.ScheduleWeekdays,
		ScheduleWeekOfMonth: int64(r.ScheduleWeekOfMonth), NextPostAt: nilIfEmpty(r.NextPostAt),
		Active: boolInt(r.Active), Buttons: buttons, Mention: r.Mention, ID: in.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchAnnouncement)
	}
	if err != nil {
		return nil, h.fail("update an announcement", err)
	}
	return &announcementOutput{Body: toAnnouncement(saved, h.typeName(ctx, saved.TypeID))}, nil
}

func (h handlers) deleteAnnouncement(ctx context.Context, in *announcementIDInput) (*struct{}, error) {
	n, err := h.write().DeleteAnnouncement(context.WithoutCancel(ctx), in.ID)
	if err != nil {
		return nil, h.fail("delete an announcement", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound(noSuchAnnouncement)
	}
	return nil, nil
}

func (h handlers) reorderAnnouncements(ctx context.Context, in *announcementOrderInput) (*announcementListOutput, error) {
	order := in.Body.OrderedIDs
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		for i, id := range order {
			n, err := q.SetAnnouncementOrder(ctx, db.SetAnnouncementOrderParams{SortOrder: int64(i), ID: id})
			if err != nil {
				return err
			}
			if n == 0 {
				return missingError{err: errNoAnnouncement, index: i}
			}
		}
		return nil
	})
	var miss missingError
	switch {
	case errors.As(err, &miss):
		return nil, unknown("body.orderedIds["+strconv.Itoa(miss.index)+"]", order[miss.index], "announcement")
	case err != nil:
		return nil, h.fail("reorder announcements", err)
	}
	return h.listAnnouncements(ctx, nil)
}

var errNoAnnouncement = errors.New("no such announcement")

func (h handlers) sendAnnouncement(ctx context.Context, in *announcementIDInput) (*announcementOutput, error) {
	a, err := h.read().GetAnnouncement(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchAnnouncement)
	}
	if err != nil {
		return nil, h.fail("read an announcement", err)
	}
	typ, err := h.read().GetAnnouncementType(ctx, a.TypeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, h.fail("read an announcement type", err)
	}
	if typ == nil || strings.TrimSpace(typ.WebhookUrl) == "" {
		return nil, noWebhook("this announcement's type")
	}
	target := discord.Target{Kind: "announcement", Name: a.Title}
	if err := h.Discord.Post(ctx, target, typ.WebhookUrl, AnnouncementPayload(ctx, h.read(), a, time.Now())); err != nil {
		return nil, h.discordProblem(err, "send announcement "+strconv.FormatInt(a.ID, 10))
	}
	slog.Info("announcement sent manually", "id", a.ID, "title", a.Title, "type", typ.Name, "by", by(ctx))

	// It is on Discord now, so it is stamped as posted even if the staff member has
	// gone: the record must not say it was never sent.
	ctx = context.WithoutCancel(ctx)
	if _, err := h.write().TouchAnnouncementPosted(ctx, a.ID); err != nil {
		return nil, h.fail("stamp an announcement posted", err)
	}
	saved, err := h.read().GetAnnouncement(ctx, a.ID)
	if err != nil {
		return nil, h.fail("read an announcement", err)
	}
	return &announcementOutput{Body: toAnnouncement(saved, typ.Name)}, nil
}

func (h handlers) skipAnnouncement(ctx context.Context, in *announcementSkipInput) (*announcementOutput, error) {
	a, err := h.read().GetAnnouncement(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchAnnouncement)
	}
	if err != nil {
		return nil, h.fail("read an announcement", err)
	}
	if a.ScheduleKind == "" || a.NextPostAt == nil {
		return nil, huma.Error409Conflict("This announcement isn't scheduled, so there is nothing to skip.")
	}
	count := int64(1)
	if in.Body.Count != nil {
		count = *in.Body.Count
	}
	ctx = context.WithoutCancel(ctx)
	if _, err := h.write().SetAnnouncementSkip(ctx, db.SetAnnouncementSkipParams{SkipCount: count, ID: a.ID}); err != nil {
		return nil, h.fail("skip an announcement", err)
	}
	slog.Info("announcement skip set", "id", a.ID, "title", a.Title, "skip_count", count, "by", by(ctx))
	saved, err := h.read().GetAnnouncement(ctx, a.ID)
	if err != nil {
		return nil, h.fail("read an announcement", err)
	}
	return &announcementOutput{Body: toAnnouncement(saved, h.typeName(ctx, saved.TypeID))}, nil
}
