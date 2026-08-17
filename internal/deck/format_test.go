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
