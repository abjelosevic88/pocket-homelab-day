package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestSignDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sent": []map[string]any{
				{"id": 1, "title": "NDA — Acme", "sent_at": "2026-11-05T08:00:00Z", "recipients": []map[string]any{{"name": "Ana Kovač", "email": "ana@example.com", "role": "signer"}, {"name": "", "email": "boss@acme.test", "role": "approver"}, {"name": "Accountant", "email": "acc@example.com", "role": "cc"}}},
				{"id": 2, "title": "Self", "self_sign": true, "sent_at": "2026-11-05T09:00:00Z", "recipients": []map[string]any{{"name": "Me", "role": "signer"}}},
			},
			"completed": []map[string]any{
				{"id": 3, "title": "Ugovor o zakupu", "completed_at": "2026-11-05T14:30:00Z"},
				{"id": 2, "title": "Self", "self_sign": true, "completed_at": "2026-11-05T09:05:00Z"},
			},
			"signed": []map[string]any{
				{"document_id": 3, "title": "Ugovor o zakupu", "name": "Marko Marić", "at": "2026-11-05T14:30:00Z"},
				{"document_id": 4, "title": "Quote 12", "name": "Ivana", "at": "2026-11-05T10:15:00Z"},
			},
			"signed_by_me": []map[string]any{
				{"recipient_id": 9, "document_id": 7, "title": "School trip consent", "sender_name": "Sanja", "role": "signer", "done_at": "2026-11-05T19:00:00Z"},
			},
		})
	}))
	defer ts.Close()
	s := NewSign(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := s.Fetch(context.Background(), "2026-11-05", time.FixedZone("CET", 3600))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Everyone signed: Ugovor o zakupu; Signed: Self; You signed: School trip consent (from Sanja) and 2 more"; res.Headline != 3 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(SignDay)
	if len(d.Sent) != 1 || d.Sent[0].Line != "Sent for signing: NDA — Acme (to Ana Kovač, boss@acme.test)" || d.Sent[0].Time != "09:00" {
		t.Fatalf("sent %+v (self-signed left out, cc left out)", d.Sent)
	}
	if len(d.Signed) != 2 || d.Signed[1].Line != "Ivana signed Quote 12" || d.Signed[1].Time != "11:15" {
		t.Fatalf("signed %+v", d.Signed)
	}
	if len(d.SignedByMe) != 1 || d.SignedByMe[0].RecipientID != 9 || d.SignedByMe[0].Time != "20:00" {
		t.Fatalf("signed by me %+v", d.SignedByMe)
	}
}
