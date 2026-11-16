package sources

import (
	"context"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Booking ----------------

// Booking reads the day in Pocket Booking: the bookings that take place that
// day (when, which booking page, who booked, whether it still waits for you
// to confirm, the meeting link or place) and, apart, the bookings people made
// that day (for whatever day they booked). The guests' answers and contact
// details stay in Pocket Booking.
type Booking struct{ Conn pocket.Conn }

// NewBooking creates a fetcher over a connection.
func NewBooking(conn pocket.Conn) *Booking { return &Booking{Conn: conn} }

// Name implements Fetcher.
func (b *Booking) Name() string { return "booking" }

// Test implements Fetcher.
func (b *Booking) Test(ctx context.Context) (string, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Booking " + v, nil
}

// BookingItem is one booking: a time someone booked with you.
type BookingItem struct {
	ID         int64  `json:"id"`
	Date       string `json:"date,omitempty"` // the day it takes place (owner's zone)
	Time       string `json:"time,omitempty"` // HH:MM it starts
	End        string `json:"end,omitempty"`  // HH:MM it ends
	Type       string `json:"type"`           // the booking page ("Intro call")
	Emoji      string `json:"emoji,omitempty"`
	Who        string `json:"who"`    // who booked (their name, else their email, else "Someone")
	Status     string `json:"status"` // pending (waits for you) | confirmed | declined | cancelled | done | no_show
	Location   string `json:"location,omitempty"`
	MeetingURL string `json:"meeting_url,omitempty"`
	URL        string `json:"url,omitempty"` // the booking in Pocket Booking
}

// BookingDay is the day in Pocket Booking.
type BookingDay struct {
	Date    string        `json:"date"`
	Count   int           `json:"count"`   // bookings taking place that day
	Pending int           `json:"pending"` // of them, the ones that wait for you to confirm
	Items   []BookingItem `json:"items"`
	Made    []BookingItem `json:"made"` // bookings people made that day
}

type bookingRow struct {
	ID         int64  `json:"id"`
	Time       string `json:"time"`
	End        string `json:"end"`
	Start      string `json:"start"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Emoji      string `json:"emoji"`
	Who        string `json:"who"`
	Email      string `json:"email"`
	Status     string `json:"status"`
	Location   string `json:"location"`
	MeetingURL string `json:"meeting_url"`
	URL        string `json:"url"`
}

func (r bookingRow) item(date string) BookingItem {
	it := BookingItem{ID: r.ID, Date: date, Time: strings.TrimSpace(r.Time), End: strings.TrimSpace(r.End), Emoji: r.Emoji,
		Status: strings.TrimSpace(r.Status), Location: strings.TrimSpace(r.Location), MeetingURL: strings.TrimSpace(r.MeetingURL), URL: strings.TrimSpace(r.URL)}
	if len(r.Start) >= 16 {
		if t, err := time.Parse(time.RFC3339, r.Start); err == nil {
			it.Date = t.Format("2006-01-02")
			if it.Time == "" {
				it.Time = t.Format("15:04")
			}
		} else if r.Start[4] == '-' {
			it.Date = r.Start[:10]
		}
	}
	it.Who = firstNonEmptyStr(strings.TrimSpace(r.Who), strings.TrimSpace(r.Email), "Someone")
	title := strings.TrimSpace(r.Title)
	if i := strings.LastIndex(title, " — "); i > 0 && strings.TrimSpace(r.Type) == "" {
		title = title[:i] // "Intro call — Ana Marić" without the guest
	}
	it.Type = firstNonEmptyStr(strings.TrimSpace(r.Type), title, "A booking")
	if it.Status == "" {
		it.Status = "confirmed"
	}
	return it
}

// Fetch implements Fetcher.
func (b *Booking) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Count int          `json:"count"`
		Items []bookingRow `json:"items"`
		Made  []bookingRow `json:"made"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := BookingDay{Date: date, Items: []BookingItem{}, Made: []BookingItem{}}
	for _, r := range page.Items {
		it := r.item(date)
		if it.Status == "cancelled" || it.Status == "declined" {
			continue // only what takes place
		}
		if it.Status == "pending" {
			d.Pending++
		}
		d.Items = append(d.Items, it)
	}
	for _, r := range page.Made {
		d.Made = append(d.Made, r.item(""))
	}
	d.Count = len(d.Items)
	return &Result{Payload: d, Headline: float64(d.Count), Summary: bookingSummary(d)}, nil
}

// BookingStatus is a booking's status in a few words; "" when it is simply on.
func BookingStatus(s string) string {
	switch s {
	case "pending":
		return "waits for you"
	case "done":
		return "done"
	case "no_show":
		return "did not come"
	case "cancelled":
		return "cancelled"
	case "declined":
		return "declined"
	}
	return ""
}

// BookingLine: "10:00 Intro call with Ana Marić (waits for you)".
func BookingLine(it BookingItem) string {
	s := it.Type + " with " + it.Who
	if it.Time != "" {
		s = it.Time + " " + s
	}
	if st := BookingStatus(it.Status); st != "" {
		s += " (" + st + ")"
	}
	return s
}

// bookingSummary: "2 bookings: 10:00 Intro call with Ana Marić, 14:30
// Consultation with Marko (waits for you); booked that day: Lejla for Intro
// call on 20 Nov". When and with whom, never what they wrote.
func bookingSummary(d BookingDay) string {
	var parts []string
	if n := len(d.Items); n > 0 {
		var ls []string
		for i, it := range d.Items {
			if i == 4 {
				ls[len(ls)-1] += " and " + Plural(n-4, "more", "more")
				break
			}
			ls = append(ls, BookingLine(it))
		}
		parts = append(parts, Plural(n, "booking", "bookings")+": "+strings.Join(ls, ", "))
	}
	if n := len(d.Made); n > 0 {
		var ls []string
		for i, it := range d.Made {
			if i == 3 {
				ls[len(ls)-1] += " and " + Plural(n-3, "more", "more")
				break
			}
			s := it.Who + " for " + it.Type
			if it.Date != "" && it.Date != d.Date {
				if t, err := time.Parse("2006-01-02", it.Date); err == nil {
					s += " on " + t.Format("2 Jan")
				}
			} else if it.Date == d.Date && it.Time != "" {
				s += " at " + it.Time
			}
			if st := BookingStatus(it.Status); st != "" && st != "done" {
				s += " (" + st + ")"
			}
			ls = append(ls, s)
		}
		parts = append(parts, "booked that day: "+strings.Join(ls, ", "))
	}
	s := strings.Join(parts, "; ")
	if s != "" {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
