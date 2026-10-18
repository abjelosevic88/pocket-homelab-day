package server

import (
	"context"
	"encoding/json"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// adjustListening turns "position reached" into "minutes listened that day"
// for sources that only remember the last time an item was played (PinePods).
//
//   - An item the previous fetch of this day had, but this one does not, was
//     played again on a later day — it is kept here with what was heard then.
//   - Minutes for an item = position now − the furthest position recorded for
//     it on earlier days, so an episode spread over three evenings counts on
//     each of them instead of all at once on the last.
func (s *Server) adjustListening(ctx context.Context, date, source string, res *sources.Result) {
	a, ok := res.Payload.(sources.Activity)
	if !ok {
		return
	}
	seen := map[string]bool{}
	for _, it := range a.Items {
		seen[it.Ref] = true
	}
	if prev, err := s.store.FactsForDay(ctx, date); err == nil {
		if f, ok := prev[source]; ok && len(f.Payload) > 4 {
			var old sources.Activity
			if json.Unmarshal(f.Payload, &old) == nil {
				for _, it := range old.Items {
					if it.Ref != "" && !seen[it.Ref] {
						a.Items = append(a.Items, it) // moved on to a later day; keep what was heard here
						seen[it.Ref] = true
					}
				}
			}
		}
	}
	prior := map[string]float64{}
	if fs, err := s.store.FactsRange(ctx, store.AddDays(date, -60), store.AddDays(date, -1), source); err == nil {
		for _, f := range fs {
			var old sources.Activity
			if json.Unmarshal(f.Payload, &old) != nil {
				continue
			}
			for _, it := range old.Items {
				if it.Position > prior[it.Ref] {
					prior[it.Ref] = it.Position
				}
			}
		}
	}
	for i := range a.Items {
		it := &a.Items[i]
		if p, ok := prior[it.Ref]; ok && p < it.Position {
			it.Minutes = sources.Round1((it.Position - p) / 60)
		} else if ok && p >= it.Position {
			it.Minutes = 0 // replayed from earlier or nothing new
		}
	}
	a.Finish()
	res.Payload = a
	res.Headline = a.Minutes
	res.Summary = sources.ActivitySummary(a, "episode", "episodes")
}
