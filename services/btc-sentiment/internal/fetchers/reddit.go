package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	redditTokenURL = "https://www.reddit.com/api/v1/access_token"
	redditUA       = "btc-sentiment/1.0 (Cloud Run sentiment service)"
)

// RedditClient fetches posts via Reddit OAuth2 client-credentials flow.
type RedditClient struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
	UserAgent    string

	mu    sync.Mutex
	token string
	exp   time.Time
}

// NewRedditClient creates a Reddit OAuth client.
func NewRedditClient(clientID, clientSecret, userAgent string, httpClient *http.Client) *RedditClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = redditUA
	}
	return &RedditClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTPClient:   httpClient,
		UserAgent:    userAgent,
	}
}

type redditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				Title       string  `json:"title"`
				Selftext    string  `json:"selftext"`
				Subreddit   string  `json:"subreddit"`
				Ups         int     `json:"ups"`
				CreatedUTC  float64 `json:"created_utc"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// FetchRedditPosts returns posts from the given subreddits in [start, end].
func (c *RedditClient) FetchRedditPosts(ctx context.Context, subreddits []string, start, end time.Time) ([]RedditPost, error) {
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" {
		return nil, NewFetchError("FetchRedditPosts", fmt.Errorf("REDDIT_CLIENT_ID and REDDIT_CLIENT_SECRET are required"))
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, NewFetchError("FetchRedditPosts", err)
	}

	var posts []RedditPost
	var lastErr error
	for _, sub := range subreddits {
		sub = strings.TrimPrefix(strings.TrimSpace(sub), "r/")
		if sub == "" {
			continue
		}
		u := fmt.Sprintf("https://oauth.reddit.com/r/%s/new?limit=100", url.PathEscape(sub))
		body, _, err := HTTPDo(ctx, c.HTTPClient, http.MethodGet, u, nil, func(req *http.Request) {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("User-Agent", c.UserAgent)
		})
		if err != nil {
			lastErr = err
			continue
		}

		var listing redditListing
		if err := json.Unmarshal(body, &listing); err != nil {
			lastErr = fmt.Errorf("decode %s: %w", sub, err)
			continue
		}
		for _, child := range listing.Data.Children {
			d := child.Data
			ts := time.Unix(int64(d.CreatedUTC), 0).UTC()
			if ts.Before(start) || ts.After(end) {
				continue
			}
			text := strings.TrimSpace(d.Title)
			if st := strings.TrimSpace(d.Selftext); st != "" {
				text = text + "\n" + st
			}
			if text == "" {
				continue
			}
			posts = append(posts, RedditPost{
				Text:      text,
				Subreddit: d.Subreddit,
				Upvotes:    d.Ups,
				Timestamp: ts,
			})
		}
	}

	if len(posts) == 0 {
		if lastErr != nil {
			return nil, NewFetchError("FetchRedditPosts", lastErr)
		}
		return nil, NewNoDataError("FetchRedditPosts")
	}
	return posts, nil
}

func (c *RedditClient) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.exp) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	body, _, err := HTTPDo(ctx, c.HTTPClient, http.MethodPost, redditTokenURL, []byte(form.Encode()), func(req *http.Request) {
		req.SetBasicAuth(c.ClientID, c.ClientSecret)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", c.UserAgent)
	})
	if err != nil {
		return "", fmt.Errorf("reddit token: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("reddit token decode: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("reddit token empty")
	}
	c.token = tr.AccessToken
	expires := tr.ExpiresIn
	if expires <= 0 {
		expires = 3600
	}
	c.exp = time.Now().Add(time.Duration(expires)*time.Second - time.Minute)
	return c.token, nil
}
