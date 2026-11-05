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

// ---- the weather: Pocket Weather or straight from Open-Meteo ----
//
// The weather source is not one of the connectable sources (it has no URL
// or key of its own), but while the person has a Pocket Core link
// Day → Pocket Weather (weather:read) the days come from Pocket Weather: it
// knows their home place, keeps a diary of each day (their notes too) and
// has the warnings. Without the link (no Pocket Core, the link off, Pocket
// Weather not registered) the weather comes straight from Open-Meteo for the
// home location set in Day, as it always did; and when the link goes off
// between two pulls, or Pocket Weather has no place for the person yet,
// Open-Meteo answers instead. Other errors from Pocket Weather are the day's
// error, like any source's.

// weatherApp is Pocket Weather as Day uses it.
var weatherApp = pocketSource{slug: "weather", app: "Pocket Weather", scope: "weather:read", reason: "Write the day's weather from Pocket Weather into your journal: your own home place, the weather diary with your notes, and the warnings of the day"}

// hasHome reports whether the person set a home location in Day (what the
// direct Open-Meteo weather needs).
func hasHome(st store.Settings) bool { return st.HomeLat != 0 || st.HomeLon != 0 }

// applyWeather sets the weather source's state: whether Pocket Weather's link
// is on (WeatherVia) and why not, and whether the source is pulled at all.
func applyWeather(ss *sourceState, src store.Source, st store.Settings, coreOK bool, links func() (map[string]core.LinkState, error)) {
	if src.Enabled {
		if !coreOK {
			ss.WeatherDetail = "there is no Pocket Core on this box"
		} else {
			all, err := links()
			l, ok := all[weatherApp.slug+" "+weatherApp.scope]
			switch {
			case err != nil:
				ss.WeatherDetail = "Pocket Core did not answer: " + err.Error()
			case ok && l.On:
				ss.WeatherVia = modeCore
			case !ok && len(all) == 0:
				ss.WeatherDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
			case !ok:
				ss.WeatherDetail = linkReason(weatherApp.app, "not_declared")
			default:
				ss.WeatherDetail = firstNonEmpty(linkReason(weatherApp.app, l.State), l.Detail, l.State)
			}
		}
	}
	ss.Active = src.Enabled && (ss.WeatherVia != "" || hasHome(st))
}

// weatherConn reaches a person's Pocket Weather with a fresh Pocket Core link
// token on every call.
func (s *Server) weatherConn(u *store.User) pocket.Conn {
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: Pocket Weather needs Pocket Core", pocket.ErrNotConnected)
		}
		tok, err := s.core.ActAs(ctx, email, weatherApp.slug, weatherApp.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(weatherApp.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/"), tok.AccessToken, 20*time.Second), nil
	}
}

// weatherAt is a weather fetcher that can also answer for another place
// (where the day was, when the location source knows it).
type weatherAt interface {
	sources.Fetcher
	FetchAt(ctx context.Context, date string, lat, lon float64, place string) (*sources.Result, error)
}

// weatherFetcher is the weather source while Pocket Weather's link is on:
// Pocket Weather first, Open-Meteo for the home location in Day (when one is
// set) when Pocket Weather is not connected after all or has no place yet.
type weatherFetcher struct {
	s      *Server
	pocket *sources.PocketWeather
	direct *sources.Weather // nil without a home location in Day
}

func (f *weatherFetcher) Name() string { return "weather" }

func (f *weatherFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	return f.FetchAt(ctx, date, 0, 0, "")
}

func (f *weatherFetcher) FetchAt(ctx context.Context, date string, lat, lon float64, place string) (*sources.Result, error) {
	res, err := f.pocket.FetchAt(ctx, date, lat, lon, place)
	if err == nil || !fallsBack(err) || f.direct == nil {
		if errors.Is(err, pocket.ErrNotFound) {
			return nil, errors.New("Pocket Weather has nothing for that day: add your home place there, or set a home location in Day")
		}
		return res, err
	}
	f.s.log.Debug("pocket weather: weather straight from open-meteo", "date", date, "err", err)
	if lat == 0 && lon == 0 {
		return f.direct.Fetch(ctx, date, time.Local)
	}
	return f.direct.FetchAt(ctx, date, lat, lon, place)
}

// fallsBack reports whether Open-Meteo answers instead of Pocket Weather: the
// link is not on after all, or Pocket Weather has nothing (no place yet).
func fallsBack(err error) bool {
	return errors.Is(err, pocket.ErrNotConnected) || errors.Is(err, pocket.ErrNotFound)
}

func (f *weatherFetcher) Test(ctx context.Context) (string, error) {
	msg, err := f.pocket.Test(ctx)
	if err == nil {
		return msg + " through Pocket Core", nil
	}
	if f.direct == nil {
		return "", err
	}
	dmsg, derr := f.direct.Test(ctx)
	if derr != nil {
		return "", fmt.Errorf("Pocket Weather: %v; Open-Meteo: %v", err, derr)
	}
	return fmt.Sprintf("Pocket Weather did not answer (%v); Open-Meteo answers instead: %s", strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": "), dmsg), nil
}

// weatherSource builds the weather fetcher for a person: Pocket Weather (with
// Open-Meteo behind it) while its link is on, else Open-Meteo for the home
// location; nil when neither works.
func (s *Server) weatherSource(u *store.User, st store.Settings, ss sourceState) weatherAt {
	var direct *sources.Weather
	if hasHome(st) {
		direct = sources.NewWeather(st.HomeLat, st.HomeLon, st.HomePlace, s.cfg.UserAgent)
	}
	if ss.WeatherVia == "" {
		if direct == nil {
			return nil
		}
		return direct
	}
	return &weatherFetcher{s: s, pocket: sources.NewPocketWeather(s.weatherConn(u)), direct: direct}
}
