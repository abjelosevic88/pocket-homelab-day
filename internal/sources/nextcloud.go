package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Nextcloud reads, over CalDAV, the tasks completed on the day (every task
// list) plus events and birthdays on it. Auth is the user name + an app
// password.
type Nextcloud struct {
	BaseURL  string
	User     string
	Password string
	HTTP     *http.Client
}

// NewNextcloud creates a fetcher. extra is the user name.
func NewNextcloud(baseURL, password, user string) *Nextcloud {
	return &Nextcloud{BaseURL: strings.TrimRight(baseURL, "/"), User: strings.TrimSpace(user), Password: password, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Nextcloud) Name() string { return "nextcloud" }

func (c *Nextcloud) dav(ctx context.Context, method, path, depth, body string) ([]byte, error) {
	if c.BaseURL == "" || c.User == "" || c.Password == "" {
		return nil, errors.New("nextcloud needs a URL, the user name and an app password")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.Password)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", depth)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("nextcloud rejected the user name or app password")
	}
	if res.StatusCode != 207 && res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nextcloud HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 16<<20))
}

type davMultistatus struct {
	Responses []struct {
		Href  string `xml:"href"`
		Props []struct {
			Name  string `xml:"prop>displayname"`
			Data  string `xml:"prop>calendar-data"`
			Comps []struct {
				Name string `xml:"name,attr"`
			} `xml:"prop>supported-calendar-component-set>comp"`
			Resource struct {
				Calendar *struct{} `xml:"calendar"`
			} `xml:"prop>resourcetype"`
		} `xml:"propstat"`
	} `xml:"response"`
}

type ncCalendar struct {
	Href, Name  string
	Todo, Event bool
}

