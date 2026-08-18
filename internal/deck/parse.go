package deck

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxDecklistBytes  = 256 << 10
	maxDecklistLines  = 1000
	maxCardNameLength = 240
	maxCardQuantity   = 999
)

var cardLinePattern = regexp.MustCompile(`^\s*(\d{1,3})\s*[xX]?\s+(.+?)\s*$`)

func ParsePlainText(input string) (Deck, error) {
	if len(input) > maxDecklistBytes {
		return Deck{}, errors.New("decklist is too large")
	}
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")
	if len(lines) > maxDecklistLines {
		return Deck{}, errors.New("decklist has too many lines")
	}
	section := ""
	commanders := map[string]Card{}
	mainboard := map[string]Card{}
	var trailingLine string
	sawCommanderSection := false
	for number, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch strings.ToLower(strings.TrimSuffix(line, ":")) {
		case "commander", "commanders":
			section = "commander"
			sawCommanderSection = true
			continue
		case "deck", "mainboard", "main deck", "maindeck":
			section = "mainboard"
			continue
		case "sideboard", "maybeboard", "considering":
			section = "ignore"
			continue
		}
		match := cardLinePattern.FindStringSubmatch(line)
		if match == nil {
			return Deck{}, fmt.Errorf("invalid decklist line %d", number+1)
		}
		if section == "" {
			// Headerless format: everything is mainboard until the final line,
			// which is treated as the commander.
			section = "mainboard"
		}
		if section == "ignore" {
			continue
		}
		quantity, _ := strconv.Atoi(match[1])
		if quantity < 1 || quantity > maxCardQuantity {
			return Deck{}, fmt.Errorf("invalid quantity on line %d", number+1)
		}
		name := cleanImportedName(match[2])
		if name == "" || len(name) > maxCardNameLength {
			return Deck{}, fmt.Errorf("invalid card name on line %d", number+1)
		}
		trailingLine = name
		target := mainboard
		commander := false
		if section == "commander" {
			target = commanders
			commander = true
		}
		key := strings.ToLower(name)
		item := target[key]
		item.Name = name
		item.Quantity += quantity
		item.Commander = commander
		target[key] = item
	}
	// The user's pasted format places the commander on its own line at the very
	// end, after a blank line, with no "Commander"/"Deck" section headers.
	if len(commanders) == 0 && !sawCommanderSection && trailingLine != "" {
		delete(mainboard, strings.ToLower(trailingLine))
		commanders[strings.ToLower(trailingLine)] = Card{Name: trailingLine, Quantity: 1, Commander: true}
	}
	if len(commanders) == 0 {
		return Deck{}, errors.New("decklist must include at least one commander")
	}
	if len(mainboard) == 0 {
		return Deck{}, errors.New("decklist mainboard is empty")
	}
	result := Deck{}
	for _, item := range commanders {
		result.Commanders = append(result.Commanders, item)
	}
	for _, item := range mainboard {
		if _, duplicate := commanders[strings.ToLower(item.Name)]; !duplicate {
			result.Mainboard = append(result.Mainboard, item)
		}
	}
	if result.CardCount() > 1000 {
		return Deck{}, errors.New("decklist card count is too large")
	}
	return result, nil
}

func cleanImportedName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.LastIndex(value, " ("); index > 0 && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[:index])
	}
	value = normalizeSplit(value)
	return value
}

// normalizeSplit folds single-slash DFC spellings like "X/Y" into the
// canonical "X // Y" separator Scryfall uses.
func normalizeSplit(value string) string {
	for index := 1; index < len(value)-1; index++ {
		if value[index] != '/' {
			continue
		}
		if value[index-1] == '/' || value[index+1] == '/' {
			continue
		}
		if value[index-1] == ' ' || value[index+1] == ' ' {
			continue
		}
		value = value[:index] + " // " + value[index+1:]
		break
	}
	return value
}
