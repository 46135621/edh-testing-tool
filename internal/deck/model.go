package deck

type Card struct {
	Name        string   `json:"name"`
	Quantity    int      `json:"quantity"`
	Commander   bool     `json:"commander,omitempty"`
	Salt        float64  `json:"-"`
	Price       float64  `json:"-"`
	Rank        float64  `json:"-"`
	CMC         float64  `json:"-"`
	Colors      []string `json:"-"`
	Produced    []string `json:"-"`
	ManaCost    string   `json:"-"`
	TypeLine    string   `json:"-"`
	Layout      string   `json:"-"`
	Reserved    bool     `json:"-"`
	GameChanger bool     `json:"-"`
}

type Deck struct {
	SourceURL  string `json:"source_url"`
	SourceID   string `json:"source_id"`
	Name       string `json:"name"`
	Commanders []Card `json:"commanders"`
	Mainboard  []Card `json:"mainboard"`
}

func (d Deck) CardCount() int {
	total := 0
	for _, card := range d.Commanders {
		total += card.Quantity
	}
	for _, card := range d.Mainboard {
		total += card.Quantity
	}
	return total
}

func (d Deck) PlainText() string {
	return FormatPlainText(d.Commanders, d.Mainboard)
}

func (d Deck) ExportPlainText() string {
	return FormatExportPlainText(d.Commanders, d.Mainboard)
}
