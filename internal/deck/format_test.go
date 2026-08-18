package deck

import "testing"

func TestFormatPlainText(t *testing.T) {
	t.Parallel()
	got := FormatPlainText(
		[]Card{{Name: "Ria Ivor, Bane of Bladehold", Quantity: 1}},
		[]Card{{Name: "Swamp", Quantity: 9}, {Name: "Arcane Signet", Quantity: 1}},
	)
	want := "1 Ria Ivor, Bane of Bladehold\n\n1 Arcane Signet\n9 Swamp"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportPlainTextRoundTrips(t *testing.T) {
	original := Deck{Commanders: []Card{{Name: "Zeta", Quantity: 1}, {Name: "Alpha", Quantity: 1}}, Mainboard: []Card{{Name: "Swamp", Quantity: 9}, {Name: "Arcane Signet", Quantity: 1}}}
	parsed, err := ParsePlainText(original.ExportPlainText())
	if err != nil {
		t.Fatalf("exported deck did not parse: %v", err)
	}
	if parsed.CardCount() != original.CardCount() || len(parsed.Commanders) != 2 || parsed.Mainboard[0].Name != "Arcane Signet" {
		t.Fatalf("round trip mismatch: %+v", parsed)
	}
}
