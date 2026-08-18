package cardcatalog

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCardPreservesDoubleFacedImagesAndAliases(t *testing.T) {
	var raw scryfallCard
	if err := json.Unmarshal([]byte(`{
		"oracle_id":"oracle","name":"Brightclimb Pathway // Grimclimb Pathway","layout":"modal_dfc","type_line":"Land // Land",
		"card_faces":[
			{"name":"Brightclimb Pathway","type_line":"Land","oracle_text":"{T}: Add {W}.","image_uris":{"small":"front-small","normal":"front-normal"}},
			{"name":"Grimclimb Pathway","type_line":"Land","oracle_text":"{T}: Add {B}.","image_uris":{"small":"back-small","normal":"back-normal"}}
		]
	}`), &raw); err != nil {
		t.Fatal(err)
	}
	card := normalizeCard(raw)
	if card.Layout != "modal_dfc" || len(card.Faces) != 2 || card.Faces[1].ImageNormal != "back-normal" {
		t.Fatalf("double-faced data lost: %+v", card)
	}
	keys := cardLookupKeys(card)
	for _, want := range []string{"brightclimb pathway // grimclimb pathway", "brightclimb pathway", "grimclimb pathway"} {
		found := false
		for _, key := range keys {
			if key == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing alias %q in %#v", want, keys)
		}
	}
}

func TestNormalizeCardKeepsAdventureSingleImage(t *testing.T) {
	var raw scryfallCard
	if err := json.Unmarshal([]byte(`{
		"name":"Topaz Dragon // Entropic Cloud","layout":"adventure","image_uris":{"small":"whole-card"},
		"card_faces":[{"name":"Topaz Dragon"},{"name":"Entropic Cloud"}]
	}`), &raw); err != nil {
		t.Fatal(err)
	}
	card := normalizeCard(raw)
	if card.ImageSmall != "whole-card" || card.Faces[0].ImageSmall != "" || card.Faces[1].ImageSmall != "" {
		t.Fatalf("unexpected adventure image mapping: %+v", card)
	}
}

func TestNormalizeCardCopiesFrontFaceImageWhenTopLevelMissing(t *testing.T) {
	var raw scryfallCard
	if err := json.Unmarshal([]byte(`{
		"name":"Brightclimb Pathway // Grimclimb Pathway","layout":"modal_dfc",
		"card_faces":[
			{"name":"Brightclimb Pathway","image_uris":{"small":"front-small","normal":"front-normal"}},
			{"name":"Grimclimb Pathway","image_uris":{"small":"back-small","normal":"back-normal"}}
		]
	}`), &raw); err != nil {
		t.Fatal(err)
	}
	card := normalizeCard(raw)
	if card.Faces[0].ImageSmall != "front-small" || card.Faces[1].ImageNormal != "back-normal" {
		t.Fatalf("face images lost: %+v", card)
	}
}

func TestNormalizeSplitName(t *testing.T) {
	cases := map[string]string{
		"dollmaker's shop/porcelain gallery":       "dollmaker's shop",
		"dollmaker's shop // porcelain gallery":    "dollmaker's shop",
		"w/x":                                      "w",
		"who/what/when/where/why":                  "who",
		"brightclimb pathway // grimclimb pathway": "brightclimb pathway",
	}
	for input, want := range cases {
		if got := normalizeSplitName(input); got != want {
			t.Fatalf("normalizeSplitName(%q) = %q, want %q", input, got, want)
		}
	}
}
