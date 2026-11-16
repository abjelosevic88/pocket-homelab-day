package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestBookingDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-18" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-18","count":3,"summary":"3 bookings",
			"items":[{"kind":"booking","id":41,"time":"10:00","end":"10:30","start":"2026-11-18T10:00:00+01:00","title":"Intro call — Ana Marić","type":"Intro call","emoji":"📞","who":"Ana Marić","email":"ana@example.com","status":"confirmed","location":"","meeting_url":"https://meet.jit.si/pocket-abc","url":"https://booking.example.org/bookings/41"},
				{"kind":"booking","id":42,"time":"14:30","end":"15:30","start":"2026-11-18T14:30:00+01:00","title":"Consultation — marko@example.com","type":"","emoji":"","who":"","email":"marko@example.com","status":"pending","location":"Obala 12, Sarajevo","meeting_url":"","url":"https://booking.example.org/bookings/42"},
				{"kind":"booking","id":43,"time":"18:00","end":"18:30","start":"2026-11-18T18:00:00+01:00","title":"Intro call — Lejla","type":"Intro call","who":"Lejla","status":"cancelled","url":"https://booking.example.org/bookings/43"}],
			"made":[{"kind":"booking","id":44,"time":"09:00","end":"09:30","start":"2026-11-20T09:00:00+01:00","title":"Intro call — Lejla","type":"Intro call","emoji":"📞","who":"Lejla","email":"lejla@example.com","status":"pending","url":"https://booking.example.org/bookings/44"}]}`))
	}))
	defer ts.Close()
	b := NewBooking(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := b.Fetch(context.Background(), "2026-11-18", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2 bookings: 10:00 Intro call with Ana Marić, 14:30 Consultation with marko@example.com (waits for you); booked that day: Lejla for Intro call on 20 Nov (waits for you)"; res.Headline != 2 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(BookingDay)
	if d.Date != "2026-11-18" || d.Count != 2 || d.Pending != 1 || len(d.Made) != 1 {
		t.Fatalf("day %+v", d)
	}
	if it := d.Items[0]; it.Time != "10:00" || it.End != "10:30" || it.Type != "Intro call" || it.Emoji != "📞" || it.MeetingURL != "https://meet.jit.si/pocket-abc" || it.URL != "https://booking.example.org/bookings/41" || it.Date != "2026-11-18" {
		t.Fatalf("booking %+v", it)
	}
	if it := d.Items[1]; it.Type != "Consultation" || it.Who != "marko@example.com" || it.Status != "pending" || it.Location != "Obala 12, Sarajevo" {
		t.Fatalf("pending booking %+v", it)
	}
	if it := d.Made[0]; it.Date != "2026-11-20" || it.Time != "09:00" || it.Who != "Lejla" {
		t.Fatalf("made %+v", it)
	}
}

func TestBookingSummary(t *testing.T) {
	bk := func(id int64, time, who, status string) BookingItem {
		return BookingItem{ID: id, Date: "2026-11-18", Time: time, Type: "Call", Who: who, Status: status}
	}
	for _, c := range []struct {
		d    BookingDay
		want string
	}{
		{BookingDay{Date: "2026-11-18"}, ""},
		{BookingDay{Date: "2026-11-18", Items: []BookingItem{bk(1, "09:00", "Ana", "done")}}, "1 booking: 09:00 Call with Ana (done)"},
		{BookingDay{Date: "2026-11-18", Items: []BookingItem{bk(1, "09:00", "A", ""), bk(2, "10:00", "B", ""), bk(3, "11:00", "C", ""), bk(4, "12:00", "D", "no_show"), bk(5, "13:00", "E", "")}},
			"5 bookings: 09:00 Call with A, 10:00 Call with B, 11:00 Call with C, 12:00 Call with D (did not come) and 1 more"},
		{BookingDay{Date: "2026-11-18", Made: []BookingItem{bk(6, "16:00", "Ana", "confirmed")}}, "Booked that day: Ana for Call at 16:00"},
		{BookingDay{Date: "2026-11-18", Made: []BookingItem{{Type: "Call", Who: "Ana", Date: "2026-12-01", Status: "cancelled"}}}, "Booked that day: Ana for Call on 1 Dec (cancelled)"},
	} {
		if got := bookingSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}

func TestBookingEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","count":0,"summary":"","items":[],"made":[]}`))
	}))
	defer ts.Close()
	b := NewBooking(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := b.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(BookingDay); res.Headline != 0 || res.Summary != "" || d.Items == nil || d.Made == nil {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
