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
			Key        string `json:"key"`
			LastUpdate time.Time `json:"last_update"`
			Outcomes   []struct {
				Name  string  `json:"name"`
				Price float64 `json:"price"`
			} `json:"outcomes"`
		} `json:"markets"`
	} `json:"bookmakers"`
}

// FetchOdds returns market odds for all upcoming AFL events.
func (c *Client) FetchOdds(ctx context.Context, resolve func(string) (afl.TeamID, error)) ([]afl.MarketOdds, error) {
	u, err := url.Parse(fmt.Sprintf("%s/sports/%s/odds", c.baseURL, c.sportKey))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("apiKey", c.apiKey)
	q.Set("regions", c.regions)
	q.Set("markets", c.markets)
	q.Set("oddsFormat", "decimal")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("odds api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("odds api status %d: %s", resp.StatusCode, string(body))
	}

	remaining := resp.Header.Get("x-requests-remaining")
	if remaining != "" {
		// logged by caller if needed
		_ = remaining
	}

	var events []apiEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("decode odds response: %w", err)
	}

	var out []afl.MarketOdds
	for _, ev := range events {
		homeID, err := resolve(ev.HomeTeam)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", ev.ID, err)
		}
		awayID, err := resolve(ev.AwayTeam)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", ev.ID, err)
		}
		for _, bm := range ev.Bookmakers {
			updated := bm.LastUpdate
			if updated.IsZero() {
				updated = time.Now().UTC()
			}
			for _, mkt := range bm.Markets {
				if mkt.Key != "h2h" {
					continue
				}
				if !mkt.LastUpdate.IsZero() {
					updated = mkt.LastUpdate
				}
				for _, oc := range mkt.Outcomes {
					teamID, err := resolve(oc.Name)
					if err != nil {
						continue
					}
					out = append(out, afl.MarketOdds{
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
			}
		}
	}
	return out, nil
}

// ParseEvents decodes API JSON for tests.
func ParseEvents(data []byte, resolve func(string) (afl.TeamID, error)) ([]afl.MarketOdds, error) {
	var events []apiEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	c := &Client{}
	var out []afl.MarketOdds
	for _, ev := range events {
		homeID, err := resolve(ev.HomeTeam)
		if err != nil {
			return nil, err
		}
		awayID, err := resolve(ev.AwayTeam)
		if err != nil {
			return nil, err
		}
		for _, bm := range ev.Bookmakers {
			for _, mkt := range bm.Markets {
				if mkt.Key != "h2h" {
					continue
				}
				for _, oc := range mkt.Outcomes {
					teamID, err := resolve(oc.Name)
					if err != nil {
						continue
					}
					out = append(out, afl.MarketOdds{
						EventID:     ev.ID,
						HomeTeam:    homeID,
						AwayTeam:    awayID,
						Bookmaker:   bm.Key,
						Team:        teamID,
						DecimalOdds: oc.Price,
						UpdatedAt:   time.Now().UTC(),
						Kickoff:     ev.CommenceTime,
					})
				}
			}
		}
	}
	_ = c
	return out, nil
}
