package sources

import "sort"

// Activity is the shared payload of the "activity" sources (podcasts,
// audiobooks, calendar, documents, bookmarks, commits, …): a list of things
// that happened on the day, optionally timed and with minutes, plus the most
// frequent groups (show, author, calendar, repository). One renderer on the
// web side shows every activity source the same way.
type Activity struct {
	Count   int            `json:"count"`
	Minutes float64        `json:"minutes"`
	Done    int            `json:"done"` // finished episodes/books, paid invoices…
	Amount  float64        `json:"amount,omitempty"`
	Cur     string         `json:"currency,omitempty"`
	Qty     float64        `json:"qty,omitempty"` // e.g. commits across all repos
	Top     []Count        `json:"top"`
	Items   []ActivityItem `json:"items"`
}

// ActivityItem is one thing that happened.
type ActivityItem struct {
	Time     string  `json:"time,omitempty"` // HH:MM local, "" for all-day/unknown
	End      string  `json:"end,omitempty"`
	Title    string  `json:"title"`
	Sub      string  `json:"sub,omitempty"`   // show, author, calendar, repo…
	Group    string  `json:"group,omitempty"` // what Top counts (defaults to Sub)
	Minutes  float64 `json:"minutes,omitempty"`
	Done     bool    `json:"done,omitempty"`
	Ref      string  `json:"ref,omitempty"`      // id in the source
	Position float64 `json:"position,omitempty"` // seconds into the item (listening)
	Length   float64 `json:"length,omitempty"`   // seconds
	Amount   float64 `json:"amount,omitempty"`
	Currency string  `json:"currency,omitempty"`
	Qty      float64 `json:"qty,omitempty"`
	URL      string  `json:"url,omitempty"`
	Image    string  `json:"image,omitempty"`
}

// Finish computes the totals, the top groups and sorts the items by time.
func (a *Activity) Finish() {
	if a.Items == nil {
		a.Items = []ActivityItem{}
	}
	sort.SliceStable(a.Items, func(i, j int) bool {
		ti, tj := a.Items[i].Time, a.Items[j].Time
		if ti == "" {
			ti = "99"
		}
		if tj == "" {
			tj = "99"
		}
		return ti < tj
	})
	groups := map[string]int{}
	a.Count, a.Minutes, a.Done, a.Amount, a.Qty = len(a.Items), 0, 0, 0, 0
	for _, it := range a.Items {
		a.Minutes += it.Minutes
		a.Amount += it.Amount
		a.Qty += it.Qty
		if a.Cur == "" && it.Currency != "" {
			a.Cur = it.Currency
		}
		if it.Done {
			a.Done++
		}
		g := it.Group
		if g == "" {
			g = it.Sub
		}
		if g != "" {
			groups[g]++
		}
	}
	a.Minutes = Round1(a.Minutes)
	a.Amount = Round2(a.Amount)
	a.Top = TopCounts(groups, 6)
}

// ActivityMeta describes an activity source for briefs, recaps and patterns.
type ActivityMeta struct {
	Label           string // "Podcasts"
	One             string // "episode"
	Many            string // "episodes"
	Unit            string // min | count | amount | qty — what the headline measures
	QtyOne, QtyMany string // "commit", "commits" when Unit is qty
	Verb            string // "listened to" — for the mood pattern line
	Group           string // what Top counts, plural: "shows"
}

// Activities lists every activity source.
var Activities = map[string]ActivityMeta{
	"pinepods":       {Label: "Podcasts", One: "episode", Many: "episodes", Unit: "min", Verb: "with podcasts", Group: "shows"},
	"audiobookshelf": {Label: "Audiobooks", One: "title", Many: "titles", Unit: "min", Verb: "with an audiobook", Group: "authors"},
	"jellyseerr":     {Label: "Requests", One: "request", Many: "requests", Unit: "count", Verb: "with a film or show requested", Group: "kinds"},
	"forgejo":        {Label: "Code", One: "repository", Many: "repositories", Unit: "qty", QtyOne: "commit", QtyMany: "commits", Verb: "with commits pushed", Group: "repositories"},
	"nextcloud":      {Label: "Tasks", One: "task", Many: "tasks", Unit: "count", Verb: "with tasks done", Group: "lists"},
	"paperless":      {Label: "Documents", One: "document", Many: "documents", Unit: "count", Verb: "with documents filed", Group: "types"},
	"invoicing":      {Label: "Invoices", One: "item", Many: "items", Unit: "amount", Verb: "with invoicing", Group: "clients"},
	"split":          {Label: "Shared costs", One: "expense", Many: "expenses", Unit: "amount", Verb: "with shared costs", Group: "groups"},
	"inventory":      {Label: "Inventory", One: "change", Many: "changes", Unit: "count", Verb: "with inventory changes", Group: "items"},
}

// IsActivity reports whether a source uses the Activity payload.
func IsActivity(name string) bool { _, ok := Activities[name]; return ok }
