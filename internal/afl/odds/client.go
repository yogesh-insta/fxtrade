package odds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/afl"
	"github.com/ym/fxtrade/internal/config"
)

// Client fetches AFL head-to-head odds from The Odds API.
type Client struct {
	baseURL    string
	apiKey     string
	sportKey   string
	regions    string
	markets    string
	httpClient *http.Client
}

func NewClient(cfg config.AFLConfig) *Client {
	timeout := 30 * time.Second
	return &Client{
		baseURL:    strings.TrimRight(cfg.OddsAPIBaseURL, "/"),
		apiKey:     cfg.OddsAPIKey,
		sportKey:   cfg.OddsSportKey,
		regions:    cfg.OddsRegions,
		markets:    cfg.OddsMarkets,
		httpClient: &http.Client{Timeout: timeout},
	}
}

type apiOutcome struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Point float64 `json:"point"`
}

type apiEvent struct {
	ID           string    `json:"id"`
	SportKey     string    `json:"sport_key"`
	CommenceTime time.Time `json:"commence_time"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
	Bookmakers   []struct {
		Key        string    `json:"key"`
		Title      string    `json:"title"`
		LastUpdate time.Time `json:"last_update"`
		Markets    []struct {
			Key        string       `json:"key"`
			LastUpdate time.Time    `json:"last_update"`
			Outcomes   []apiOutcome `json:"outcomes"`
		} `json:"markets"`
	} `json:"bookmakers"`
}

// FetchResult holds head-to-head and totals markets from The Odds API.
type FetchResult struct {
	H2H    []afl.MarketOdds
	Totals []afl.TotalsOdds
}

// FetchOdds returns h2h and totals markets for all upcoming AFL events.
func (c *Client) FetchOdds(ctx context.Context, resolve func(string) (afl.TeamID, error)) (FetchResult, error) {
	u, err := url.Parse(fmt.Sprintf("%s/sports/%s/odds", c.baseURL, c.sportKey))
	if err != nil {
		return FetchResult{}, err
	}
	q := u.Query()
	q.Set("apiKey", c.apiKey)
	q.Set("regions", c.regions)
	q.Set("markets", c.markets)
	q.Set("oddsFormat", "decimal")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FetchResult{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("odds api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return FetchResult{}, fmt.Errorf("odds api status %d: %s", resp.StatusCode, string(body))
	}

	remaining := resp.Header.Get("x-requests-remaining")
	if remaining != "" {
		// logged by caller if needed
		_ = remaining
	}

	var events []apiEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return FetchResult{}, fmt.Errorf("decode odds response: %w", err)
	}
	return parseEvents(events, resolve), nil
}

func parseEvents(events []apiEvent, resolve func(string) (afl.TeamID, error)) FetchResult {
	var h2h []afl.MarketOdds
	var totals []afl.TotalsOdds
	for _, ev := range events {
		homeID, err := resolve(ev.HomeTeam)
		if err != nil {
			continue
		}
		awayID, err := resolve(ev.AwayTeam)
		if err != nil {
			continue
		}
		for _, bm := range ev.Bookmakers {
			updated := bm.LastUpdate
			if updated.IsZero() {
				updated = time.Now().UTC()
			}
			for _, mkt := range bm.Markets {
				if !mkt.LastUpdate.IsZero() {
					updated = mkt.LastUpdate
				}
				switch mkt.Key {
				case "h2h":
					for _, oc := range mkt.Outcomes {
						teamID, err := resolve(oc.Name)
						if err != nil {
							continue
						}
						h2h = append(h2h, afl.MarketOdds{
							EventID:     ev.ID,
							HomeTeam:    homeID,
							AwayTeam:    awayID,
							Bookmaker:   bm.Key,
							Team:        teamID,
							DecimalOdds: oc.Price,
							UpdatedAt:   updated,
							Kickoff:     ev.CommenceTime,
						})
					}
				case "totals":
					line, over, under := parseTotalsOutcomes(mkt.Outcomes)
					if line <= 0 {
						continue
					}
					totals = append(totals, afl.TotalsOdds{
						EventID:    ev.ID,
						HomeTeam:   homeID,
						AwayTeam:   awayID,
						Bookmaker:  bm.Key,
						Line:       line,
						OverPrice:  over,
						UnderPrice: under,
						UpdatedAt:  updated,
						Kickoff:    ev.CommenceTime,
					})
				}
			}
		}
	}
	return FetchResult{H2H: h2h, Totals: totals}
}

func parseTotalsOutcomes(outcomes []apiOutcome) (line, over, under float64) {
	for _, oc := range outcomes {
		if oc.Point > 0 {
			line = oc.Point
		}
		name := strings.ToLower(strings.TrimSpace(oc.Name))
		switch {
		case name == "over":
			over = oc.Price
		case name == "under":
			under = oc.Price
		case strings.HasPrefix(name, "over "):
			fmt.Sscanf(name, "over %f", &line)
			over = oc.Price
		case strings.HasPrefix(name, "under "):
			fmt.Sscanf(name, "under %f", &line)
			under = oc.Price
		}
	}
	return line, over, under
}

// ParseEvents decodes API JSON for tests.
func ParseEvents(data []byte, resolve func(string) (afl.TeamID, error)) (FetchResult, error) {
	var events []apiEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return FetchResult{}, err
	}
	return parseEvents(events, resolve), nil
}
