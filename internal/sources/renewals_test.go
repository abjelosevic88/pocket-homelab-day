package sources

import "testing"

func TestRenewedLine(t *testing.T) {
	if got, want := renewedLine("Car insurance", "2027-01-05", 41200, "EUR"), "Renewed: Car insurance — now until 5 Jan 2027 (€412.00)"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := renewedLine("Domain", "", 0, "EUR"), "Renewed: Domain"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := minorAmount(1250, "chf"), "12.50 CHF"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
