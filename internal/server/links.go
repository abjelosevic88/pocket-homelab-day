package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// importMaxBytes is the largest journal export the import takes (facts make
// years of a journal big).
const importMaxBytes = 256 << 20

// portable is what Pocket Move may copy out and bring in (docs/move.md in
// pocket-homelab-core), for the person the link acts for.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "day.json", Kind: core.KindPocket, Title: "Your whole journal: days, moments, pins and what the apps knew about each day"},
		{Path: "/api/v1/export.md", File: "journal.md", Kind: core.KindMarkdown, Title: "Your journal as one Markdown file, a heading per day"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", MaxBytes: importMaxBytes, Title: "Adds the journal from another box; what you wrote here is kept"},
	},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a pcx_ link token with these scopes may make
// this request. Day offers nothing but Pocket Move's routes.
func linkAllows(scopes []string, r *http.Request) bool {
	return portable.LinkAllows(scopes, r)
}

// linkUser validates a pcx_ token and maps it to the person it acts for.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" {
		return nil, http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, errLinkScope
	}
	return s.coreIdentityUser(ctx, in)
}
