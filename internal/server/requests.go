package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- what you asked for: Pocket Requests or Jellyseerr ----
//
// The requests source keeps the id "jellyseerr" (the days recorded so far,
// the cards, the calendar and year layers and the briefs all read it), but
// each person picks where the day comes from, like the music source:
//
//   - requests: Pocket Requests, the person's own films, shows and albums
//     (including the requests it imported from Jellyseerr), through a Pocket
//     Core app link (requests:read). Only through Core.
//   - jellyseerr: Jellyseerr, in whichever mode its connection is set to.
//
// Not chosen yet: Pocket Requests on a box with Pocket Core, Jellyseerr
// otherwise. While Pocket Requests is chosen but its link is not on, or a
// fetch from it fails, a connected Jellyseerr answers instead.

const (
	reqPocket = "requests"
	reqSeerr  = "jellyseerr"
)

// requestsApp is Pocket Requests as Day uses it.
var requestsApp = pocketSource{slug: "requests", app: "Pocket Requests", scope: "requests:read", reason: "The films, shows and albums you asked for, and the ones that arrived, on each day of your journal"}

func validRequestsProvider(p string) bool { return p == reqPocket || p == reqSeerr }

// requestsProvider is where a person's requests come from: their choice,
// else Pocket Requests when the box has Pocket Core. Without Pocket Core it
// is always Jellyseerr.
func requestsProvider(src store.Source, coreOK bool) string {
	if !coreOK || src.Provider == reqSeerr {
		return reqSeerr
	}
	return reqPocket
}

// applyRequests adds Pocket Requests to the requests source's state: the
// provider in effect and, when it is Pocket Requests, whether its link is on.
func applyRequests(ss *sourceState, src store.Source, coreOK bool, u *store.User, links func() (map[string]core.LinkState, error)) {
	ss.Provider = requestsProvider(src, coreOK)
	ss.SeerrVia = ss.Via
	if ss.Provider != reqPocket {
		return
	}
	if u == nil {
		ss.RequestsDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[requestsApp.slug+" "+requestsApp.scope]
	switch {
	case err != nil:
		ss.RequestsDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.RequestsVia = modeCore
	case !ok && len(all) == 0:
		ss.RequestsDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.RequestsDetail = linkReason(requestsApp.app, "not_declared")
	default:
		ss.RequestsDetail = firstNonEmpty(linkReason(requestsApp.app, l.State), l.Detail, l.State)
	}
	ss.Active = ss.RequestsVia != "" || ss.SeerrVia != ""
}

// requestsConn reaches a person's Pocket Requests with a fresh Pocket Core
// link token on every call.
func (s *Server) requestsConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Requests needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, requestsApp.slug, requestsApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(requestsApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 30*time.Second), nil
	}
}

// requestsFetcher is the requests source for a person who picked Pocket
// Requests: Pocket Requests first, a connected Jellyseerr when it does not answer.
type requestsFetcher struct {
	s        *Server
	requests sources.Fetcher
	fallback sources.Fetcher
}

func (f *requestsFetcher) Name() string { return "jellyseerr" }

func (f *requestsFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	res, err := f.requests.Fetch(ctx, date, loc)
	if err == nil || f.fallback == nil {
		return res, err
	}
	f.s.log.Warn("pocket requests: falling back to jellyseerr", "date", date, "err", err)
	return f.fallback.Fetch(ctx, date, loc)
}

func (f *requestsFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.requests.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.fallback == nil {
		return "", err
	}
	fmsg, ferr := f.fallback.Test(ctx)
	if ferr != nil {
		return "", fmt.Errorf("Pocket Requests: %v; Jellyseerr: %v", err, ferr)
	}
	return fmt.Sprintf("Pocket Requests did not answer (%v); Jellyseerr answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), fmsg), nil
}

// requestsSource builds the requests fetcher for a person: Pocket Requests
// (with Jellyseerr behind it) when that is their provider and its link is
// on, else Jellyseerr.
func (s *Server) requestsSource(u *store.User, src store.Source, ss sourceState) sources.Fetcher {
	var seerr sources.Fetcher
	if ss.SeerrVia != "" {
		seerr = s.serviceFetcher(u, "jellyseerr", src, ss.Mode)
	}
	if ss.Provider != reqPocket || ss.RequestsVia == "" {
		return seerr
	}
	return &requestsFetcher{s: s, requests: sources.NewPocketRequests(s.requestsConn(u)), fallback: seerr}
}
