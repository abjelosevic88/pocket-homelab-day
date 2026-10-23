package sources

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMovementWhere(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, loc).Unix()
	raw := `{"summary":{"distance":12345,"moving_s":1800,"modes":{"driving":12000,"walking":345}},"entries":[
	 {"type":"visit","start_ts":` + jsonInt(day+8*3600) + `,"end_ts":` + jsonInt(day+9*3600) + `,"name":"Home","status":"confirmed","lat":44.77,"lng":17.2},
	 {"type":"journey","start_ts":` + jsonInt(day+9*3600) + `,"end_ts":` + jsonInt(day+9*3600+1800) + `,"distance":12345,"mode":"driving","start":[44.77,17.2],"end":[44.9,16.7]},
	 {"type":"visit","start_ts":` + jsonInt(day+10*3600) + `,"end_ts":` + jsonInt(day+11*3600) + `,"name":"Nope","status":"declined","lat":1,"lng":1}]}`
	var d movementDay
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	w := movementWhere(d, loc)
	if w.DistanceKm != 12.3 || w.MovingMinutes != 30 || w.Journeys != 1 || w.Places != 1 || len(w.Entries) != 2 || w.Provider != "movement" {
		t.Fatalf("where = %+v", w)
	}
	if w.Modes["driving"] != 12 || w.FirstMove != "09:00" || w.LastMove != "09:30" || w.Entries[0].Name != "Home" || w.Entries[0].Minutes != 60 {
		t.Fatalf("where = %+v", w)
	}
	if w.Bounds == nil || w.Bounds[0] != 44.77 || w.Bounds[3] != 17.2 || w.Bounds[1] != 16.7 {
		t.Fatalf("bounds = %v (declined visits must not count)", w.Bounds)
	}
	if s := whereSummary(w); s != "12.3 km · 30m moving · 1 place" && s == "" {
		t.Fatalf("summary %q", s)
	}
}

func jsonInt(v int64) string { b, _ := json.Marshal(v); return string(b) }
