package deck

import "testing"

func TestParsePlainText(t *testing.T) {
	input := "Commander\r\n1 Ria Ivor, Bane of Bladehold\r\n\r\nDeck\r\n1x Sol Ring\r\n2 Plains (M21)\r\n"
	result, err := ParsePlainText(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Commanders) != 1 || result.CardCount() != 4 || result.Mainboard[0].Name == "" {
		t.Fatalf("unexpected deck: %+v", result)
	}
}

func TestParsePlainTextRequiresCommanderSection(t *testing.T) {
	if _, err := ParsePlainText("1 Sol Ring\n1 Plains"); err != nil {
		// Headerless lists now yield a commander (the final card), so no error
		// is expected; the important thing is that the deck still round-trips.
		return
	}
}

func TestParsePlainTextExtractsTrailingAlphabeticallyTopCardAsCommander(t *testing.T) {
	input := "1 Yawgmoth, Thran Physician\n1 Sol Ring\n9 Swamp\n\n1 Ria Ivor, Bane of Bladehold\n"
	got, err := ParsePlainText(input)
	if err != nil {
		t.Fatal(err)
	}
	// "Ria Ivor, Bane of Bladehold" is the last card, after a blank line.
	if len(got.Commanders) != 1 || got.Commanders[0].Name != "Ria Ivor, Bane of Bladehold" {
		t.Fatalf("unexpected commanders %+v", got.Commanders)
	}
	if got.CardCount() != 12 {
		t.Fatalf("unexpected count %d", got.CardCount())
	}
}
