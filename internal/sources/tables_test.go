package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestTablesDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-10" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-10","items":[
			{"id":7,"title":"Logo for the bakery","table":"Projects","base":"Work","field":"Due","date":"2026-11-10","url":"https://tables.example.org/r/7"},
			{"id":12,"title":"Ana Marić","table":"Clients","table_icon":"👥","base":"Bakery","field":"Visit","date":"2026-11-10T14:30","url":"https://tables.example.org/r/12"},
			{"id":12,"title":"Ana Marić","table":"Clients","table_icon":"👥","base":"Bakery","field":"Visit","date":"2026-11-10T14:30","url":"https://tables.example.org/r/12"},
			{"id":20,"title":"","table":"Plants","base":"Home","field":"Water","date":"2026-11-10","url":"https://tables.example.org/r/20"},
			{"id":21,"title":"Fern","table":"Plants","base":"Home","field":"Water","date":"2026-11-10","url":"https://tables.example.org/r/21"}]}`))
	}))
	defer ts.Close()
	tb := NewTables(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := tb.Fetch(context.Background(), "2026-11-10", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := "Logo for the bakery (Projects: Due); Ana Marić at 14:30 (Clients: Visit); A row (Plants: Water) and 1 more row"
	if res.Summary != want || res.Headline != 4 {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(TablesDay)
	if len(d.Rows) != 4 || d.Rows[1].Time != "14:30" || d.Rows[1].Icon != "👥" || d.Rows[0].Time != "" || d.Rows[1].URL != "https://tables.example.org/r/12" {
		t.Fatalf("day %+v", d)
	}
}

func TestTablesSummary(t *testing.T) {
	r := func(title, table, field, tm string) TablesRow {
		return TablesRow{Title: title, Table: table, Field: field, Time: tm}
	}
	for _, c := range []struct {
		d    TablesDay
		want string
	}{
		{TablesDay{}, ""},
		{TablesDay{Rows: []TablesRow{r("Fern", "Plants", "Water", "")}}, "Fern (Plants: Water)"},
		{TablesDay{Rows: []TablesRow{r("Fern", "Plants", "", "08:00")}}, "Fern at 08:00 (Plants)"},
		{TablesDay{Rows: []TablesRow{r("A", "T", "D", ""), r("B", "T", "D", ""), r("C", "T", "D", ""), r("D", "T", "D", ""), r("E", "T", "D", "")}}, "A (T: D); B (T: D); C (T: D) and 2 more rows"},
	} {
		if got := tablesSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}
