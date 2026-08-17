package edhpowerlevel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"

	"powerlevel/internal/deck"
)

type Client struct {
	pageURL         string
	browserPath     string
	browserHeadless bool
	semaphore       chan struct{}
	allocatorCtx    context.Context
	cancelAllocator context.CancelFunc
	browserCtx      context.Context
	cancelBrowser   context.CancelFunc
	closeOnce       sync.Once
}

func New(pageURL, browserPath string, browserHeadless bool, maxConcurrency int) (*Client, error) {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.Flag("headless", browserHeadless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("mute-audio", true),
	)
	if browserPath != "" {
		allocatorOptions = append(allocatorOptions, chromedp.ExecPath(browserPath))
	}
	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocatorCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		cancelBrowser()
		cancelAllocator()
		return nil, fmt.Errorf("start shared browser: %w", err)
	}
	return &Client{
		pageURL:         pageURL,
		browserPath:     browserPath,
		browserHeadless: browserHeadless,
		semaphore:       make(chan struct{}, maxConcurrency),
		allocatorCtx:    allocatorCtx,
		cancelAllocator: cancelAllocator,
		browserCtx:      browserCtx,
		cancelBrowser:   cancelBrowser,
	}, nil
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.cancelBrowser()
		c.cancelAllocator()
	})
	return nil
}

func (c *Client) Analyze(ctx context.Context, target deck.Deck) (map[string]any, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	tabCtx, cancelTab := chromedp.NewContext(c.browserCtx)
	defer cancelTab()
	stopCancellation := context.AfterFunc(ctx, cancelTab)
	defer stopCancellation()

	deckText := target.PlainText()
	expectedCommander := ""
	if len(target.Commanders) > 0 {
		expectedCommander = target.Commanders[0].Name
	}
	var submittedDeckText string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(c.pageURL),
		chromedp.WaitVisible(`#decklist`, chromedp.ByQuery),
		chromedp.Focus(`#decklist`, chromedp.ByQuery),
		chromedp.SetValue(`#decklist`, deckText, chromedp.ByQuery),
		chromedp.KeyEvent(" "),
		chromedp.KeyEvent("\b"),
		chromedp.Evaluate(`document.querySelector('#decklist')?.value || ''`, &submittedDeckText),
	); err != nil {
		return nil, fmt.Errorf("fill EDH Power Level decklist: %w", err)
	}
	if strings.TrimSpace(submittedDeckText) != strings.TrimSpace(deckText) {
		return nil, errors.New("EDH Power Level did not accept the submitted decklist")
	}

	var raw map[string]string
	var bracketDetails bracketDetails
	var analyzedCommander string
	if err := chromedp.Run(tabCtx,
		chromedp.Click(`#analyze`, chromedp.ByQuery),
		chromedp.Poll(`(() => {
			const body = document.body.innerText || '';
			return body.includes('100 total cards imported.') && body.includes('`+escapeJSString(expectedCommander)+`') && !body.includes('Sample Deck');
		})()`, nil, chromedp.WithPollingInterval(200)),
		chromedp.Evaluate(metricsScript, &raw),
		chromedp.Evaluate(bracketDetailsScript, &bracketDetails),
		chromedp.Evaluate(`(() => {
			const report = document.querySelector('.area-resultintro')?.parentElement || document.body;
			return (report.innerText || '').includes('`+escapeJSString(expectedCommander)+`') ? '`+escapeJSString(expectedCommander)+`' : '';
		})()`, &analyzedCommander),
	); err != nil {
		return nil, fmt.Errorf("run EDH Power Level analysis: %w", err)
	}
	if expectedCommander != "" && analyzedCommander != expectedCommander {
		return nil, errors.New("EDH Power Level returned results for a different deck")
	}

	metrics := normalizeMetrics(raw)
	if _, ok := metrics["rules_bracket"]; !ok {
		return nil, errors.New("EDH Power Level did not return its minimum bracket")
	}
	if _, ok := metrics["evaluated_bracket"]; !ok {
		return nil, errors.New("EDH Power Level did not return its recommended bracket")
	}
	if details := normalizeBracketDetails(bracketDetails); len(details.RulesBracketReasons) > 0 || details.EvaluatedBracketReason != "" {
		metrics["bracket_details"] = details
	}
	if _, ok := metrics["power_level"]; !ok {
		return nil, errors.New("EDH Power Level did not return a power level")
	}
	return metrics, nil
}

func escapeJSString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\r", `\r`, "\n", `\n`)
	return replacer.Replace(value)
}