func (c *Nextcloud) calendars(ctx context.Context) ([]ncCalendar, error) {
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:displayname/><d:resourcetype/><c:supported-calendar-component-set/></d:prop></d:propfind>`
	raw, err := c.dav(ctx, "PROPFIND", "/remote.php/dav/calendars/"+c.User+"/", "1", body)
	if err != nil {
		return nil, err
	}
	var ms davMultistatus
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, err
	}
	var out []ncCalendar
	for _, r := range ms.Responses {
		var cal ncCalendar
		isCal := false
		for _, p := range r.Props {
			if p.Resource.Calendar != nil {
				isCal = true
			}
			if p.Name != "" {
				cal.Name = p.Name
			}
			for _, comp := range p.Comps {
				switch comp.Name {
				case "VTODO":
					cal.Todo = true
				case "VEVENT":
					cal.Event = true
				}
			}
		}
		if !isCal || strings.Contains(r.Href, "/trashbin") || strings.Contains(r.Href, "/inbox") || strings.Contains(r.Href, "/outbox") {
			continue
		}
		cal.Href = r.Href
		if cal.Name == "" {
			cal.Name = strings.Trim(r.Href[strings.LastIndex(strings.TrimSuffix(r.Href, "/"), "/")+1:], "/")
		}
		out = append(out, cal)
	}
	return out, nil
}

// Test implements Fetcher.
func (c *Nextcloud) Test(ctx context.Context) (string, error) {
	cals, err := c.calendars(ctx)
	if err != nil {
		return "", err
	}
	todo := 0
	for _, cl := range cals {
		if cl.Todo {
			todo++
		}
	}
	return fmt.Sprintf("Nextcloud reachable: %d calendars, %d with tasks", len(cals), todo), nil
}

func (c *Nextcloud) report(ctx context.Context, href, filter, expand string) ([]map[string]icalProp, error) {
	body := `<?xml version="1.0"?><c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-data>` + expand + `</c:calendar-data></d:prop><c:filter><c:comp-filter name="VCALENDAR">` + filter + `</c:comp-filter></c:filter></c:calendar-query>`
	raw, err := c.dav(ctx, "REPORT", href, "1", body)
	if err != nil {
		return nil, err
	}
	var ms davMultistatus
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, err
	}
	var out []map[string]icalProp
	for _, r := range ms.Responses {
		for _, p := range r.Props {
			if p.Data != "" {
				out = append(out, parseICalComponents(p.Data)...)
			}
		}
	}
	return out, nil
}

// Fetch implements Fetcher.
func (c *Nextcloud) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	z := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	cals, err := c.calendars(ctx)
	if err != nil {
		return nil, err
	}
	act := Activity{}
	for _, cal := range cals {
		if cal.Todo {
			comps, err := c.report(ctx, cal.Href, `<c:comp-filter name="VTODO"><c:prop-filter name="COMPLETED"><c:time-range start="`+z(from)+`" end="`+z(to)+`"/></c:prop-filter></c:comp-filter>`, "")
			if err != nil {
				return nil, err
			}
			for _, p := range comps {
				if p["_kind"].Value != "VTODO" {
					continue
				}
				done, _ := p["COMPLETED"].Time(loc)
				if done.IsZero() || done.Before(from) || !done.Before(to) {
					continue
				}
				act.Items = append(act.Items, ActivityItem{Time: LocalClock(done, loc), Title: p["SUMMARY"].Value, Sub: cal.Name, Done: true, Ref: p["UID"].Value})
			}
		}
		if cal.Event {
			comps, err := c.report(ctx, cal.Href, `<c:comp-filter name="VEVENT"><c:time-range start="`+z(from)+`" end="`+z(to)+`"/></c:comp-filter>`, `<c:expand start="`+z(from)+`" end="`+z(to)+`"/>`)
			if err != nil {
				return nil, err
			}
			birthdays := strings.Contains(cal.Href, "contact_birthdays")
			for _, p := range comps {
				if p["_kind"].Value != "VEVENT" {
					continue
				}
				title := p["SUMMARY"].Value
				it := ActivityItem{Title: title, Sub: cal.Name, Ref: p["UID"].Value}
				if birthdays {
					it.Title, it.Sub, it.Group = "🎂 "+strings.TrimPrefix(title, "🎂 "), "birthday", "birthdays"
				} else if !p["DTSTART"].AllDay() {
					st, _ := p["DTSTART"].Time(loc)
					en, _ := p["DTEND"].Time(loc)
					it.Time, it.End = LocalClock(st, loc), LocalClock(en, loc)
				}
				act.Items = append(act.Items, it)
			}
		}
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		tasks, other := 0, 0
		for _, it := range act.Items {
			if it.Done {
				tasks++
			} else {
				other++
			}
		}
		var parts []string
		if tasks > 0 {
			parts = append(parts, Plural(tasks, "task done", "tasks done"))
		}
		if other > 0 {
			parts = append(parts, Plural(other, "event", "events"))
		}
		sum = strings.Join(parts, " · ")
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}

// ---- a tiny iCalendar reader: enough for SUMMARY, UID, DTSTART/DTEND/COMPLETED ----

type icalProp struct {
	Value  string
	Params map[string]string
}

// Time parses a DATE-TIME or DATE value (UTC "Z", TZID or floating local).
func (p icalProp) Time(loc *time.Location) (time.Time, bool) {
	v := p.Value
	if v == "" {
		return time.Time{}, false
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return t, err == nil
	}
	l := loc
	if tz := p.Params["TZID"]; tz != "" {
		if z, err := time.LoadLocation(tz); err == nil {
			l = z
		}
	}
	if len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, l)
		return t, err == nil
	}
	t, err := time.ParseInLocation("20060102T150405", v, l)
	return t, err == nil
}

// AllDay reports a DATE (not DATE-TIME) value.
func (p icalProp) AllDay() bool { return p.Params["VALUE"] == "DATE" || len(p.Value) == 8 }

// parseICalComponents returns one property map per VTODO/VEVENT ("_kind" holds the type).
func parseICalComponents(data string) []map[string]icalProp {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\n ", "")
	data = strings.ReplaceAll(data, "\n\t", "")
	var out []map[string]icalProp
	var cur map[string]icalProp
	depth := 0
	for _, line := range strings.Split(data, "\n") {
		switch {
		case line == "BEGIN:VTODO" || line == "BEGIN:VEVENT":
			cur = map[string]icalProp{"_kind": {Value: strings.TrimPrefix(line, "BEGIN:")}}
			depth = 0
			continue
		case line == "END:VTODO" || line == "END:VEVENT":
			if cur != nil {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "BEGIN:") { // VALARM inside an event
			depth++
			continue
		}
		if strings.HasPrefix(line, "END:") {
			depth--
			continue
		}
		if depth > 0 {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		head, val := line[:i], line[i+1:]
		parts := strings.Split(head, ";")
		p := icalProp{Value: icalUnescape(val), Params: map[string]string{}}
		for _, kv := range parts[1:] {
			if j := strings.IndexByte(kv, '='); j > 0 {
				p.Params[strings.ToUpper(kv[:j])] = strings.Trim(kv[j+1:], `"`)
			}
		}
		name := strings.ToUpper(parts[0])
		if _, exists := cur[name]; !exists {
			cur[name] = p
		}
	}
	return out
}

func icalUnescape(s string) string {
	r := strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}
