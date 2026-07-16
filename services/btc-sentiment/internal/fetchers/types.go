package fetchers

import (
	"context"
	"time"
)

// NewsItem is a single news headline.
type NewsItem struct {
	Headline  string
	Source    string
	Timestamp time.Time
	URL       string
}

// RedditPost is a single Reddit post within the lookback window.
type RedditPost struct {
	Text      string
	Subreddit string
	Upvotes   int
	Timestamp time.Time
}

// NewsFetcher fetches news headlines for a time window.
type NewsFetcher interface {
	FetchNews(ctx context.Context, start, end time.Time) ([]NewsItem, error)
}

// RedditFetcher fetches Reddit posts for a time window.
type RedditFetcher interface {
	FetchRedditPosts(ctx context.Context, subreddits []string, start, end time.Time) ([]RedditPost, error)
}
