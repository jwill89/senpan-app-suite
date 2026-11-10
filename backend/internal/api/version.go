package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// AppVersion is the app's version as the server runs it.
type AppVersion struct {
	Version string `json:"version" doc:"The app's semantic version: one version for the SPA and the API since 4.0.0." example:"5.0.0"`
}

type versionOutput struct {
	Body AppVersion
}

func (h handlers) registerVersion(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-version",
		Method:      http.MethodGet,
		Path:        "/version",
		Summary:     "The app's version",
		Description: "The version the server runs. An open tab compares it with its own build's, to offer a reload after a deploy. GET /healthz reports it too.",
		Tags:        []string{"System"},
		Middlewares: huma.Middlewares{public},
	}, func(context.Context, *struct{}) (*versionOutput, error) {
		return &versionOutput{Body: AppVersion{Version: h.Version}}, nil
	})
}
