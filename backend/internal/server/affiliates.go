package server

import (
	"net/http"
	"strings"

	"app-suite/internal/model"
)

// ── Affiliates admin (list + CRUD) ──────────────────────────────────────────
//
// An affiliate is a partner establishment (see model.Affiliate). Admins manage a
// flat list of them — create/edit/delete — with owners and opening hours edited
// as repeatable rows and persisted as JSON. The logo + screenshot are picked from
// the "Affiliate Logos" / "Affiliate Images" permanent image categories. There's
// no public view: this is admin-only management gated by permTeahouseAffiliates.

// handleAffiliatesList returns every affiliate, alphabetically by name.
//
//	Endpoint:  GET /api/affiliates
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  {"affiliates": [...]}
func (s *Server) handleAffiliatesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	affiliates, err := s.store.ListAffiliates()
	if err != nil {
		writeInternalError(w, "list affiliates", err)
		return
	}
	writeJSON(w, http.StatusOK, model.AffiliatesResponse{Affiliates: affiliates})
}

// affiliateWriteRequest is the JSON body for creating (POST /api/affiliates) or
// replacing (PUT /api/affiliates/{id}) an affiliate. The id comes from the path.
type affiliateWriteRequest struct {
	Name       string                `json:"name"`
	Owners     []string              `json:"owners"`
	Location   string                `json:"location"`
	Timezone   string                `json:"timezone"`
	Hours      []model.AffiliateHour `json:"hours"`
	Details    string                `json:"details"`
	Logo       string                `json:"logo"`
	Screenshot string                `json:"screenshot"`
}

// sanitizeOwners trims each owner name and drops blanks, preserving order. Blank
// repeater rows from the form are discarded so they aren't persisted.
func sanitizeOwners(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// sanitizeAffiliateHours trims each hours row and drops rows without a start time
// (an empty repeater row), preserving order. A row needs at least a start.
func sanitizeAffiliateHours(in []model.AffiliateHour) []model.AffiliateHour {
	out := make([]model.AffiliateHour, 0, len(in))
	for _, h := range in {
		start := strings.TrimSpace(h.Start)
		if start == "" {
			continue
		}
		out = append(out, model.AffiliateHour{
			Label: strings.TrimSpace(h.Label),
			Start: start,
			End:   strings.TrimSpace(h.End),
		})
	}
	return out
}

// affiliateFromRequest builds a sanitized model.Affiliate (sans ID) from a request.
func affiliateFromRequest(req affiliateWriteRequest, name string) *model.Affiliate {
	return &model.Affiliate{
		Name:       name,
		Owners:     sanitizeOwners(req.Owners),
		Location:   strings.TrimSpace(req.Location),
		Timezone:   strings.TrimSpace(req.Timezone),
		Hours:      sanitizeAffiliateHours(req.Hours),
		Details:    req.Details,
		Logo:       strings.TrimSpace(req.Logo),
		Screenshot: strings.TrimSpace(req.Screenshot),
	}
}

// handleAffiliateCreate creates an affiliate.
//
//	Endpoint:  POST /api/affiliates
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  201 {"affiliate": Affiliate}
func (s *Server) handleAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	req, err := readJSON[affiliateWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}
	affiliate := affiliateFromRequest(req, name)
	id, err := s.store.CreateAffiliate(affiliate)
	if err != nil {
		writeInternalError(w, "create affiliate", err)
		return
	}
	affiliate.ID = id
	writeJSON(w, http.StatusCreated, model.AffiliateResponse{Affiliate: *affiliate})
}

// handleAffiliateUpdate replaces an affiliate.
//
//	Endpoint:  PUT /api/affiliates/{id}
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  200 {"ok": true}
func (s *Server) handleAffiliateUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	req, err := readJSON[affiliateWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}
	affiliate := affiliateFromRequest(req, name)
	affiliate.ID = id
	if err := s.store.UpdateAffiliate(affiliate); err != nil {
		writeInternalError(w, "update affiliate", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleAffiliateDelete deletes an affiliate.
//
//	Endpoint:  DELETE /api/affiliates/{id}
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  204 No Content
func (s *Server) handleAffiliateDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	if _, err := s.store.DeleteAffiliate(id); err != nil {
		writeInternalError(w, "delete affiliate", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
