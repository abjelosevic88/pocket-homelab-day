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

// ---- what you listened to: Pocket Music or Maloja ----
//
// The music source keeps the id "maloja" (the days recorded so far, the
// cards, the timeline, the year page and the briefs all read it), but each
// person picks where the day comes from, like the location source:
//
//   - music: Pocket Music, the person's own listening history (every play
//     there, plus what it imported from Maloja / ListenBrainz), through a
//     Pocket Core app link (listening:read). Only through Core.
//   - maloja: Maloja, in whichever mode its connection is set to.
//
// Not chosen yet: Pocket Music on a box with Pocket Core, Maloja otherwise.
// While Pocket Music is chosen but its link is not on, or a fetch from it
// fails, a connected Maloja answers instead.

const (
	musPocket = "music"
	musMaloja = "maloja"
)

// musicApp is Pocket Music as Day uses it.
var musicApp = pocketSource{slug: "music", app: "Pocket Music", scope: "listening:read", reason: "What you listened to: songs, artists and minutes on each day of your journal"}

func validMusicProvider(p string) bool { return p == musPocket || p == musMaloja }

// musicProvider is where a person's music comes from: their choice, else
// Pocket Music when the box has Pocket Core. Without Pocket Core it is
// always Maloja.
func musicProvider(src store.Source, coreOK bool) string {
	if !coreOK || src.Provider == musMaloja {
		return musMaloja
	}
	return musPocket
}

// applyMusic adds Pocket Music to the music source's state: the provider in
// effect and, when it is Pocket Music, whether its link is on.
func applyMusic(ss *sourceState, src store.Source, coreOK bool, u *store.User, links func() (map[string]core.LinkState, error)) {
	ss.Provider = musicProvider(src, coreOK)
	ss.MalojaVia = ss.Via
	if ss.Provider != musPocket {
		return
	}
	if u == nil {
		ss.MusicDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[musicApp.slug+" "+musicApp.scope]
	switch {
	case err != nil:
		ss.MusicDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.MusicVia = modeCore
	case !ok && len(all) == 0:
		ss.MusicDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.MusicDetail = linkReason(musicApp.app, "not_declared")
	default:
		ss.MusicDetail = firstNonEmpty(linkReason(musicApp.app, l.State), l.Detail, l.State)
	}
	ss.Active = ss.MusicVia != "" || ss.MalojaVia != ""
}

// musicConn reaches a person's Pocket Music with a fresh Pocket Core link
// token on every call.
func (s *Server) musicConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Music needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, musicApp.slug, musicApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(musicApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 30*time.Second), nil
	}
}

// musicFetcher is the music source for a person who picked Pocket Music:
// Pocket Music first, a connected Maloja when it does not answer.
type musicFetcher struct {
	s        *Server
	music    sources.Fetcher
	fallback sources.Fetcher
}

func (f *musicFetcher) Name() string { return "maloja" }

func (f *musicFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	res, err := f.music.Fetch(ctx, date, loc)
	if err == nil || f.fallback == nil {
		return res, err
	}
	f.s.log.Warn("pocket music: falling back to maloja", "date", date, "err", err)
	return f.fallback.Fetch(ctx, date, loc)
}

func (f *musicFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.music.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.fallback == nil {
		return "", err
	}
	fmsg, ferr := f.fallback.Test(ctx)
	if ferr != nil {
		return "", fmt.Errorf("Pocket Music: %v; Maloja: %v", err, ferr)
	}
	return fmt.Sprintf("Pocket Music did not answer (%v); Maloja answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), fmsg), nil
}

// musicSource builds the music fetcher for a person: Pocket Music (with
// Maloja behind it) when that is their provider and its link is on, else Maloja.
func (s *Server) musicSource(u *store.User, src store.Source, ss sourceState) sources.Fetcher {
	var maloja sources.Fetcher
	if ss.MalojaVia != "" {
		maloja = s.serviceFetcher(u, "maloja", src, ss.Mode)
	}
	if ss.Provider != musPocket || ss.MusicVia == "" {
		return maloja
	}
	return &musicFetcher{s: s, music: sources.NewPocketMusic(s.musicConn(u)), fallback: maloja}
}
