package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ym/fxtrade/internal/afl"
)

// Client fetches forecast data from Open-Meteo (no API key).
type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 15 * time.Second}}
}

type forecastResponse struct {
	Hourly struct {
		Time               []string  `json:"time"`
		Precipitation      []float64 `json:"precipitation"`
		WindSpeed10m       []float64 `json:"windspeed_10m"`
		RelativeHumidity2m []float64 `json:"relativehumidity_2m"`
	} `json:"hourly"`
}

// FetchForVenue returns weather metrics closest to kickoff at venue coordinates.
func (c *Client) FetchForVenue(ctx context.Context, venue afl.VenueProfile, kickoff time.Time) (afl.WeatherMetrics, error) {
	u, _ := url.Parse("https://api.open-meteo.com/v1/forecast")
	q := u.Query()
	q.Set("latitude", fmt.Sprintf("%.4f", venue.Latitude))
	q.Set("longitude", fmt.Sprintf("%.4f", venue.Longitude))
	q.Set("hourly", "precipitation,windspeed_10m,relativehumidity_2m")
	q.Set("timezone", "auto")
	q.Set("forecast_days", "3")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return afl.WeatherMetrics{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return afl.WeatherMetrics{}, fmt.Errorf("open-meteo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return afl.WeatherMetrics{}, fmt.Errorf("open-meteo status %d", resp.StatusCode)
	}

	var fc forecastResponse
	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return afl.WeatherMetrics{}, err
	}

	target := kickoff.UTC().Format("2006-01-02T15")
	var rain, wind, humidity float64
	found := false
	for i, t := range fc.Hourly.Time {
		if len(t) >= 13 && t[:13] == target[:13] {
			if i < len(fc.Hourly.Precipitation) {
				rain = fc.Hourly.Precipitation[i]
			}
			if i < len(fc.Hourly.WindSpeed10m) {
				wind = fc.Hourly.WindSpeed10m[i]
			}
			if i < len(fc.Hourly.RelativeHumidity2m) {
				humidity = fc.Hourly.RelativeHumidity2m[i]
			}
			found = true
			break
		}
	}
	if !found && len(fc.Hourly.Precipitation) > 0 {
		rain = fc.Hourly.Precipitation[0]
		wind = fc.Hourly.WindSpeed10m[0]
		humidity = fc.Hourly.RelativeHumidity2m[0]
	}

	contest := afl.Clamp01(rain/8.0 + wind/40.0)
	totalPts := 1.0 - contest*0.25
	return afl.WeatherMetrics{
		RainMM:              rain,
		WindKPH:             wind,
		Humidity:            humidity,
		ContestFavorability: contest,
		TotalPointsFactor:   totalPts,
	}, nil
}

// DefaultMetrics returns neutral weather when API is unavailable.
func DefaultMetrics() afl.WeatherMetrics {
	return afl.WeatherMetrics{
		ContestFavorability: 0.2,
		TotalPointsFactor:   1.0,
	}
}
