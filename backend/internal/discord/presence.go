package discord

import (
	"slices"
	"strconv"
	"strings"
)

// Presence is how the bot shows in the member list: the dot beside its name and the line
// under it.
type Presence struct {
	Status   string // online (default), idle, dnd, invisible
	Activity string // none (default), custom, playing, listening, watching, competing
	Text     string
}

// Discord's numbers for the kinds of activity a bot may show. Streaming is left out: it
// needs a stream to point at.
var activityTypes = map[string]int{"playing": 0, "listening": 2, "watching": 3, "custom": 4, "competing": 5}

// payload is the presence as the gateway takes it, in an identify or an update. An activity
// with no words, or of a kind Discord does not have, is no activity.
func (p Presence) payload() map[string]any {
	status := p.Status
	if status == "" {
		status = "online"
	}
	activities := []any{}
	text := strings.TrimSpace(p.Text)
	if kind, ok := activityTypes[p.Activity]; ok && text != "" {
		activity := map[string]any{"type": kind, "name": text}
		if p.Activity == "custom" {
			// A custom status's words go in state; Discord still wants a name, never shown.
			activity["name"] = "Custom Status"
			activity["state"] = text
		}
		activities = append(activities, activity)
	}
	return map[string]any{"status": status, "since": nil, "afk": false, "activities": activities}
}

// permAdministrator is Discord's Administrator permission bit.
const permAdministrator = 1 << 3

// HasAdministrator reports whether a member holding these roles is an administrator of the
// server: whether @everyone (the role with the server's own id) or any held role grants
// Administrator. Administrator bypasses every channel overwrite, so for the app's bot this
// one check stands in for every per-channel permission question.
func HasAdministrator(guildID string, holds []string, roles []Role) bool {
	for _, r := range roles {
		if r.ID != guildID && !slices.Contains(holds, r.ID) {
			continue
		}
		bits, _ := strconv.ParseUint(r.Permissions, 10, 64)
		if bits&permAdministrator != 0 {
			return true
		}
	}
	return false
}
