package deck

import (
	"fmt"
	"sort"
	"strings"
)

func FormatExportPlainText(commanders, mainboard []Card) string {
	var builder strings.Builder
	builder.WriteString("Commander\n")
	builder.WriteString(FormatPlainText(commanders, nil))
	builder.WriteString("\n\nDeck\n")
	builder.WriteString(FormatPlainText(nil, mainboard))
	return strings.TrimSpace(builder.String())
}

func FormatPlainText(commanders, mainboard []Card) string {
	commanders = append([]Card(nil), commanders...)
	mainboard = append([]Card(nil), mainboard...)
	sort.Slice(commanders, func(i, j int) bool { return commanders[i].Name < commanders[j].Name })
	sort.Slice(mainboard, func(i, j int) bool { return mainboard[i].Name < mainboard[j].Name })

	var builder strings.Builder
	for _, card := range commanders {
		fmt.Fprintf(&builder, "%d %s\n", card.Quantity, card.Name)
	}
	if len(commanders) > 0 && len(mainboard) > 0 {
		builder.WriteByte('\n')
	}
	for _, card := range mainboard {
		fmt.Fprintf(&builder, "%d %s\n", card.Quantity, card.Name)
	}
	return strings.TrimSpace(builder.String())
}
