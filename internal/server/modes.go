package server

import (
	"context"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- connection modes on Pocket Core's map ----
//
// Core can switch its own links but cannot see a URL and key pasted into
// Day, so Day tells it how each person actually reaches every connectable
// source: "core", "direct" or "off", the mode in effect (what Settings →
// Sources shows, defaults included), with the source's Core target ("habits",
// "money" for the money source, "service:immich"…). Reported after each
// registration (on start and every 30 minutes) and right after a person's
// modes change (a mode picked, Disconnect, a key pasted); one batched call
// for everybody. Report only: failures are logged and nothing depends on them.

// connectableOrder is every source reached through Core or directly, in
// manifest order: the Pocket apps, then the services.
func connectableOrder() []string {
	return append(append([]string(nil), pocketSourceOrder...), serviceSourceOrder...)
}

// personModes is one person's effective mode for every connectable source.
func (s *Server) personModes(email string, st store.Settings) []core.ConnMode {
	coreOK := s.core.Configured()
	names := connectableOrder()
	out := make([]core.ConnMode, 0, len(names))
	for _, n := range names {
		ct, _ := connectable(n)
		out = append(out, core.ConnMode{User: email, Target: ct.target, Mode: connMode(n, st.Sources[n], coreOK)})
	}
	return out
}

// connModes is everybody's.
func (s *Server) connModes(ctx context.Context) ([]core.ConnMode, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := []core.ConnMode{}
	for _, u := range users {
		if u.Email == "" {
			continue
		}
		st, err := s.store.GetSettings(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, s.personModes(u.Email, st)...)
	}
	return out, nil
}

// reportModes sends the modes to Core when Day is connected to one
// (registered, so it has an app token). Serialized, and computed at send
// time, so the last report always carries the latest modes.
func (s *Server) reportModes(ctx context.Context) {
	if !s.core.Configured() || s.core.Token() == "" {
		return
	}
	s.modesMu.Lock()
	defer s.modesMu.Unlock()
	modes, err := s.connModes(ctx)
	if err != nil {
		s.log.Warn("pocket core: reading connection modes failed", "err", err)
		return
	}
	if err := s.core.ReportModes(ctx, modes); err != nil {
		s.log.Warn("pocket core: reporting connection modes failed", "err", err)
		return
	}
	s.log.Debug("pocket core: connection modes reported", "count", len(modes))
}

// reportModesSoon reports in the background, so saving settings never waits on Core.
func (s *Server) reportModesSoon() {
	if !s.core.Configured() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.reportModes(ctx)
	}()
}

// sameModes reports whether two lists from personModes say the same.
func sameModes(a, b []core.ConnMode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
