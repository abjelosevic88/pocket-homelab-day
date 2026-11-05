package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Sign ----------------

// Sign reads what happened with signatures that day: documents sent for
// signing, people who signed them, documents everyone has signed, and what
// the person signed themselves (sent by someone else on the box).
type Sign struct{ Conn pocket.Conn }

// NewSign creates a fetcher over a connection.
func NewSign(conn pocket.Conn) *Sign { return &Sign{Conn: conn} }

// Name implements Fetcher.
func (s *Sign) Name() string { return "sign" }

// Test implements Fetcher.
func (s *Sign) Test(ctx context.Context) (string, error) {
	cl, err := s.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Sign " + v, nil
}

// SignItem is one line of the day: a document sent, completed, signed by
// someone, or signed by the person.
type SignItem struct {
	DocumentID  int64  `json:"document_id"`
	RecipientID int64  `json:"recipient_id,omitempty"` // signed by me: their own signing page
	Title       string `json:"title"`
	Who         string `json:"who,omitempty"` // who signed, who it went to, or who sent it
	Time        string `json:"time,omitempty"`
	Line        string `json:"line"`
}

// SignDay is the day in Pocket Sign.
type SignDay struct {
	Completed  []SignItem `json:"completed"`
	SignedByMe []SignItem `json:"signed_by_me"`
	Sent       []SignItem `json:"sent"`
	Signed     []SignItem `json:"signed"`
}

type signDocRow struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	SelfSign    bool   `json:"self_sign"`
	SentAt      string `json:"sent_at"`
	CompletedAt string `json:"completed_at"`
	Recipients  []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	} `json:"recipients"`
}

// Fetch implements Fetcher.
func (s *Sign) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := s.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Sent      []signDocRow `json:"sent"`
		Completed []signDocRow `json:"completed"`
		Signed    []struct {
			DocumentID int64  `json:"document_id"`
			Title      string `json:"title"`
			Name       string `json:"name"`
			At         string `json:"at"`
		} `json:"signed"`
		SignedByMe []struct {
			RecipientID int64  `json:"recipient_id"`
			DocumentID  int64  `json:"document_id"`
			Title       string `json:"title"`
			SenderName  string `json:"sender_name"`
			SenderEmail string `json:"sender_email"`
			Role        string `json:"role"`
			DoneAt      string `json:"done_at"`
		} `json:"signed_by_me"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	if loc == nil {
		loc = time.Local
	}
	d := SignDay{Completed: []SignItem{}, SignedByMe: []SignItem{}, Sent: []SignItem{}, Signed: []SignItem{}}
	done := map[int64]bool{}
	for _, r := range page.Completed {
		line := "Everyone signed: " + r.Title
		if r.SelfSign {
			line = "Signed: " + r.Title
		}
		d.Completed = append(d.Completed, SignItem{DocumentID: r.ID, Title: r.Title, Time: clockIn(r.CompletedAt, loc), Line: line})
		done[r.ID] = true
	}
	for _, r := range page.SignedByMe {
		verb := "You signed"
		switch r.Role {
		case "approver":
			verb = "You approved"
		case "viewer":
			verb = "You read"
		}
		who := firstSign(r.SenderName, r.SenderEmail)
		line := verb + ": " + r.Title
		if who != "" {
			line += " (from " + who + ")"
		}
		d.SignedByMe = append(d.SignedByMe, SignItem{DocumentID: r.DocumentID, RecipientID: r.RecipientID, Title: r.Title, Who: who, Time: clockIn(r.DoneAt, loc), Line: line})
	}
	for _, r := range page.Sent {
		if r.SelfSign {
			continue // signing it yourself is not sending it to anyone; it shows when signed
		}
		var names []string
		for _, p := range r.Recipients {
			if p.Role == "cc" {
				continue
			}
			names = append(names, firstSign(p.Name, p.Email))
		}
		who := strings.Join(names, ", ")
		if len(names) > 2 {
			who = fmt.Sprintf("%s and %d more", strings.Join(names[:2], ", "), len(names)-2)
		}
		line := "Sent for signing: " + r.Title
		if who != "" {
			line += " (to " + who + ")"
		}
		d.Sent = append(d.Sent, SignItem{DocumentID: r.ID, Title: r.Title, Who: who, Time: clockIn(r.SentAt, loc), Line: line})
	}
	for _, r := range page.Signed {
		who := strings.TrimSpace(r.Name)
		if who == "" {
			who = "Someone"
		}
		d.Signed = append(d.Signed, SignItem{DocumentID: r.DocumentID, Title: r.Title, Who: who, Time: clockIn(r.At, loc), Line: who + " signed " + r.Title})
	}
	var parts []string
	for _, list := range [][]SignItem{d.Completed, d.SignedByMe, d.Sent} {
		for _, it := range list {
			parts = append(parts, it.Line)
		}
	}
	for _, it := range d.Signed {
		if !done[it.DocumentID] { // "Everyone signed" already says it
			parts = append(parts, it.Line)
		}
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Completed) + len(d.SignedByMe)), Summary: sum}, nil
}

// firstSign is the name, else the email.
func firstSign(name, email string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return strings.TrimSpace(email)
}
