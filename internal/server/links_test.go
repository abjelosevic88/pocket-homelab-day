package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// The routes each target allows a pcx_ token, exactly as the targets offer them.
var fakeLinkRoutes = map[string][]*regexp.Regexp{
	"history:read":     {regexp.MustCompile(`^/api/v1/listens(/dates)?$`)},
	"tasks:read":       {regexp.MustCompile(`^/api/v1/tasks$`)},
	"contacts:read":    {regexp.MustCompile(`^/api/v1/birthdays$`)},
	"projects:read":    {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"habits:read":      {regexp.MustCompile(`^/api/v1/habits(/\d+)?$`), regexp.MustCompile(`^/api/v1/days/\d{4}-\d\d-\d\d$`)},
	"health:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"mindfulness:read": {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"horoscope:read":   {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"home:read":        {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"renewals:read":    {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"wealth:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"pets:read":        {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"parcels:read":     {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"signing:read":     {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"places:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"voice:read":       {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"wallet:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"fitness:read":     {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"nutrition:read":   {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"blog:read":        {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"chat:read":        {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"forms:read":       {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"crm:read":         {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`), regexp.MustCompile(`^/api/v1/deals/\d+$`)},
	"league:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"tables:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"rentals:read":     {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`), regexp.MustCompile(`^/api/v1/today$`)},
	"trails:read":      {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`), regexp.MustCompile(`^/api/v1/summary$`), regexp.MustCompile(`^/api/v1/outings$`)},
	"books:read":       {regexp.MustCompile(`^/api/v1/books(/\d+)?$`), regexp.MustCompile(`^/api/v1/reading/sessions$`)},
	"trips:read":       {regexp.MustCompile(`^/api/v1/trips(/\d+)?$`)},
	"cooklog:read":     {regexp.MustCompile(`^/api/v1/cooklog$`)},
	"vehicles:read":    {regexp.MustCompile(`^/api/v1/vehicles(/\d+(/fuel|/records)?)?$`)},
	"puzzlecraft:read": {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"capsule:read":     {regexp.MustCompile(`^/api/v1/day/\d{4}-\d\d-\d\d$`)},
	"invoices:read":    {regexp.MustCompile(`^/api/v1/invoices(/\d+)?$`), regexp.MustCompile(`^/api/v1/payments$`)},
	"groups:read":      {regexp.MustCompile(`^/api/v1/groups(/\d+(/expenses)?)?$`), regexp.MustCompile(`^/api/v1/categories$`)},
	"activity:read":    {regexp.MustCompile(`^/api/v1/activity$`)},
	"documents:read": {regexp.MustCompile(`^/api/v1/(documents|dashboard|statistics|tags|correspondents|document_types|storage_paths|custom_fields|taxonomy|saved_views|changes)$`),
		regexp.MustCompile(`^/api/v1/documents/\d+(/download|/preview|/thumb|/text|/notes|/history|/similar|/suggestions|/pages/\d+(\.jpg)?)?$`), regexp.MustCompile(`^/api/v1/documents/next_asn$`)},
	"transactions:read": {regexp.MustCompile(`^/api/v1/transactions`), regexp.MustCompile(`^/firefly/api/v1/(about|about/user|accounts|categories|tags|tags/[^/]+|tags/[^/]+/transactions|transactions|transactions/\d+|currencies)$`)},
}

// The routes a pcx_ token may POST to, per scope (the rest are read-only).
var fakeLinkPosts = map[string][]*regexp.Regexp{
	"voice:transcribe": {regexp.MustCompile(`^/api/v1/transcribe$`)},
}

// linkWorld is a fake Pocket Core plus one fake target per Pocket app.
type linkWorld struct {
	t       *testing.T
	mu      sync.Mutex
	targets map[string]*httptest.Server // slug → server
	calls   map[string][]string         // slug → "Bearer … PATH"
	bad     []string                    // pcx_ calls outside the scope
	tokens  []string                    // "user app scopes" asked of Core
	off     map[string]string           // "app scope" → state (status and tokens refused)
	tokOff  map[string]bool             // "app scope" → only the token is refused
	tokDown bool                        // link tokens answer 503 (Core restarting)
	coreURL string
	reg     json.RawMessage // the last manifest Day registered

	// third-party services (see services_test.go)
	svcs        map[string]*httptest.Server // type → fake service
	svcCalls    map[string][]string         // type → "METHOD PATH?QUERY auth=…"
	svcAsked    []string                    // "user type" credentials asked of Core
	svcOff      map[string]string           // "user type" → state (status and credential refused)
	svcCredOff  map[string]bool             // "user type" → only the credential is refused
	svcPersonal map[string]bool             // "user type" → the person has their own key in Core
	svcDown     bool                        // credentials answer 503 (Core restarting)

	modes  []map[string]string // the last PUT /api/v1/links/modes
	nModes int                 // how many reports came in
}

const day0 = "2026-09-01"

func newLinkWorld(t *testing.T) *linkWorld {
	w := &linkWorld{t: t, targets: map[string]*httptest.Server{}, calls: map[string][]string{}, off: map[string]string{}, tokOff: map[string]bool{},
		svcs: map[string]*httptest.Server{}, svcCalls: map[string][]string{}, svcOff: map[string]string{}, svcCredOff: map[string]bool{}, svcPersonal: map[string]bool{}}
	for _, n := range serviceSourceOrder {
		typ := serviceSources[n].typ
		srv := httptest.NewServer(w.service(typ))
		t.Cleanup(srv.Close)
		w.svcs[typ] = srv
	}
	for _, n := range pocketSourceOrder {
		slug := pocketSources[n].slug
		srv := httptest.NewServer(w.target(slug))
		t.Cleanup(srv.Close)
		w.targets[slug] = srv
	}
	c := httptest.NewServer(http.HandlerFunc(w.core))
	t.Cleanup(c.Close)
	w.coreURL = c.URL
	return w
}

func (w *linkWorld) core(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	w.mu.Lock()
	defer w.mu.Unlock()
	switch r.URL.Path {
	case "/api/v1/apps/register":
		var in struct {
			Manifest json.RawMessage `json:"manifest"`
		}
		b, _ := io.ReadAll(r.Body)
		w.reg = b
		_ = json.Unmarshal(b, &in)
		_, _ = rw.Write([]byte(`{"app":{}}`))
	case "/api/v1/links/status":
		user := r.URL.Query().Get("user")
		out := []map[string]any{}
		for _, n := range pocketSourceOrder {
			ps := pocketSources[n]
			for _, scope := range append([]string{ps.scope}, ps.more...) {
				state := "on"
				if user != "owner@x" {
					state = "not_in_audience"
				} else if s, ok := w.off[ps.slug+" "+scope]; ok {
					state = s
				}
				out = append(out, map[string]any{"app": ps.slug, "scope": scope, "state": state, "on": state == "on"})
			}
		}
		for _, extra := range []pocketSource{movementApp, weatherApp} {
			if w.targets[extra.slug] == nil { // only in the tests that add a Pocket Movement (or Weather)
				continue
			}
			state := "on"
			if user != "owner@x" {
				state = "not_in_audience"
			} else if s, ok := w.off[extra.slug+" "+extra.scope]; ok {
				state = s
			}
			out = append(out, map[string]any{"app": extra.slug, "scope": extra.scope, "state": state, "on": state == "on"})
		}
		for _, n := range serviceSourceOrder {
			state := w.serviceState(user, serviceSources[n].typ)
			out = append(out, map[string]any{"app": "service:" + serviceSources[n].typ, "scope": "account", "state": state, "on": state == "on"})
		}
		_ = json.NewEncoder(rw).Encode(out)
	case "/api/v1/services/credential":
		if r.Header.Get("Authorization") != "Bearer pca_day" || r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			User string `json:"user"`
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.svcAsked = append(w.svcAsked, in.User+" "+in.Type)
		if w.svcDown {
			rw.WriteHeader(http.StatusServiceUnavailable)
			_, _ = rw.Write([]byte(`{"error":"restarting"}`))
			return
		}
		state := w.serviceState(in.User, in.Type)
		if state == "on" && w.svcCredOff[in.User+" "+in.Type] {
			state = "off_person"
		}
		if state != "on" {
			rw.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(rw).Encode(map[string]any{"error": in.Type + ": refused", "reason": state})
			return
		}
		source, key := "box", "box-"+in.Type
		if w.svcPersonal[in.User+" "+in.Type] {
			source, key = "personal", "own-"+in.Type+"-"+strings.SplitN(in.User, "@", 2)[0]
		}
		fields := map[string]string{}
		switch in.Type {
		case "maloja": // keyless: the address alone
			key = ""
		case "nextcloud":
			fields["USERNAME"] = strings.SplitN(in.User, "@", 2)[0] + "-nc"
			fields["APP_PASSWORD"] = key
		case "audiobookshelf", "forgejo":
			fields["TOKEN"] = key
		default:
			fields["API_KEY"] = key
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"type": in.Type, "name": in.Type, "url": w.svcs[in.Type].URL, "public_url": "https://" + in.Type + ".example", "key": key, "fields": fields, "source": source})
	case "/api/v1/links/modes":
		var in struct {
			Modes []map[string]string `json:"modes"`
		}
		if r.Method != http.MethodPut || r.Header.Get("Authorization") != "Bearer pca_day" || json.NewDecoder(r.Body).Decode(&in) != nil {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		w.modes, w.nModes = in.Modes, w.nModes+1
		_, _ = fmt.Fprintf(rw, `{"recorded":%d}`, len(in.Modes))
	case "/api/v1/links/token":
		if r.Header.Get("Authorization") != "Bearer pca_day" {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			User   string   `json:"user"`
			App    string   `json:"app"`
			Scopes []string `json:"scopes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		key := in.App + " " + strings.Join(in.Scopes, ",")
		w.tokens = append(w.tokens, in.User+" "+key)
		if w.tokDown {
			rw.WriteHeader(http.StatusServiceUnavailable)
			_, _ = rw.Write([]byte(`{"error":"restarting"}`))
			return
		}
		if s, ok := w.off[key]; ok || w.tokOff[key] || in.User != "owner@x" {
			if s == "" {
				s = "off_person"
			}
			rw.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(rw).Encode(map[string]any{"error": "link is off", "reason": s})
			return
		}
		srv := w.targets[in.App]
		if srv == nil {
			rw.WriteHeader(http.StatusForbidden)
			_, _ = rw.Write([]byte(`{"error":"no such app","reason":"target_missing"}`))
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"access_token": "pcx_" + in.App + "_" + strings.Join(in.Scopes, ","), "expires_in": 300, "url": srv.URL, "public_url": "https://" + in.App + ".example"})
	default:
		http.NotFound(rw, r)
	}
}

// serviceState is a service link's state for a person: the owner (an admin)
// gets the box's key, a member only their own.
func (w *linkWorld) serviceState(user, typ string) string {
	if s, ok := w.svcOff[user+" "+typ]; ok {
		return s
	}
	switch {
	case user == "owner@x" || w.svcPersonal[user+" "+typ]:
		return "on"
	case user == "member@x":
		return "no_account"
	}
	return "unknown_user"
}

// service is one fake third-party service: it records how it was called and
// answers the paths Day's Test and (Immich, Maloja, Audiobookshelf) Fetch read.
func (w *linkWorld) service(typ string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		auth := firstNonEmpty(r.Header.Get("x-api-key"), r.Header.Get("x-api-token"), r.Header.Get("Api-Key"), r.Header.Get("Authorization"))
		if u, p, ok := r.BasicAuth(); ok {
			auth = "basic " + u + ":" + p
		}
		w.mu.Lock()
		w.svcCalls[typ] = append(w.svcCalls[typ], r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" auth="+auth)
		w.mu.Unlock()
		write := func(s string) { _, _ = rw.Write([]byte(s)) }
		switch p := r.URL.Path; {
		case p == "/api/server/version":
			write(`{"major":1,"minor":2,"patch":3}`)
		case p == "/api/search/metadata":
			write(`{"assets":{"items":[{"id":"a1","type":"IMAGE","localDateTime":"` + day0 + `T10:00:00.000Z"}],"nextPage":null}}`)
		case strings.HasPrefix(p, "/api/assets/"):
			rw.Header().Set("Content-Type", "image/jpeg")
			write("jpeg")
		case p == "/apis/mlj_1/numscrobbles":
			write(`{"status":"ok","amount":5}`)
		case p == "/apis/mlj_1/scrobbles":
			write(`{"status":"ok","list":[],"pagination":{"next_page":null}}`)
		case p == "/api/v1/timeline":
			write(`{"days":[]}`)
		case strings.HasPrefix(p, "/api/v1/users/") && strings.HasSuffix(p, "/activities/feeds"):
			write(`[]`)
		case strings.HasPrefix(p, "/api/data/user_history/"):
			write(`{"data":[]}`)
		case p == "/api/v1/health":
			write(`{"status":"ok"}`)
		case p == "/api/getHistory":
			write(`{"current_page":1,"pages":1,"results":[]}`)
		case p == "/api/data/get_user":
			write(`{"retrieved_id":2}`)
		case p == "/api/data/get_stats":
			write(`{"PodcastsPlayed":1,"PodcastsAdded":2}`)
		case p == "/api/sessions" || p == "/api/me/listening-sessions":
			write(`{"total":3,"numPages":1,"sessions":[{"userId":"u1","libraryItemId":"li1","mediaType":"book","displayTitle":"Dune","date":"` + day0 + `","timeListening":600,"duration":3600}]}`)
		case p == "/api/v1/request":
			write(`{"pageInfo":{"results":4}}`)
		case p == "/api/v1/user":
			write(`{"login":"owner"}`)
		case p == "/api/v1/version":
			write(`{"version":"9.0"}`)
		case strings.HasPrefix(p, "/remote.php/dav/calendars/"):
			rw.WriteHeader(207)
			write(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:"></d:multistatus>`)
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	})
}

func (w *linkWorld) svcCallCount(typ string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.svcCalls[typ])
}

func (w *linkWorld) svcLastCall(typ string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.svcCalls[typ]) == 0 {
		return ""
	}
	return w.svcCalls[typ][len(w.svcCalls[typ])-1]
}

// target is one fake Pocket app: it answers the paths Day reads and refuses a
// pcx_ token outside its scope.
func (w *linkWorld) target(slug string) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/healthz" {
			_, _ = rw.Write([]byte(`{"status":"ok","version":"9.9"}`))
			return
		}
		auth := r.Header.Get("Authorization")
		w.mu.Lock()
		w.calls[slug] = append(w.calls[slug], auth+" "+r.URL.Path)
		w.mu.Unlock()
		tok := strings.TrimPrefix(auth, "Bearer ")
		switch {
		case strings.HasPrefix(tok, "pcx_"+slug+"_"):
			scope := strings.TrimPrefix(tok, "pcx_"+slug+"_")
			ok := r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me"
			for _, re := range fakeLinkRoutes[scope] {
				ok = ok || (r.Method == http.MethodGet && re.MatchString(r.URL.Path))
			}
			for _, re := range fakeLinkPosts[scope] {
				ok = ok || (r.Method == http.MethodPost && re.MatchString(r.URL.Path))
			}
			if !ok {
				w.mu.Lock()
				w.bad = append(w.bad, slug+" "+scope+" "+r.Method+" "+r.URL.Path)
				w.mu.Unlock()
				rw.WriteHeader(http.StatusForbidden)
				_, _ = rw.Write([]byte(`{"error":"outside the link's scope"}`))
				return
			}
		case tok == "direct-"+slug:
		default:
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		d := day0
		p := r.URL.Path
		write := func(s string) { _, _ = rw.Write([]byte(s)) }
		switch {
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "health":
			write(`{"date":"` + d + `","metrics":{"steps":{"value":9123,"unit":"count","name":"Steps"},"sleep_asleep":{"value":408,"unit":"min","name":"Time asleep"}},"workouts":[{"id":1,"kind":"running","name":"Running","start_at":"` + d + `T07:00:00Z","duration_s":2100,"distance_m":5200,"energy_kcal":310,"avg_hr":151}],"sleep":[{"start_at":"` + d + `T23:10:00Z","end_at":"` + d + `T06:40:00Z","asleep_s":24480,"in_bed_s":27000,"deep_s":3960,"rem_s":5040}],"goals_met":["steps"],"summary":"9123 steps · slept 6h 48m"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "still":
			write(`{"date":"` + d + `","minutes":15,"sessions":2,"streak":4,"goal_min":10,"goal_met":true,"practices":[{"id":7,"kind":"breath","kind_name":"Breathing","title":"Box breathing","started_at":"` + d + `T07:10:00Z","minutes":5,"calm_before":2,"calm_after":4},{"id":8,"kind":"meditation","kind_name":"Meditation","title":"Silent sit","started_at":"` + d + `T21:00:00Z","minutes":10}],"lessons":[{"program":"The Path","stage":"Arrive","title":"Just three minutes"}],"milestones":["A full week"],"summary":"15 min of practice: box breathing, silent sit (day 4 of a streak)"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "home":
			write(`{"date":"` + d + `","records":[{"id":3,"kind":"repair","title":"Replaced the kitchen tap","thing_name":"Kitchen sink","home_name":"House","pro_name":"Marko","cost":8900,"currency":"EUR"}],"readings":[{"meter_id":1,"meter":"Electricity","kind":"electricity","unit":"kWh","value":48230.5,"usage":312.4,"days":31}],"due":[]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "wealth":
			write(`{"date":"` + d + `","items":[{"type":"activity","id":3,"kind":"buy","title":"Bought 2.5 Vanguard FTSE All-World","amount":30000,"currency":"EUR","asset_id":4,"asset":"Broker account","url":"https://wealth.x/a/4"},{"type":"valuation","id":9,"title":"Pension updated","amount":1960000,"currency":"EUR","asset_id":6,"asset":"Pension","url":"https://wealth.x/a/6"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "pets":
			write(`{"date":"` + d + `","records":[{"id":11,"pet_id":1,"pet_name":"Luna","pet_species":"dog","kind":"vet_visit","title":"Limping on the front left leg","notes":"","valid_until":"","cost":4500,"currency":"BAM","pro_name":"Dr. Hodžić","care_title":"","date":"` + d + `"},{"id":12,"pet_id":2,"pet_name":"Mici","pet_species":"cat","kind":"purchase","title":"Food","cost":6900,"currency":"BAM","date":"` + d + `"}],"weights":[{"id":4,"pet_id":2,"pet_name":"Mici","weighed_on":"` + d + `","kg":4.9,"change":-0.04}],"due":[],"birthdays":[{"pet_id":1,"name":"Luna","turns":5}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "sign":
			write(`{"sent":[{"id":1,"title":"NDA — Acme","sent_at":"` + d + `T08:00:00Z","recipients":[{"name":"Ana Kovač","email":"ana@example.com","role":"signer"}]}],"completed":[{"id":3,"title":"Ugovor o zakupu","completed_at":"` + d + `T14:30:00Z"}],"signed":[{"document_id":3,"title":"Ugovor o zakupu","name":"Marko Marić","at":"` + d + `T14:30:00Z"}],"signed_by_me":[]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "places":
			write(`{"date":"` + d + `","visits":[{"id":1,"place_id":1,"date":"` + d + `","time":"19:30","planned":false,"rating":5,"notes":"","with_whom":"Ana","minutes":0,"spent":6800,"currency":"BAM","place_name":"Kibe Mahala","place_kind":"restaurant","city":"Sarajevo","trip_name":""}],"planned":[],"added":[{"id":1,"name":"Kibe Mahala","kind":"restaurant","cuisine":"Bosnian","city":"Sarajevo","status":"been"},{"id":5,"name":"Zlatna Ribica","kind":"bar","cuisine":"","city":"Sarajevo","status":"want"}],"spent":[{"currency":"BAM","total":6800,"visits":1}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "fitness":
			write(`{"date":"` + d + `","workouts":[{"id":96,"name":"Pull day","started_at":"` + d + `T16:55:00Z","duration_min":72,"sets":15,"volume_kg":5072.5,"records":2,"feel":4,"notes":"","url":"https://fitness.example.org/workouts/96","exercises":[{"exercise":"Deadlift (Barbell)","exercise_id":10,"records":3,"sets":"115 kg × 5, 115 kg × 5, 115 kg × 5"}]}],"other":[{"id":7,"name":"Morning run","kind":"running","minutes":31,"distance_m":5200,"health":true}],"planned":null,"text":"Pull day (72 min, 15 sets); Morning run (31 min)."}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "chat":
			write(`{"date":"` + d + `","messages":12,"mine":3,"photos":2,"voice":0,"conversations":[{"conversation_id":2,"title":"Family","messages":8,"mine":2},{"conversation_id":4,"title":"Weekend hikers","messages":4,"mine":1}],"plans":[{"conversation":"Family","from":"Ana","text":"Dinner at Pirpa at 20:00?","date":"` + d + `","time":"20:00","message_id":91,"conversation_id":2}],"shared":[{"conversation":"Family","from":"Ana","app":"kitchen","title":"Grandma's stuffed peppers","subtitle":"Dinner · 4 servings","message_id":95,"conversation_id":2}],"summary":"12 messages, 3 by you: Family (8), Weekend hikers (4); 2 photos"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "crm":
			write(`{"date":"` + d + `","activities":[{"id":5,"deal_id":3,"kind":"won","title":"Won 🎉","body":"","at":"` + d + `T09:15:00Z","deal":"Online shop for cakes","deal_url":"https://crm.example.org/deals/3"},{"id":6,"deal_id":4,"kind":"call","title":"Talked through the pages","body":"She wants a booking calendar and a gallery of the rooms, and asked about the price of a second language","at":"` + d + `T11:00:00Z","deal":"Booking site","deal_url":"https://crm.example.org/deals/4"}],"counts":{"won":1,"call":1},"due":[{"id":4,"title":"Booking site","stage_name":"Proposal sent","status":"open","amount":4500,"currency":"EUR","contact_name":"Ana Marić","company":"Pansion Una","next_action":"Send the proposal","next_due":"` + d + `","url":"https://crm.example.org/deals/4"},{"id":7,"title":"Logo","next_action":"Call back","next_due":"` + d + `","url":"https://crm.example.org/deals/7"},{"id":8,"title":"Menu cards","next_action":"","next_due":"` + d + `","url":"https://crm.example.org/deals/8"}],"won":1,"lost":0,"created":0,"url":"https://crm.example.org/"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "tables":
			write(`{"date":"` + d + `","items":[{"id":12,"title":"Ana Marić","table":"Clients","table_icon":"👥","base":"Bakery","field":"Visit","date":"` + d + `T14:30","url":"https://tables.example.org/r/12"},{"id":7,"title":"Logo for the bakery","table":"Projects","base":"Work","field":"Due","date":"` + d + `","url":"https://tables.example.org/r/7"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "rentals":
			write(`{"date":"` + d + `","items":[{"kind":"payment","title":"Rent in: €650.00","subtitle":"Flat 2 — Ana Marić","url":"https://rentals.example.org/leases/3"},{"kind":"repair","title":"Repair reported: Leaking tap","subtitle":"Flat 1, Obala 12","url":"https://rentals.example.org/repairs/9"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "puzzlecraft":
			write(`{"date":"` + d + `","count":2,"minutes":5,"items":[{"time":"08:10","title":"Puzzle of the day: Sudoku","sub":"You solved it in 5:12","kind":"sudoku","mine":true},{"time":"20:00","title":"Monday mini","sub":"Ana solved it in 3:40","kind":"crossword","mine":false}],"planned":[]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "capsule":
			write(`{"date":"` + d + `","items":[{"time":"09:30","kind":"sealed","text":"Sealed “Birthday letter” (opens 20 Nov 2026)","emoji":"🎂","capsule_id":3,"url":"https://capsule.example.org/c/3"},{"kind":"added","text":"Put 2 things in “Summer 2026”","emoji":"☀️","capsule_id":4,"url":"https://capsule.example.org/c/4"}],"text":"","url":"https://capsule.example.org/"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "trails":
			write(`{"date":"` + d + `","count":2,"outings":[{"id":2,"title":"Hike: Trebević from Sarajevo","status":"done","activity":"hike","start_time":"08:00","distance_m":12400,"gain_m":980,"minutes":270,"rating":5,"people":"Ana","text":"12.4 km · 980 m up · 4 h 30 min · with Ana","route":"Trebević from Sarajevo","url":"https://trails.example.org/outings/2"},{"id":3,"title":"Run by the river","status":"planned","activity":"run","start_time":"18:00","text":"8.0 km · 20 m up · 45 min","url":"https://trails.example.org/outings/3"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "league":
			write(`{"date":"` + d + `","games":[{"id":31,"title":"Tennis · Tuesday Tennis","group":"Tuesday Tennis","emoji":"🎾","time":"20:00","venue":"TK Koševo","status":"planned","my_answer":"in","in":4,"url":"https://league.example.org/games/31"},{"id":32,"title":"Futsal / 5-a-side · Friday 5-a-side","group":"Friday 5-a-side","emoji":"⚽","time":"19:00","venue":"Dvorana Mojmilo","status":"planned","my_answer":"","in":8,"url":"https://league.example.org/games/32"}],"matches":[{"id":40,"group":"Tuesday Tennis","emoji":"🎾","text":"Aleksandar beat Marko Petrović 6–4 6–3","outcome":"W","url":"https://league.example.org/games/31"}]}`)
		case strings.HasPrefix(p, "/api/v1/deals/") && slug == "crm":
			write(`{"deal":{"id":3,"title":"Online shop for cakes","company":"Pekara Marić","status":"won"},"activities":[],"made":[],"url":"https://crm.example.org/deals/3"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "forms":
			write(`{"date":"` + d + `","count":2,"summary":"2 answers to your forms","responses":[{"id":41,"time":"09:12","form":"Client brief","form_id":2,"emoji":"📋","name":"Ana Kovač","preview":"A new website for the bakery","number":7,"url":"https://forms.example.org/answers/41","status":"new"},{"id":42,"time":"11:40","form":"RSVP","form_id":3,"emoji":"🎉","name":"Marko","preview":"Yes, two of us","number":3,"url":"https://forms.example.org/answers/42","status":"seen"}],"opened":[{"id":9,"title":"Summer party RSVP","emoji":"🎉","url":"https://forms.example.org/f/summer-party"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "blog":
			write(`{"date":"` + d + `","published":[{"id":12,"title":"Spring in Sarajevo","kind":"post","status":"published","url":"https://blog.example.org/spring-in-sarajevo","words":1240,"excerpt":"The river was loud.","tags":["travel"],"blog":"Notes","time":"09:30"}],"written":[{"id":13,"title":"Notes on rain","kind":"post","status":"draft","url":"https://blog.example.org/p/13?preview=k","words":80,"excerpt":"","tags":[],"blog":"Notes","time":"21:10"}],"views":312,"comments":4}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "nutrition":
			write(`{"date":"` + d + `","meals":[{"meal":"breakfast","label":"Breakfast","totals":{"kcal":520.4,"protein":18,"carbs":80,"fat":12},"entries":[{"id":1,"name":"Oats","kcal":380,"created_at":"` + d + `T06:40:00Z"},{"id":2,"name":"Banana","kcal":105,"created_at":"` + d + `T06:41:00Z"},{"id":3,"name":"Coffee","kcal":35.4,"created_at":"` + d + `T06:41:30Z"}]},{"meal":"lunch","label":"Lunch","totals":{"kcal":0},"entries":[]},{"meal":"dinner","label":"Dinner","totals":{"kcal":1329.6,"protein":94,"carbs":110,"fat":52},"entries":[{"id":4,"name":"Ćevapi","kcal":1329.6,"created_at":"` + d + `T18:30:00Z"}]},{"meal":"snacks","label":"Snacks","totals":{"kcal":0},"entries":[]}],"totals":{"kcal":1850,"protein":112.3,"carbs":190,"fat":64},"entries":4,"water_ml":1500,"goals":{"kcal":2300,"protein":140,"carbs":250,"fat":75,"fiber":30,"water":2000,"burned":0,"budget":2300},"remaining":{"kcal":450},"note":"Ate late.","text":"1,850 kcal of 2,300; protein 112.3 g, carbs 190 g, fat 64 g. Breakfast: oats, banana, coffee. Dinner: ćevapi."}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "wallet":
			write(`{"date":"` + d + `","passes":[{"id":14,"kind":"ticket","kind_label":"Ticket","name":"Dubioza kolektiv","issuer":"Entrio","path":"/cards/14","starts_at":"` + d + `T20:00","venue":"Zetra Olympic Hall","has_code":true}],"url":"https://wallet.example.org"}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "voice":
			write(`{"date":"` + d + `","memos":[{"id":12,"title":"Dentist, dry cleaning and photos for Amra","kind":"todo","recorded_at":"` + d + `T08:12:00Z","time":"08:12","duration_ms":21000,"summary":"Book the dentist, pick up the dry cleaning, send Amra the photos.","text":"Remind me to book the dentist…","status":"done","sent":[{"app":"tasks","kind":"task","ref":"41","title":"Book the dentist","url":"https://tasks.x/t/41","created_at":"` + d + `T08:13:00Z"},{"app":"tasks","kind":"task","ref":"42","title":"Pick up the dry cleaning","url":"https://tasks.x/t/42","created_at":"` + d + `T08:13:00Z"}],"url":"https://voice.x/memos/12"},{"id":13,"title":"","kind":"note","recorded_at":"` + d + `T09:30:00Z","time":"09:30","duration_ms":4000,"summary":"","text":"","status":"working","sent":[],"url":"https://voice.x/memos/13"}],"count":2,"duration_ms":25000}`)
		case p == "/api/v1/transcribe" && slug == "voice":
			f, _, err := r.FormFile("file")
			if err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				write(`{"error":"no recording in field \"file\""}`)
				return
			}
			b, _ := io.ReadAll(f)
			if string(b) == "silence" {
				rw.WriteHeader(http.StatusUnprocessableEntity)
				write(`{"error":"nothing was said in the recording"}`)
				return
			}
			_ = json.NewEncoder(rw).Encode(map[string]any{"text": "Heard: " + string(b) + " (" + r.FormValue("language") + ")", "language": "bs", "segments": []any{}, "engine": "whisper base"})
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "parcels":
			write(`{"date":"` + d + `","delivered":[{"id":5,"title":"Running shoes","shop":"Decathlon","carrier_name":"GLS","direction":"in","delivered_at":"` + d + `T13:20:00Z","days":3,"price":7999,"currency":"EUR"}],"sent":[{"id":6,"title":"Birthday present for Ana","shop":"Ana","direction":"out","ordered_on":"` + d + `"}],"added":[{"id":7,"title":"USB-C hub","shop":"AliExpress","direction":"in","ordered_on":"2026-01-01"}],"expected":[],"steps":[{"parcel_id":8,"title":"Phone case","at":"` + d + `T09:00:00Z","status":"pickup","description":"Ready for pick-up","location":"Pošta Sarajevo"},{"parcel_id":5,"title":"Running shoes","at":"` + d + `T08:00:00Z","status":"out_for_delivery","description":"Out for delivery"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "renewals":
			write(`{"date":"` + d + `","expires":[{"id":12,"title":"ID card","kind":"id_card","holder":"Luka","provider":"","expires_on":"` + d + `","status":"expired","open_url":"https://renewals.x/r/12"}],"renewed":[{"id":4,"renewal_id":7,"title":"Car insurance","renewed_on":"` + d + `","old_expires":"2026-01-05","new_expires":"2027-01-05","cost":41200,"currency":"EUR","note":"","source":"manual"}]}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "horoscope":
			write(`{"date":"` + d + `","url":"https://horoscope.x/?date=` + d + `","moon":{"sign":"taurus","phase":"full","illumination":0.99},"events":[{"kind":"eclipse","time":"` + d + `T20:12:00+02:00","title":"Total lunar eclipse in Taurus","icon":"🌘"}],"headline":"A day to get moving","summary":"x","scores":{"love":60,"work":70,"mood":55,"energy":80},"overall":66,"transits":["Venus trine your Sun"],"journal":{"mood":4,"energy":3,"note":"fine","tags":["work"]}}`)
		case strings.HasPrefix(p, "/api/v1/day/") && slug == "projects":
			write(`{"issues":[{"identifier":"AS-6","title":"Rules","url":"https://projects.x/issue/AS-6","project":"Relaunch","done":true,"minutes":80,"first":"10:00","last":"15:00","summary":"done (Done) · 1 commit","events":[{"time":"10:00","kind":"created","text":"Created the issue"}]}],"projects":[]}`)
		case p == "/api/v1/birthdays":
			write(`[{"contact":{"id":12,"fn":"Marko"},"kind":"birthday","date":"` + d + `","age":38}]`)
		case p == "/api/v1/tasks":
			write(`{"tasks":[{"id":1,"title":"Fix the shelf","completed_at":"` + d + `T12:00:00Z"}],"total":1}`)
		case p == "/api/v1/auth/me":
			write(`{"id":1,"email":"owner@x"}`)
		case strings.HasPrefix(p, "/api/v1/days/"):
			write(`{"done":2,"total":3,"journal":"good day","habits":[],"trackers":[]}`)
		case p == "/api/v1/listens":
			write(`{"listens":[{"episode_id":1,"title":"Episode","podcast":"Show","seconds":120}]}`)
		case p == "/api/v1/reading/sessions":
			write(`{"sessions":[{"book_id":1,"title":"Dune","start":"` + d + `T20:00:00Z","end":"` + d + `T20:30:00Z","minutes":30,"pages":25}]}`)
		case p == "/api/v1/books":
			write(`[{"id":1,"title":"Dune","status":"finished","finished_on":"` + d + `"}]`)
		case p == "/api/v1/trips":
			write(`[{"id":1,"name":"Rome","start_date":"2026-08-30","end_date":"2026-09-03"}]`)
		case p == "/api/v1/cooklog":
			write(`[{"id":1,"date":"` + d + `","recipe_title":"Pasta"}]`)
		case p == "/api/v1/vehicles":
			write(`[{"id":7,"name":"Fabia"}]`)
		case p == "/api/v1/vehicles/7/fuel":
			write(`[{"date":"` + d + `","volume":30,"cost":7000,"currency":"BAM"}]`)
		case p == "/api/v1/vehicles/7/records":
			write(`[]`)
		case p == "/api/v1/invoices":
			write(`[{"number":"2026-1","client_name":"ACME","status":"sent","issue_date":"` + d + `","total":100,"currency":"EUR"}]`)
		case p == "/api/v1/payments":
			write(`[]`)
		case p == "/api/v1/groups":
			write(`[{"id":3,"name":"Flat","currency":"EUR"}]`)
		case p == "/api/v1/groups/3":
			write(`{"members":[{"id":1,"name":"Ana"}]}`)
		case p == "/api/v1/groups/3/expenses":
			write(`[{"id":1,"title":"Groceries","amount":2000,"currency":"EUR","date":"` + d + `","paid_by":1}]`)
		case p == "/api/v1/activity":
			write(`[{"item_name":"Milk","delta":-1,"created_at":"` + d + `T10:00:00Z"}]`)
		case p == "/api/v1/documents":
			write(`{"results":[{"id":12,"title":"Water bill","added_at":"` + d + `T12:00:00Z","source":"mail","correspondent_name":"ViK","document_type_name":"Invoice"}],"count":1}`)
		case p == "/firefly/api/v1/about":
			write(`{"data":{"version":"6.7.9","php_version":"pocket-money 0.4.0"}}`)
		case p == "/firefly/api/v1/transactions":
			write(`{"data":[{"id":"1","attributes":{"transactions":[{"transaction_journal_id":"1","type":"withdrawal","date":"` + d + `T12:00:00+00:00","description":"Konzum","amount":"12.50","currency_code":"BAM"}]}}],"meta":{"pagination":{"total_pages":1}}}`)
		default:
			rw.WriteHeader(http.StatusNotFound)
			write(`{"error":"not found"}`)
		}
	})
}

func (w *linkWorld) callCount(slug string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.calls[slug])
}

func (w *linkWorld) lastCall(slug string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.calls[slug]) == 0 {
		return ""
	}
	return w.calls[slug][len(w.calls[slug])-1]
}

type linkEnv struct {
	*linkWorld
	s                   *Server
	st                  *store.Store
	ts                  *httptest.Server
	owner, member       *store.User
	ownerTok, memberTok string
	cfg                 *config.Config
}

func newLinkEnv(t *testing.T, withCore bool) *linkEnv {
	t.Helper()
	ctx := context.Background()
	w := newLinkWorld(t)
	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.Sources = map[string]config.SourceEnv{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(cfg, st, log, nil)
	if err := s.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if withCore {
		// ServiceCache -1: Core is asked on every use, so the tests see each
		// ask; TestServiceCredentialCache covers the SDK's one-minute cache.
		s.core = core.New(w.coreURL, core.Options{DB: s.store.DB, Token: "pca_day", ServiceCache: -1})
	} else {
		s.core = core.New("", core.Options{DB: s.store.DB})
	}
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	e := &linkEnv{linkWorld: w, s: s, st: st, ts: ts, cfg: cfg}
	e.owner = &store.User{Email: "owner@x", Name: "Owner", Role: "admin"}
	e.member = &store.User{Email: "member@x", Name: "Member", Role: "member"}
	_ = st.CreateUser(ctx, e.owner)
	_ = st.CreateUser(ctx, e.member)
	e.ownerTok, _, _ = st.CreateAPIToken(ctx, e.owner.ID, "owner")
	e.memberTok, _, _ = st.CreateAPIToken(ctx, e.member.ID, "member")
	return e
}

func (e *linkEnv) do(tok, method, path string, body any) (int, string) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, e.ts.URL+"/api/v1"+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

type settingsView struct {
	CoreConfigured bool                    `json:"core_configured"`
	Sources        map[string]publicSource `json:"sources"`
}

func (e *linkEnv) settings(tok string) settingsView {
	e.t.Helper()
	_, body := e.do(tok, "GET", "/settings", nil)
	var v settingsView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		e.t.Fatalf("settings: %v %s", err, body)
	}
	return v
}

func (e *linkEnv) facts(u *store.User, date string) map[string]store.Fact {
	f, err := e.st.FactsForDay(context.Background(), u.ID, date)
	if err != nil {
		e.t.Fatal(err)
	}
	return f
}

func TestManifestUsesEveryPocketSource(t *testing.T) {
	e := newLinkEnv(t, true)
	if err := e.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Uses []core.Use `json:"uses"`
	}
	if err := json.Unmarshal(e.reg, &m); err != nil || len(m.Uses) == 0 {
		// the SDK may wrap the manifest
		var wrap struct {
			Manifest struct {
				Uses []core.Use `json:"uses"`
			} `json:"manifest"`
		}
		_ = json.Unmarshal(e.reg, &wrap)
		m.Uses = wrap.Manifest.Uses
	}
	got := []string{}
	for _, u := range m.Uses {
		if u.Reason == "" {
			t.Errorf("use %s has no reason", u.App)
		}
		got = append(got, u.App+"="+strings.Join(u.Scopes, ","))
	}
	sort.Strings(got)
	want := "ai=inference:write audiobooks=listening:read blog=blog:read bookshelf=books:read capsule=capsule:read chat=chat:read contacts=contacts:read crm=crm:read documents=documents:read fitness=fitness:read forms=forms:read garage=vehicles:read habits=habits:read health=health:read home=home:read horoscope=horoscope:read inventory=activity:read invoicing=invoices:read kitchen=cooklog:read league=league:read money=transactions:read movement=movement:read music=listening:read nutrition=nutrition:read parcels=parcels:read pets=pets:read places=places:read podcasts=history:read projects=projects:read puzzlecraft=puzzlecraft:read renewals=renewals:read rentals=rentals:read requests=requests:read sign=signing:read split=groups:read still=mindfulness:read tables=tables:read tasks=tasks:read trails=trails:read travel=trips:read voice=voice:read,voice:transcribe wallet=wallet:read watch=watching:read wealth=wealth:read weather=weather:read"
	if strings.Join(got, " ") != want {
		t.Fatalf("uses\n got %s\nwant %s\n(%s)", strings.Join(got, " "), want, e.reg)
	}
}

// With Pocket Core and nothing saved, every Pocket app source comes through a
// link: a fresh pcx_ token per fetch, with the one scope each target offers.
func TestPocketSourcesThroughCore(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	v := e.settings(e.ownerTok)
	if !v.CoreConfigured {
		t.Fatal("core not configured")
	}
	for _, n := range pocketSourceOrder {
		ps := v.Sources[n]
		if !ps.Pocket || ps.Mode != "core" || ps.Via != "core" || !ps.Configured {
			t.Errorf("%s: %+v", n, ps)
		}
	}
	if v.Sources["firefly"].App != "Pocket Money" || v.Sources["dawarich"].Pocket || !v.Sources["dawarich"].Service || v.Sources["weather"].Mode != "" {
		t.Errorf("labels: %+v / %+v", v.Sources["firefly"], v.Sources["dawarich"])
	}

	errs, ran := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if !ran || len(errs) != 0 {
		t.Fatalf("pull: ran=%v errs=%v", ran, errs)
	}
	facts := e.facts(e.owner, day0)
	for _, n := range pocketSourceOrder {
		if f, ok := facts[n]; !ok || f.Error != "" || f.Summary == "" {
			t.Errorf("fact %s: %+v", n, f)
		}
	}
	if f := facts["horoscope"]; f.Headline != 66 || !strings.Contains(f.Summary, "Full Moon in Taurus") || !strings.Contains(f.Summary, "lunar eclipse") {
		t.Errorf("horoscope fact: %+v", f)
	}
	if f := facts["still"]; f.Headline != 15 || !strings.Contains(f.Summary, "box breathing") {
		t.Errorf("still fact: %+v", f)
	}
	if f := facts["parcels"]; f.Headline != 1 || f.Summary != "Delivered: Running shoes (Decathlon); Sent: Birthday present for Ana; Ready to pick up: Phone case (Pošta Sarajevo)" {
		t.Errorf("parcels fact (ordered earlier, out for delivery left out): %+v", f)
	}
	if f := facts["sign"]; f.Headline != 1 || f.Summary != "Everyone signed: Ugovor o zakupu; Sent for signing: NDA — Acme (to Ana Kovač)" {
		t.Errorf("sign fact (Marko's signature is in \"everyone signed\"): %+v", f)
	}
	if f := facts["places"]; f.Headline != 1 || f.Summary != "Dinner at Kibe Mahala with Ana, ★5, 68.00 BAM; Saved to Pocket Places: Zlatna Ribica" {
		t.Errorf("places fact (Kibe Mahala saved and visited is said once): %+v", f)
	}
	if f := facts["wallet"]; f.Headline != 1 || f.Summary != "Dubioza kolektiv at Zetra Olympic Hall" {
		t.Errorf("wallet fact: %+v", f)
	}
	if f := facts["fitness"]; f.Headline != 1 || f.Summary != "Pull day (72 min, 15 sets, 2 records)" {
		t.Errorf("fitness fact (Pocket Health's run left to the health source): %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Pull day (72 min, 15 sets, 2 records): Deadlift (Barbell) 3 sets of 115 kg × 5 (a record)") || strings.Contains(l, "Morning run") {
		t.Errorf("the AI's training line:\n%s", l)
	}
	if f := facts["nutrition"]; f.Headline != 1850 || f.Summary != "1,850 kcal of 2,300 (protein 112 g, carbs 190 g, fat 64 g)" {
		t.Errorf("nutrition fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Food: 1,850 kcal of 2,300 (protein 112 g, carbs 190 g, fat 64 g), water 1.5 l of 2 l | Breakfast: oats, banana, coffee (520 kcal) | Dinner: ćevapi (1,330 kcal)") || strings.Contains(l, "Ate late") {
		t.Errorf("the AI's food line:\n%s", l)
	}
	if f := facts["blog"]; f.Headline != 1 || f.Summary != "Published “Spring in Sarajevo” (1,240 words); Wrote “Notes on rain” (a draft); 312 readers, 4 comments on the blog" {
		t.Errorf("blog fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Blog: Published “Spring in Sarajevo” (1,240 words); Wrote “Notes on rain” (a draft); 312 readers, 4 comments on the blog | “Spring in Sarajevo”: The river was loud.") {
		t.Errorf("the AI's blog line:\n%s", l)
	}
	if f := facts["chat"]; f.Headline != 12 || f.Summary != "Chatted in Family (8 messages), Weekend hikers (4); 2 photos; plans: 20:00 that day (Family); 1 thing shared" {
		t.Errorf("chat fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Chat: Chatted in Family (8 messages), Weekend hikers (4); 2 photos; plans: 20:00 that day (Family); 1 thing shared | shared: “Grandma's stuffed peppers” (kitchen) in Family") || strings.Contains(l, "Pirpa") {
		t.Errorf("the AI's chat line (never the words of a message):\n%s", l)
	}
	if f := facts["forms"]; f.Headline != 2 || f.Summary != "2 answers: Ana Kovač to “Client brief”, Marko to “RSVP”; opened “Summer party RSVP”" {
		t.Errorf("forms fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Forms: 2 answers: Ana Kovač to “Client brief”, Marko to “RSVP”; opened “Summer party RSVP” | Ana Kovač on “Client brief”: A new website for the bakery | Marko on “RSVP”: Yes, two of us") {
		t.Errorf("the AI's forms line:\n%s", l)
	}
	if f := facts["crm"]; f.Headline != 2 || f.Summary != "Won: Online shop for cakes (Pekara Marić); call — Booking site; 3 next steps were due" {
		t.Errorf("crm fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Deals: Won: Online shop for cakes (Pekara Marić); call — Booking site; 3 next steps were due | Booking site: Talked through the pages | due on Booking site: Send the proposal | due on Logo: Call back") || strings.Contains(l, "gallery") {
		t.Errorf("the AI's deals line (never the notes):\n%s", l)
	}
	if f := facts["league"]; f.Headline != 1 || f.Summary != "Won in Tuesday Tennis: Aleksandar beat Marko Petrović 6–4 6–3" {
		t.Errorf("league fact (the game you have not answered stays in the card): %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Sport: Won in Tuesday Tennis: Aleksandar beat Marko Petrović 6–4 6–3 | Tuesday Tennis at 20:00: you're in, 4 in | Friday 5-a-side at 19:00: not answered, 8 in") {
		t.Errorf("the AI's sport line:\n%s", l)
	}
	if f := facts["tables"]; f.Headline != 2 || f.Summary != "Ana Marić at 14:30 (Clients: Visit); Logo for the bakery (Projects: Due)" {
		t.Errorf("tables fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Tables: Ana Marić at 14:30 (Clients: Visit) in Bakery | Logo for the bakery (Projects: Due) in Work") {
		t.Errorf("the AI's tables line:\n%s", l)
	}
	if f := facts["rentals"]; f.Headline != 2 || f.Summary != "Rent in: €650.00 (Flat 2 — Ana Marić); Repair reported: Leaking tap (Flat 1, Obala 12)" {
		t.Errorf("rentals fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Rentals: Rent in: €650.00 (Flat 2 — Ana Marić) | Repair reported: Leaking tap (Flat 1, Obala 12)") {
		t.Errorf("the AI's rentals line:\n%s", l)
	}
	if f := facts["trails"]; f.Headline != 1 || f.Summary != "Hike: Trebević from Sarajevo (12.4 km · 980 m up · 4 h 30 min · with Ana); Run by the river at 18:00 (planned)" {
		t.Errorf("trails fact: %+v", f)
	}
	if l := strings.Join(factLines(facts, true), "\n"); !strings.Contains(l, "Outdoors: Hike: Trebević from Sarajevo (12.4 km · 980 m up · 4 h 30 min · with Ana) | Run by the river at 18:00 (planned)") {
		t.Errorf("the AI's outdoors line:\n%s", l)
	}
	if f := facts["voice"]; f.Headline != 1 || f.Summary != "Voice memo at 08:12: Dentist, dry cleaning and photos for Amra (0:21) → 2 tasks" {
		t.Errorf("voice fact (the memo still being written out left out): %+v", f)
	}
	if f := facts["pets"]; f.Headline != 1 || f.Summary != "Luna turned 5; Luna at the vet: limping on the front left leg — Dr. Hodžić; Mici weighed 4.9 kg (−0.04)" {
		t.Errorf("pets fact (no food purchase, nothing due): %+v", f)
	}
	if f := facts["firefly"]; f.Summary != "12.50 BAM in 1 transaction" || f.Headline != 12.5 {
		t.Errorf("money fact under the old id: %+v", f)
	}
	if _, ok := facts["money"]; ok {
		t.Error("money fact stored under a new id")
	}
	e.mu.Lock()
	if len(e.bad) > 0 {
		t.Errorf("calls outside the scopes: %v", e.bad)
	}
	asked := map[string]int{}
	for _, tk := range e.tokens {
		asked[tk]++
	}
	e.mu.Unlock()
	for _, n := range pocketSourceOrder {
		ps := pocketSources[n]
		if asked["owner@x "+ps.slug+" "+ps.scope] != 1 {
			t.Errorf("token for %s %s asked %d times: %v", ps.slug, ps.scope, asked["owner@x "+ps.slug+" "+ps.scope], asked)
		}
		if last := e.lastCall(ps.slug); !strings.HasPrefix(last, "Bearer pcx_"+ps.slug+"_"+ps.scope+" ") {
			t.Errorf("%s called with %q", ps.slug, last)
		}
	}
	if last := e.lastCall("money"); last != "Bearer pcx_money_transactions:read /firefly/api/v1/transactions" {
		t.Errorf("money read %q", last)
	}

	// A fresh token per fetch: pulling again asks Core again.
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	e.mu.Lock()
	n := len(e.tokens)
	e.mu.Unlock()
	if n != 2*len(pocketSourceOrder) {
		t.Errorf("%d token requests after two pulls, want %d", n, 2*len(pocketSourceOrder))
	}

	// Test connection honours the mode.
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "travel"})
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "Pocket Travel 9.9 through Pocket Core") {
		t.Errorf("travel test %s", body)
	}
	_, body = e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "firefly"})
	if !strings.Contains(body, "Pocket Money 0.4.0 (Firefly-compatible API) through Pocket Core") {
		t.Errorf("money test %s", body)
	}

	// The day page links to where Core says the app lives.
	_, body = e.do(e.ownerTok, "GET", "/days/"+day0, nil)
	if !strings.Contains(body, `"garage"`) {
		t.Errorf("day sources %s", clipStr(body))
	}
}

// A saved token means direct (pasted tokens keep working); Off makes no calls;
// a link switched off in Core shows as not connected and the source is
// skipped without failing the day.
func TestPocketSourceModes(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()

	// direct: the token typed into Settings, nothing asked of Core for it
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"habits": map[string]any{"url": e.targets["habits"].URL, "key": "direct-habits"}}})
	v := e.settings(e.ownerTok)
	if h := v.Sources["habits"]; h.Mode != "direct" || h.Via != "direct" || !h.Configured {
		t.Fatalf("habits after a token: %+v", h)
	}
	// off: no calls at all
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"kitchen": map[string]any{"mode": "off"}}})
	v = e.settings(e.ownerTok)
	if k := v.Sources["kitchen"]; k.Mode != "off" || k.Via != "" || k.Detail != "switched off" || k.Configured || k.Enabled {
		t.Fatalf("kitchen off: %+v", k)
	}
	// the garage link switched off by the person in Core
	e.mu.Lock()
	e.off["garage vehicles:read"] = "off_person"
	e.mu.Unlock()
	v = e.settings(e.ownerTok)
	if g := v.Sources["garage"]; g.Mode != "core" || g.Via != "" || g.Configured || !strings.Contains(g.Detail, "switched the Day → Pocket Garage link off") {
		t.Fatalf("garage link off: %+v", g)
	}
	// the split link answers on in the status but Core refuses the token (switched off in between)
	e.mu.Lock()
	e.tokOff["split groups:read"] = true
	e.mu.Unlock()

	errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if len(errs) != 0 {
		t.Fatalf("a link that is off failed the day: %v", errs)
	}
	facts := e.facts(e.owner, day0)
	for _, n := range []string{"kitchen", "garage", "split"} {
		if f, ok := facts[n]; ok {
			t.Errorf("%s pulled or recorded as failed: %+v", n, f)
		}
	}
	for _, n := range []string{"habits", "travel", "firefly", "bookshelf"} {
		if f := facts[n]; f.Error != "" || f.Summary == "" {
			t.Errorf("%s: %+v", n, f)
		}
	}
	if e.callCount("kitchen") != 0 || e.callCount("garage") != 0 || e.callCount("split") != 0 {
		t.Errorf("calls with the link off: kitchen %d garage %d split %d", e.callCount("kitchen"), e.callCount("garage"), e.callCount("split"))
	}
	if last := e.lastCall("habits"); last != "Bearer direct-habits /api/v1/days/"+day0 {
		t.Errorf("habits direct read %q", last)
	}
	e.mu.Lock()
	for _, tk := range e.tokens {
		if strings.Contains(tk, " habits ") || strings.Contains(tk, " kitchen ") || strings.Contains(tk, " garage ") {
			t.Errorf("asked Core for %q", tk)
		}
	}
	e.mu.Unlock()
	// backfill counts only what is connected (kitchen off, garage link off)
	st, _ := e.st.GetSettings(ctx, e.owner.ID)
	if n := len(e.s.fetchers(ctx, e.owner, st)); n != len(pocketSourceOrder)-2+len(serviceSourceOrder) {
		t.Errorf("%d fetchers", n)
	}

	// Choosing Pocket Core ignores the saved token (it stays for switching back).
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"habits": map[string]any{"mode": "core"}}})
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if last := e.lastCall("habits"); !strings.HasPrefix(last, "Bearer pcx_habits_habits:read ") {
		t.Errorf("habits in core mode read %q", last)
	}
	if st, _ := e.st.GetSettings(ctx, e.owner.ID); st.Sources["habits"].Key != "direct-habits" {
		t.Error("switching to Pocket Core dropped the saved token")
	}
	// Direct without a token says what is missing.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"travel": map[string]any{"mode": "direct"}}})
	if tr := e.settings(e.ownerTok).Sources["travel"]; tr.Via != "" || tr.Detail != "no token yet" {
		t.Errorf("direct without a token: %+v", tr)
	}
	// Disconnect clears URL and token; with no mode chosen it falls back to Pocket Core.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"invoicing": map[string]any{"url": e.targets["invoicing"].URL, "key": "direct-invoicing"}}})
	if iv := e.settings(e.ownerTok).Sources["invoicing"]; iv.Via != "direct" {
		t.Fatalf("invoicing direct: %+v", iv)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"invoicing": map[string]any{"url": "", "key": "", "public_url": ""}}})
	if iv := e.settings(e.ownerTok).Sources["invoicing"]; iv.Via != "core" || iv.Key != "" || iv.URL != "" {
		t.Errorf("invoicing after disconnect: %+v", iv)
	}

	// The member is not in the apps' audience: nothing connected, nothing pulled, nothing failed.
	mv := e.settings(e.memberTok)
	if h := mv.Sources["habits"]; h.Mode != "core" || h.Via != "" || !strings.Contains(h.Detail, "do not have access to Pocket Habits") {
		t.Errorf("member habits: %+v", h)
	}
	errs, _ = e.s.pullDay(ctx, e.member.ID, day0, false)
	if len(errs) != 0 || len(e.facts(e.member, day0)) != 0 {
		t.Errorf("member pull: %v %v", errs, e.facts(e.member, day0))
	}
}

// The money source reads Pocket Money's /firefly API, directly with a Money
// token, under the old "firefly" id; days frozen from Firefly III stay as they are.
func TestMoneySourceViaFireflyAPI(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	frozenDay := "2026-08-01"
	old := json.RawMessage(`{"count":1,"totals":{"BAM":40},"items":[{"id":"ff-9","description":"Firefly era","amount":40,"currency":"BAM"}],"main_currency":"BAM"}`)
	if err := e.st.PutFact(ctx, e.owner.ID, &store.Fact{Date: frozenDay, Source: "firefly", Payload: old, Headline: 40, Summary: "40.00 BAM in 1 transaction"}); err != nil {
		t.Fatal(err)
	}
	money := e.targets["money"].URL
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"firefly": map[string]any{"mode": "direct", "url": money + "/firefly", "key": "direct-money"}}})
	if f := e.settings(e.ownerTok).Sources["firefly"]; f.Mode != "direct" || f.Via != "direct" {
		t.Fatalf("money direct: %+v", f)
	}
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "firefly"})
	if !strings.Contains(body, "Pocket Money 0.4.0 (Firefly-compatible API), direct connection") {
		t.Errorf("money test %s", body)
	}
	e.s.pullDayOnly(ctx, e.owner.ID, day0, false, []string{"firefly"})
	if last := e.lastCall("money"); last != "Bearer direct-money /firefly/api/v1/transactions" {
		t.Errorf("money read %q", last)
	}
	if f := e.facts(e.owner, day0)["firefly"]; f.Summary != "12.50 BAM in 1 transaction" {
		t.Errorf("money fact %+v", f)
	}
	// the frozen Firefly day is not fetched again, so its facts do not change
	calls := e.callCount("money")
	e.s.pullDay(ctx, e.owner.ID, frozenDay, false)
	f := e.facts(e.owner, frozenDay)["firefly"]
	if f.Summary != "40.00 BAM in 1 transaction" || !strings.Contains(string(f.Payload), "Firefly era") {
		t.Errorf("frozen money day changed: %+v", f)
	}
	for _, c := range e.calls["money"][calls:] {
		if strings.Contains(c, "transactions") {
			t.Errorf("frozen day re-fetched: %s", c)
		}
	}
	// the webhook-driven refresh still pulls the "firefly" source
	if _, ran := e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"firefly"}); !ran {
		t.Error("money refresh did not run")
	}
}

// Without Pocket Core a Pocket app source is direct (nothing saved: not
// connected yet), as before.
func TestPocketSourcesWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	v := e.settings(e.ownerTok)
	if v.CoreConfigured {
		t.Fatal("core configured")
	}
	if h := v.Sources["habits"]; h.Mode != "direct" || h.Via != "" || h.Detail != "no token yet" {
		t.Errorf("habits without core: %+v", h)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"habits": map[string]any{"mode": "core"}}})
	if h := e.settings(e.ownerTok).Sources["habits"]; h.Via != "" || !strings.Contains(h.Detail, "no Pocket Core on this box") {
		t.Errorf("core mode without core: %+v", h)
	}
	if errs, _ := e.s.pullDay(context.Background(), e.owner.ID, day0, false); len(errs) != 0 {
		t.Errorf("errs %v", errs)
	}
}

// Env values for a Pocket app source are copied once: a Disconnect survives a restart.
func TestPocketSourceEnvSeededOnce(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.cfg.Sources["habits"] = config.SourceEnv{URL: e.targets["habits"].URL, Key: "direct-habits"}
	e.cfg.Sources["dawarich"] = config.SourceEnv{URL: "http://dawarich.invalid", Key: "dk"}
	if err := e.s.seedOwner(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := e.st.GetSettings(ctx, e.owner.ID)
	if st.Sources["habits"].Key != "direct-habits" || st.Sources["dawarich"].Key != "dk" {
		t.Fatalf("not seeded: %+v", st.Sources["habits"])
	}
	if h := e.settings(e.ownerTok).Sources["habits"]; h.Mode != "direct" {
		t.Errorf("seeded token should mean direct: %+v", h)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"habits": map[string]any{"url": "", "key": ""}}})
	if err := e.s.seedOwner(ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = e.st.GetSettings(ctx, e.owner.ID)
	if st.Sources["habits"].Key != "" || st.Sources["habits"].URL != "" {
		t.Errorf("disconnect undone by a restart: %+v", st.Sources["habits"])
	}
	if h := e.settings(e.ownerTok).Sources["habits"]; h.Mode != "core" || h.Via != "core" {
		t.Errorf("after disconnect: %+v", h)
	}
}
