// Command reddit-smoke verifies Reddit OAuth + post fetch for the agent.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/config"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if !cfg.RedditConfigured {
		fmt.Println("Reddit is NOT configured.")
		fmt.Println()
		fmt.Println("1. Open https://www.reddit.com/prefs/apps")
		fmt.Println("2. Create app → type \"script\" → redirect http://localhost:8080")
		fmt.Println("3. Copy client id (under app name) and secret into .credentials → btc_sentiment:")
		fmt.Println(`     "reddit_client_id": "...",`)
		fmt.Println(`     "reddit_client_secret": "...",`)
		fmt.Println(`     "reddit_user_agent": "fxtrade:btc-sentiment:1.0 (by /u/YOUR_USERNAME)",`)
		fmt.Println("4. Re-run: go run ./cmd/reddit-smoke")
		if cfg.CredentialsPath != "" {
			fmt.Println()
			fmt.Println("credentials file:", cfg.CredentialsPath)
		}
		os.Exit(2)
	}

	client := fetchers.NewRedditClient(cfg.RedditClientID, cfg.RedditClientSecret, cfg.RedditUserAgent, &http.Client{Timeout: 30 * time.Second})
	end := time.Now().UTC()
	start := end.Add(-cfg.Window)
	posts, err := client.FetchRedditPosts(context.Background(), cfg.RedditSubreddits, start, end)
	if err != nil {
		log.Error("fetch failed", "err", err)
		os.Exit(1)
	}
	fmt.Printf("OK: fetched %d posts from %v in [%s, %s]\n", len(posts), cfg.RedditSubreddits, start.Format(time.RFC3339), end.Format(time.RFC3339))
	n := len(posts)
	if n > 5 {
		n = 5
	}
	for i := 0; i < n; i++ {
		p := posts[i]
		preview := p.Text
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		fmt.Printf("  - r/%s ups=%d %s | %s\n", p.Subreddit, p.Upvotes, p.Timestamp.Format(time.RFC3339), preview)
	}
}
