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

// ---- what you watched: Pocket Watch or Jellystat ----
//
// The watched source keeps the id "jellystat" (the days recorded so far,
// the cards, the calendar and year layers and the briefs all read it), but
// each person picks where the day comes from, like the music source:
//
//   - watch: Pocket Watch, the person's own Jellyfin history (its player and
//     every other Jellyfin app), through a Pocket Core app link
//     (watching:read). Only through Core.
//   - jellystat: Jellystat, in whichever mode its connection is set to.
//
// Not chosen yet: Pocket Watch on a box with Pocket Core, Jellystat
// otherwise. While Pocket Watch is chosen but its link is not on, or a
// fetch from it fails, a connected Jellystat answers instead.

const (
	watchPocket    = "watch"
	watchJellystat = "jellystat"
)

// watchApp is Pocket Watch as Day uses it.
var watchApp = pocketSource{slug: "watch", app: "Pocket Watch", scope: "watching:read", reason: "The films and episodes you watched, with minutes, on each day of your journal"}

func validWatchProvider(p string) bool { return p == watchPocket || p == watchJellystat }

// watchProvider is where a person's watching comes from: their choice,
// else Pocket Watch when the box has Pocket Core. Without Pocket Core it is
// always Jellystat.
func watchProvider(src store.Source, coreOK bool) string {
	if !coreOK || src.Provider == watchJellystat {
		return watchJellystat
	}
	return watchPocket
}

// applyWatch adds Pocket Watch to the watched source's state: the provider
// in effect and, when it is Pocket Watch, whether its link is on.
func applyWatch(ss *sourceState, src store.Source, coreOK bool, u *store.User, links func() (map[string]core.LinkState, error)) {
	ss.Provider = watchProvider(src, coreOK)
	ss.JellystatVia = ss.Via
	if ss.Provider != watchPocket {
		return
	}
	if u == nil {
		ss.WatchDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[watchApp.slug+" "+watchApp.scope]
	switch {
	case err != nil:
		ss.WatchDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.WatchVia = modeCore
	case !ok && len(all) == 0:
		ss.WatchDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.WatchDetail = linkReason(watchApp.app, "not_declared")
	default:
		ss.WatchDetail = firstNonEmpty(linkReason(watchApp.app, l.State), l.Detail, l.State)
	}
	ss.Active = ss.WatchVia != "" || ss.JellystatVia != ""
}

// watchConn reaches a person's Pocket Watch with a fresh Pocket Core
// link token on every call.
func (s *Server) watchConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Watch needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, watchApp.slug, watchApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(watchApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 30*time.Second), nil
	}
}

// watchFetcher is the watched source for a person who picked Pocket Watch:
// Pocket Watch first, a connected Jellystat when it does not answer.
type watchFetcher struct {
	s        *Server
	watch    sources.Fetcher
	fallback sources.Fetcher
}

func (f *watchFetcher) Name() string { return "jellystat" }

func (f *watchFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	res, err := f.watch.Fetch(ctx, date, loc)
	if err == nil || f.fallback == nil {
		return res, err
	}
	f.s.log.Warn("pocket watch: falling back to jellystat", "date", date, "err", err)
	return f.fallback.Fetch(ctx, date, loc)
}

func (f *watchFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.watch.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.fallback == nil {
		return "", err
	}
	fmsg, ferr := f.fallback.Test(ctx)
	if ferr != nil {
		return "", fmt.Errorf("Pocket Watch: %v; Jellystat: %v", err, ferr)
	}
	return fmt.Sprintf("Pocket Watch did not answer (%v); Jellystat answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), fmsg), nil
}

// watchSource builds the watched fetcher for a person: Pocket Watch (with
// Jellystat behind it) when that is their provider and its link is on, else
// Jellystat.
func (s *Server) watchSource(u *store.User, src store.Source, ss sourceState) sources.Fetcher {
	var jellystat sources.Fetcher
	if ss.JellystatVia != "" {
		jellystat = s.serviceFetcher(u, "jellystat", src, ss.Mode)
	}
	if ss.Provider != watchPocket || ss.WatchVia == "" {
		return jellystat
	}
	return &watchFetcher{s: s, watch: sources.NewPocketWatch(s.watchConn(u)), fallback: jellystat}
}
