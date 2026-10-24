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

// ---- audiobooks: Pocket Audiobooks or Audiobookshelf ----
//
// The audiobooks source keeps the id "audiobookshelf" (the days recorded so
// far, the cards, the timeline, the year page and the briefs all read it), but
// each person picks where the day comes from, like the music source:
//
//   - audiobooks: Pocket Audiobooks, the person's own listening history (their
//     sessions in every Audiobookshelf app, books only), through a Pocket Core
//     app link (listening:read). Only through Core.
//   - audiobookshelf: Audiobookshelf's sessions, in whichever mode its
//     connection is set to (podcasts there included).
//
// Not chosen yet: Pocket Audiobooks on a box with Pocket Core, Audiobookshelf
// otherwise. While Pocket Audiobooks is chosen but its link is not on, or a
// fetch from it fails, a connected Audiobookshelf answers instead.

const (
	abPocket = "audiobooks"
	abABS    = "audiobookshelf"
)

// audiobooksApp is Pocket Audiobooks as Day uses it.
var audiobooksApp = pocketSource{slug: "audiobooks", app: "Pocket Audiobooks", scope: "listening:read", reason: "What you listened to: audiobooks and minutes on each day of your journal"}

func validAudiobooksProvider(p string) bool { return p == abPocket || p == abABS }

// audiobooksProvider is where a person's audiobooks come from: their choice,
// else Pocket Audiobooks when the box has Pocket Core.
func audiobooksProvider(src store.Source, coreOK bool) string {
	if !coreOK || src.Provider == abABS {
		return abABS
	}
	return abPocket
}

// applyAudiobooks adds Pocket Audiobooks to the audiobooks source's state.
func applyAudiobooks(ss *sourceState, src store.Source, coreOK bool, u *store.User, links func() (map[string]core.LinkState, error)) {
	ss.Provider = audiobooksProvider(src, coreOK)
	ss.ABSVia = ss.Via
	if ss.Provider != abPocket {
		return
	}
	if u == nil {
		ss.AudiobooksDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[audiobooksApp.slug+" "+audiobooksApp.scope]
	switch {
	case err != nil:
		ss.AudiobooksDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.AudiobooksVia = modeCore
	case !ok && len(all) == 0:
		ss.AudiobooksDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.AudiobooksDetail = linkReason(audiobooksApp.app, "not_declared")
	default:
		ss.AudiobooksDetail = firstNonEmpty(linkReason(audiobooksApp.app, l.State), l.Detail, l.State)
	}
	ss.Active = ss.AudiobooksVia != "" || ss.ABSVia != ""
}

// audiobooksConn reaches a person's Pocket Audiobooks with a fresh Pocket Core link token on every call.
func (s *Server) audiobooksConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Audiobooks needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, audiobooksApp.slug, audiobooksApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(audiobooksApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 30*time.Second), nil
	}
}

// audiobooksFetcher is the audiobooks source for a person who picked Pocket
// Audiobooks: Pocket Audiobooks first, a connected Audiobookshelf when it does not answer.
type audiobooksFetcher struct {
	s        *Server
	books    sources.Fetcher
	fallback sources.Fetcher
}

func (f *audiobooksFetcher) Name() string { return "audiobookshelf" }

func (f *audiobooksFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	res, err := f.books.Fetch(ctx, date, loc)
	if err == nil || f.fallback == nil {
		return res, err
	}
	f.s.log.Warn("pocket audiobooks: falling back to audiobookshelf", "date", date, "err", err)
	return f.fallback.Fetch(ctx, date, loc)
}

func (f *audiobooksFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.books.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.fallback == nil {
		return "", err
	}
	fmsg, ferr := f.fallback.Test(ctx)
	if ferr != nil {
		return "", fmt.Errorf("Pocket Audiobooks: %v; Audiobookshelf: %v", err, ferr)
	}
	return fmt.Sprintf("Pocket Audiobooks did not answer (%v); Audiobookshelf answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), fmsg), nil
}

// audiobooksSource builds the audiobooks fetcher for a person: Pocket
// Audiobooks (with Audiobookshelf behind it) when that is their provider and
// its link is on, else Audiobookshelf.
func (s *Server) audiobooksSource(u *store.User, src store.Source, ss sourceState) sources.Fetcher {
	var direct sources.Fetcher
	if ss.ABSVia != "" {
		direct = s.serviceFetcher(u, "audiobookshelf", src, ss.Mode)
	}
	if ss.Provider != abPocket || ss.AudiobooksVia == "" {
		return direct
	}
	return &audiobooksFetcher{s: s, books: sources.NewPocketAudiobooks(s.audiobooksConn(u)), fallback: direct}
}
