package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/discord"
)

// DiscordWebhook is the Discord webhook a page posts to.
type DiscordWebhook struct {
	WebhookURL string `json:"webhookUrl" maxLength:"500" doc:"The Discord webhook: https, on Discord, under /api/webhooks/. Empty for none. A webhook grants posting access to a channel, so only staff with the page see it."`
}

// check is a 422 when the webhook is set and isn't a Discord webhook URL.
func (in DiscordWebhook) check() error {
	if in.WebhookURL != "" && !discord.IsWebhookURL(in.WebhookURL) {
		return huma.Error422UnprocessableEntity("The webhook can't be saved as it is.", &huma.ErrorDetail{
			Location: "body.webhookUrl", Value: in.WebhookURL,
			Message: "A Discord webhook URL looks like https://discord.com/api/webhooks/...",
		})
	}
	return nil
}

// discordErrors are the errors an operation that posts to Discord can answer, beyond
// its guards.
var discordErrors = []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable}

// noWebhook is the 500 for posting where no webhook is set: the server's
// configuration is missing, and Discord is never called.
func noWebhook(what string) error {
	return huma.Error500InternalServerError("No Discord webhook is set for " + what + ". Add one first.")
}

// discordProblem logs a failed post to Discord once and answers it: 429 with
// Retry-After when Discord limits posts, 503 when it can't be reached or didn't
// answer (the post may have gone out), 500 when it refused the post. what names the
// post for the log.
func (h handlers) discordProblem(err error, what string) error {
	return h.discordProblemAfter(err, what, "")
}

// discordProblemAfter is discordProblem for a post that failed part-way through a run
// of them: prefix leads the detail, to say how far it got.
func (h handlers) discordProblemAfter(err error, what, prefix string) error {
	status, wait := discord.HTTPStatus(err)
	var de *discord.Error
	errors.As(err, &de)

	level := h.Logger.Error
	if de != nil && (de.Kind == discord.RateLimited || de.Kind == discord.Ambiguous) {
		level = h.Logger.Warn
	}
	level("discord post failed", "what", what, "status", status, "error", err)

	switch {
	case de == nil:
		return huma.Error500InternalServerError(prefix + "The post to Discord failed.")
	case de.Kind == discord.Ambiguous:
		return huma.Error503ServiceUnavailable(prefix + "Discord didn't answer, so the post may or may not have gone out. Check the channel before trying again.")
	case de.Kind == discord.Unreachable:
		return huma.Error503ServiceUnavailable(prefix + "Discord can't be reached right now. Try again shortly.")
	case de.Kind == discord.RateLimited:
		seconds := max(int((wait+time.Second-1)/time.Second), 1)
		return huma.ErrorWithHeaders(
			huma.Error429TooManyRequests(prefix+"Discord is limiting posts right now. Try again in "+strconv.Itoa(seconds)+" seconds."),
			http.Header{"Retry-After": {strconv.Itoa(seconds)}})
	default:
		return huma.Error500InternalServerError(prefix + "Discord refused the post: " + de.Message + ". Check the webhook.")
	}
}
