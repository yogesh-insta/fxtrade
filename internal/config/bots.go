package config

import "fmt"

const (
	BotUniverseScanner = "universe_scanner"
	BotFxSentiment     = "fx_sentiment"
	BotBtcCfd          = "btc_cfd"

	// Deprecated: use BotFxSentiment. Accepted as alias for one release.
	BotRangeTrend = "range_trend"
)

type BotsConfig struct {
	Enabled []string `json:"enabled"`
}

// NormalizeBotID maps deprecated bot IDs to their canonical form.
func NormalizeBotID(id string) string {
	if id == BotRangeTrend {
		return BotFxSentiment
	}
	return id
}

func (c *Config) EnabledBots() []string {
	if len(c.Bots.Enabled) > 0 {
		out := make([]string, 0, len(c.Bots.Enabled))
		for _, id := range c.Bots.Enabled {
			out = append(out, NormalizeBotID(id))
		}
		return out
	}
	// Backward compatibility with strategy.mode switch.
	if c.ScannerEnabled() {
		return []string{BotUniverseScanner}
	}
	if c.Strategy.Enabled {
		return []string{BotFxSentiment}
	}
	return nil
}

func (c *Config) ValidateBots() error {
	known := map[string]struct{}{
		BotUniverseScanner: {},
		BotFxSentiment:     {},
		BotBtcCfd:          {},
		BotRangeTrend:      {}, // deprecated alias
	}
	seen := make(map[string]struct{}, len(c.Bots.Enabled))
	for _, id := range c.Bots.Enabled {
		if id == "" {
			return fmt.Errorf("bots.enabled must not contain empty strings")
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("unknown bot %q (known: %s, %s, %s)", id, BotUniverseScanner, BotFxSentiment, BotBtcCfd)
		}
		normalized := NormalizeBotID(id)
		if _, dup := seen[normalized]; dup {
			return fmt.Errorf("duplicate bot %q in bots.enabled", normalized)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func StateFileForBot(basePath, botID string) string {
	if basePath == "" {
		basePath = "data/state.json"
	}
	botID = NormalizeBotID(botID)
	if botID == "" {
		return basePath
	}
	// data/state.json -> data/state-universe_scanner.json
	dir := ""
	name := basePath
	if i := len(basePath) - 1; i >= 0 {
		for j := i; j >= 0; j-- {
			if basePath[j] == '/' {
				dir = basePath[:j+1]
				name = basePath[j+1:]
				break
			}
		}
	}
	ext := ".json"
	if dot := len(name) - 5; dot > 0 && name[dot:] == ".json" {
		ext = name[dot:]
		name = name[:dot]
	}
	return dir + name + "-" + botID + ext
}

func TradesDBForBot(botID string) string {
	botID = NormalizeBotID(botID)
	switch botID {
	case BotBtcCfd:
		return "data/btc_cfd/trades.db"
	case BotFxSentiment:
		return "data/fx_sentiment/trades.db"
	case BotUniverseScanner:
		return "data/universe_scanner/trades.db"
	default:
		return "data/" + botID + "/trades.db"
	}
}

func JournalDirForBot(botID string) string {
	botID = NormalizeBotID(botID)
	switch botID {
	case BotBtcCfd:
		return "logs/btc_cfd"
	case BotFxSentiment:
		return "logs/fx_sentiment"
	case BotUniverseScanner:
		return "logs/universe_scanner"
	default:
		return "logs/" + botID
	}
}

func HaltFileForBot(basePath, botID string) string {
	if basePath == "" {
		basePath = DefaultHaltFile
	}
	botID = NormalizeBotID(botID)
	if botID == "" {
		return basePath
	}
	return basePath + "." + botID
}