const metricsScript = `(() => {
	const text = (selector) => document.querySelector(selector)?.textContent?.trim() || '';
	const values = {
		'Power Level': text('.res-power-level .val'),
		'Tipping Point': text('.res-tipping-point .val'),
		'Efficiency': text('.res-efficiency .val'),
		'Impact': text('.res-impact .val'),
		'Score': text('.res-score .val'),
		'Average Playability': text('.res-playability .val')
	};
	document.querySelectorAll('.result').forEach((node) => {
		const label = node.querySelector('.lab')?.textContent?.trim();
		const value = node.querySelector('.val')?.textContent?.trim();
		if (label && value) values[label] = value;
	});
	const heading = Array.from(document.querySelectorAll('h2')).find((node) => node.textContent.includes('Commander Bracket'));
	if (heading) values['Evaluated Bracket'] = heading.textContent;
	const pageText = (document.body.innerText || '') + ' ' + (document.body.textContent || '');
	const minimumPatterns = [
		/Minimum\s*Bracket\s*:\s*(\d+)/i,
		/minimum\s*bracket\s*is\s*(\d+)/i,
		/Lets\s*look\s*at\s*why\s*your\s*minimum\s*bracket\s*is\s*(\d+)/i,
		/minimum\s*based\s*only\s*on[^.]*?Bracket\s*(\d+)/i
	];
	for (const pattern of minimumPatterns) {
		const match = pageText.match(pattern);
		if (match) {
			values['Rules Bracket'] = match[1];
			break;
		}
	}
	return values;
})()`

const bracketDetailsScript = `(() => {
	const details = { rules_bracket_reasons: [], evaluated_bracket_reason: '' };
	const buttons = Array.from(document.querySelectorAll('button'));
	const detailsButton = buttons.find((node) => node.textContent.includes('Your Bracket Details'));
	const region = detailsButton?.closest('.accordion-item') || detailsButton?.parentElement?.parentElement;
	const sourceText = (region?.textContent || '').trim();
	const compact = sourceText.replace(/\s+/g, ' ').trim();
	const reasonPatterns = [
		['Early 2-Card Combos', /Early 2-Card Combos:\s*(\d+)[\s\S]*?(Your deck contains[^.]*\.)/i],
		['Game Changers', /Game Changers:\s*(\d+)[\s\S]*?(Your deck contains[^.]*\.)/i],
		['Extra Turns', /Extra Turns:\s*(\d+)[\s\S]*?(Your deck contains[^.]*\.)/i],
		['Mass Land Denial', /Mass Land Denial:\s*(\d+)[\s\S]*?(Your deck contains[^.]*\.)/i]
	];
	for (const [label, pattern] of reasonPatterns) {
		const match = compact.match(pattern);
		if (match && Number(match[1]) > 0) details.rules_bracket_reasons.push(label + ': ' + match[1] + ' - ' + match[2]);
	}
	const recommended = compact.match(/Your recommended bracket is\s+(?:also\s+)?(\d+)/i);
	if (recommended) details.evaluated_bracket_reason = 'EDH Power Level recommends Bracket ' + recommended[1] + ' after considering the deck power level.';
	return details;
})()`

func BrowserPathFromEnv() string {
	if path := os.Getenv("BROWSER_PATH"); path != "" {
		return path
	}
	candidates := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

type bracketDetails struct {
	RulesBracketReasons    []string `json:"rules_bracket_reasons"`
	EvaluatedBracketReason string   `json:"evaluated_bracket_reason"`
}

func normalizeBracketDetails(details bracketDetails) bracketDetails {
	seen := make(map[string]struct{}, len(details.RulesBracketReasons))
	reasons := make([]string, 0, len(details.RulesBracketReasons))
	for _, reason := range details.RulesBracketReasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		if _, exists := seen[reason]; exists {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	details.RulesBracketReasons = reasons
	details.EvaluatedBracketReason = strings.TrimSpace(details.EvaluatedBracketReason)
	return details
}

func normalizeMetrics(raw map[string]string) map[string]any {
	metrics := make(map[string]any)
	for label, value := range raw {
		key := ""
		switch {
		case strings.Contains(label, "Power Level"):
			key = "power_level"
		case strings.Contains(label, "Tipping Point"):
			key = "tipping_point"
		case strings.Contains(label, "Efficiency"):
			key = "efficiency"
		case strings.Contains(label, "Impact"):
			key = "impact"
		case strings.Contains(label, "Score"):
			key = "score"
		case strings.Contains(label, "Average Playability"):
			key = "average_playability"
		case strings.Contains(label, "Rules Bracket"):
			key = "rules_bracket"
		case strings.Contains(label, "Evaluated Bracket"):
			key = "evaluated_bracket"
		}
		if key == "" {
			continue
		}
		cleaned := strings.TrimSpace(value)
		cleaned = strings.TrimSuffix(cleaned, "/ 10")
		cleaned = strings.TrimSuffix(cleaned, "/ 1000")
		cleaned = strings.TrimSuffix(cleaned, "%")
		if key == "evaluated_bracket" {
			if index := strings.LastIndex(cleaned, ":"); index >= 0 {
				cleaned = strings.TrimSpace(cleaned[index+1:])
			}
		}
		if number, err := strconv.ParseFloat(strings.TrimSpace(cleaned), 64); err == nil {
			metrics[key] = number
		} else {
			metrics[key] = value
		}
	}
	return metrics
}
