package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ClientConfig is what the SPA needs from the server's own configuration before
// anyone signs in.
type ClientConfig struct {
	TurnstileSiteKey string `json:"turnstileSiteKey" doc:"The Cloudflare Turnstile site key the sign-in, sign-up and public entry forms show; empty when the bot check is off."`
	DiscordSignIn    bool   `json:"discordSignIn" doc:"Whether signing in with Discord is set up (GET /auth/discord/start)."`
}

type configOutput struct {
	Body ClientConfig
}

func (h handlers) registerConfig(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-config",
		Method:      http.MethodGet,
		Path:        "/config",
		Summary:     "The client's configuration",
		Description: "The non-secret configuration a page needs before sign-in: the Turnstile site key, and whether Discord sign-in is on.",
		Tags:        []string{"System"},
		Middlewares: huma.Middlewares{public},
	}, func(context.Context, *struct{}) (*configOutput, error) {
		out := &configOutput{}
		if h.TurnstileSiteKey != nil {
			out.Body.TurnstileSiteKey = h.TurnstileSiteKey()
		}
		out.Body.DiscordSignIn = h.DiscordOAuth.Enabled()
		return out, nil
	})
}
