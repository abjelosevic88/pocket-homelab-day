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

// ---- where you were: Pocket Movement or Dawarich ----
//
// The location source keeps the id "dawarich" (the days recorded so far, the
// cards, the timeline, the calendar layers, the year page and the AI briefs
// all read it), but each person picks where the day comes from:
//
//   - movement: Pocket Movement, the person's own location history, through
//     a Pocket Core app link (movement:read, its day view). Only through
//     Core: there is no URL and token to paste for it.
//   - dawarich: Dawarich, in whichever mode its connection is set to
//     (Pocket Core service connection, direct URL and key, or off).
//
// Not chosen yet: Pocket Movement on a box with Pocket Core, Dawarich
// otherwise. A Dawarich URL and key from the environment do not change that;
// Dawarich stays the alternative. While Pocket Movement is chosen but its
// link is not on (switched off in Core, or Movement missing), or a fetch
// from it fails, a connected Dawarich answers instead, so a day never goes
// empty because of the switch.

const (
	locMovement = "movement"
	locDawarich = "dawarich"
)

// movementApp is Pocket Movement as Day uses it.
var movementApp = pocketSource{slug: "movement", app: "Pocket Movement", scope: "movement:read", reason: "Where you went: journeys, stops and places on each day of your journal"}

func validLocationProvider(p string) bool { return p == locMovement || p == locDawarich }

// locationProvider is where a person's days come from: their choice, else
// Pocket Movement when the box has Pocket Core. Without Pocket Core it is
// always Dawarich (a stored "movement" cannot work there).
func locationProvider(src store.Source, coreOK bool) string {
	if !coreOK || src.Provider == locDawarich {
		return locDawarich
	}
	return locMovement
}

// applyLocation adds Pocket Movement to the location source's state: the
// provider in effect and, when it is Pocket Movement, whether its link is on.
// The source is pulled when either Pocket Movement or Dawarich answers.
func applyLocation(ss *sourceState, src store.Source, coreOK bool, u *store.User, links func() (map[string]core.LinkState, error)) {
	ss.Provider = locationProvider(src, coreOK)
	ss.DawarichVia = ss.Via
	if ss.Provider != locMovement {
		return
	}
	if u == nil {
		ss.MovementDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[movementApp.slug+" "+movementApp.scope]
	switch {
	case err != nil:
		ss.MovementDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.MovementVia = modeCore
	case !ok && len(all) == 0:
		ss.MovementDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.MovementDetail = linkReason(movementApp.app, "not_declared")
	default:
		ss.MovementDetail = firstNonEmpty(linkReason(movementApp.app, l.State), l.Detail, l.State)
	}
	ss.Active = ss.MovementVia != "" || ss.DawarichVia != ""
}

// movementConn reaches a person's Pocket Movement with a fresh Pocket Core
// link token on every call.
func (s *Server) movementConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Movement needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, movementApp.slug, movementApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(movementApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 30*time.Second), nil
	}
}

// locationFetcher is the location source for a person who picked Pocket
// Movement: Movement first, a connected Dawarich when Movement does not
// answer (nil fallback = Dawarich is not connected).
type locationFetcher struct {
	s        *Server
	movement sources.Fetcher
	fallback sources.Fetcher
}

func (f *locationFetcher) Name() string { return "dawarich" }

func (f *locationFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	res, err := f.movement.Fetch(ctx, date, loc)
	if err == nil || f.fallback == nil {
		return res, err
	}
	f.s.log.Warn("pocket movement: falling back to dawarich", "date", date, "err", err)
	return f.fallback.Fetch(ctx, date, loc)
}

func (f *locationFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.movement.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.fallback == nil {
		return "", err
	}
	fmsg, ferr := f.fallback.Test(ctx)
	if ferr != nil {
		return "", fmt.Errorf("Pocket Movement: %v; Dawarich: %v", err, ferr)
	}
	return fmt.Sprintf("Pocket Movement did not answer (%v); Dawarich answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), fmsg), nil
}

// locationSource builds the location fetcher for a person: Pocket Movement
// (with Dawarich behind it) when that is their provider and its link is on,
// else Dawarich alone.
func (s *Server) locationSource(u *store.User, src store.Source, ss sourceState) sources.Fetcher {
	var dawarich sources.Fetcher
	if ss.DawarichVia != "" {
		dawarich = s.serviceFetcher(u, "dawarich", src, ss.Mode)
	}
	if ss.Provider != locMovement || ss.MovementVia == "" {
		return dawarich
	}
	return &locationFetcher{s: s, movement: sources.NewMovement(s.movementConn(u)), fallback: dawarich}
}
