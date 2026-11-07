package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func rentalsServer(t *testing.T, asked *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.Path)
		switch {
		case r.URL.Path == "/api/v1/today":
			today := time.Now().In(time.UTC).Format("2006-01-02")
			_, _ = w.Write([]byte(`{"date":"` + today + `",
				"late":[{"charge_id":4,"lease_id":3,"lease":"Flat 2 — Ana Marić","place":"Flat 2, Obala 12","tenants":[{"name":"Ana Marić"}],"title":"Rent October","due_date":"2026-10-01","amount":65000,"paid":15000,"currency":"EUR","state":"late","days":12}],
				"due":[{"charge_id":5,"lease_id":3,"lease":"Flat 2 — Ana Marić","place":"Flat 2, Obala 12","tenants":[{"name":"Ana Marić"}],"title":"Rent","due_date":"` + today + `","amount":65000,"paid":0,"currency":"EUR","state":"due","days":0},
				       {"charge_id":6,"lease_id":5,"lease":"Studio — Marko","place":"Studio","title":"Rent","due_date":"2999-01-01","amount":40000,"paid":0,"currency":"EUR","state":"upcoming","days":-5}],
				"repairs":[{"id":9,"title":"Burst pipe","priority":"urgent","status":"in_progress","reported_on":"` + today + `","property_name":"Obala 12","unit_name":"Flat 1"},
				           {"id":10,"title":"Squeaky door","priority":"normal","status":"open","property_name":"Obala 12","unit_name":"Whole place"}]}`))
		case len(r.URL.Path) > len("/api/v1/day/") && r.URL.Path[:len("/api/v1/day/")] == "/api/v1/day/":
			_, _ = w.Write([]byte(`{"date":"x","items":[
				{"kind":"payment","title":"Rent in: €650.00","subtitle":"Flat 2 — Ana Marić","url":"https://rentals.example.org/leases/3"},
				{"kind":"lease","title":"Lease started","subtitle":"Studio — Marko","url":"https://rentals.example.org/leases/5"},
				{"kind":"repair","title":"Repair done: Leaking tap","subtitle":"Flat 1, Obala 12","url":"https://rentals.example.org/repairs/8"},
				{"kind":"inspection","title":"Moving-in inspection","subtitle":"Studio","url":"https://rentals.example.org/inspections/2"},
				{"kind":"repair","title":"  ","subtitle":"","url":""}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRentalsPastDay(t *testing.T) {
	var asked []string
	ts := rentalsServer(t, &asked)
	defer ts.Close()
	rt := NewRentals(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := rt.Fetch(context.Background(), "2026-09-01", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := "Rent in: €650.00 (Flat 2 — Ana Marić); Lease started (Studio — Marko); Repair done: Leaking tap (Flat 1, Obala 12) and 1 more"
	if res.Summary != want || res.Headline != 4 {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(RentalsDay)
	if d.Today != nil || len(asked) != 1 || d.Items[3].Kind != "inspection" || d.Items[0].URL != "https://rentals.example.org/leases/3" {
		t.Fatalf("a past day reads only the day: %v %+v", asked, d)
	}
}

func TestRentalsToday(t *testing.T) {
	var asked []string
	ts := rentalsServer(t, &asked)
	defer ts.Close()
	rt := NewRentals(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := rt.Fetch(context.Background(), time.Now().In(time.UTC).Format("2006-01-02"), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	d := res.Payload.(RentalsDay)
	if d.Today == nil {
		t.Fatalf("today: %v", asked)
	}
	td := d.Today
	if len(td.Late) != 1 || td.Late[0].Owed != 50000 || td.Late[0].Days != 12 || td.Late[0].Tenants != "Ana Marić" {
		t.Errorf("late %+v", td.Late)
	}
	if len(td.Due) != 1 || td.Due[0].LeaseID != 3 {
		t.Errorf("due today only (not the coming days): %+v", td.Due)
	}
	if len(td.Urgent) != 1 || td.Urgent[0].Place != "Flat 1, Obala 12" || td.Urgent[0].Status != "in progress" || td.Repairs != 2 {
		t.Errorf("urgent %+v", td)
	}
	if got := RentalsNeedsYou(td); got != "1 rent late, 1 due today, 1 urgent repair" {
		t.Errorf("needs you %q", got)
	}
	if RentalsNeedsYou(nil) != "" || RentalsNeedsYou(&RentalsToday{}) != "" {
		t.Error("nothing needs you")
	}
}
