package config

import "fmt"

const (
	BotUniverseScanner = "universe_scanner"
	BotRangeTrend      = "range_trend"
)

type BotsConfig struct {
	Enabled []string `json:"enabled"`
}

func (c *Config) EnabledBots() []string {
	if len(c.Bots.Enabled) > 0 {
		return append([]string(nil), c.Bots.Enabled...)
	}
	// Backward compatibility with strategy.mode switch.
	if c.ScannerEnabled() {
		return []string{BotUniverseScanner}
	}
	if c.Strategy.Enabled {
		return []string{BotRangeTrend}
	}
	return nil
}

func (c *Config) ValidateBots() error {
	known := map[string]struct{}{
		BotUniverseScanner: {},
		BotRangeTrend:      {},
	}
	seen := make(map[string]struct{}, len(c.Bots.Enabled))
	for _, id := range c.Bots.Enabled {
		if id == "" {
			return fmt.Errorf("bots.enabled must not contain empty strings")
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("unknown bot %q (known: %s, %s)", id, BotUniverseScanner, BotRangeTrend)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate bot %q in bots.enabled", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func StateFileForBot(basePath, botID string) string {
	if basePath == "" {
		basePath = "data/state.json"
	}
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

func HaltFileForBot(basePath, botID string) string {
	if basePath == "" {
		basePath = DefaultHaltFile
	}
	if botID == "" {
		return basePath
	}
	return basePath + "." + botID
}
