package scanner

import (
	"context"
	"fmt"
	"strings"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

var presetVolatilePhase1 = []string{
	"GBP_JPY", "EUR_JPY", "AUD_JPY", "GBP_AUD", "EUR_GBP",
	"GBP_USD", "EUR_USD", "USD_JPY", "AUD_USD", "NZD_USD",
	"XAU_USD", "XAG_USD", "BTC_USD",
	"NAS100_USD", "US30_USD", "SPX500_USD", "US2000_USD",
	"BCO_USD", "WTICO_USD",
}

var presetExpanded = append([]string(nil), presetVolatilePhase1...)

func PresetSymbols(preset string) []string {
	switch preset {
	case config.PresetVolatilePhase1:
		return append([]string(nil), presetVolatilePhase1...)
	case config.PresetExpanded:
		return append([]string(nil), presetExpanded...)
	default:
		return append([]string(nil), presetVolatilePhase1...)
	}
}

type Universe struct {
	Symbols []string
	Meta    map[string]oanda.Instrument
}

func ResolveUniverse(ctx context.Context, cfg *config.Config, client *oanda.Client) (Universe, error) {
	all, err := client.ListInstruments(ctx)
	if err != nil {
		return Universe{}, fmt.Errorf("list instruments: %w", err)
	}
	meta := make(map[string]oanda.Instrument, len(all))
	for _, inst := range all {
		meta[inst.Name] = inst
	}

	var symbols []string
	switch cfg.Scanner.UniverseMode {
	case config.UniverseModeWatchlist:
		if len(cfg.Scanner.Watchlist) > 0 {
			symbols = append([]string(nil), cfg.Scanner.Watchlist...)
		} else {
			symbols = PresetSymbols(cfg.Scanner.UniversePreset)
		}
	case config.UniverseModePreset:
		if len(cfg.Scanner.Watchlist) > 0 {
			symbols = append([]string(nil), cfg.Scanner.Watchlist...)
		} else {
			symbols = PresetSymbols(cfg.Scanner.UniversePreset)
		}
	case config.UniverseModeAccount:
		symbols = filterAccountUniverse(all, cfg.Scanner.UniverseFilters)
	default:
		symbols = PresetSymbols(cfg.Scanner.UniversePreset)
	}

	out := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		sym = strings.ToUpper(strings.TrimSpace(sym))
		if sym == "" {
			continue
		}
		if _, ok := meta[sym]; !ok {
			continue
		}
		out = append(out, sym)
	}
	if len(out) == 0 {
		return Universe{}, fmt.Errorf("universe resolved to zero tradeable instruments")
	}
	return Universe{Symbols: out, Meta: meta}, nil
}

func filterAccountUniverse(all []oanda.Instrument, f config.UniverseFiltersConfig) []string {
	typeSet := make(map[string]struct{}, len(f.Types))
	for _, t := range f.Types {
		typeSet[strings.ToUpper(t)] = struct{}{}
	}
	excludeQuote := make(map[string]struct{}, len(f.ExcludeQuoteCurrencies))
	for _, q := range f.ExcludeQuoteCurrencies {
		excludeQuote[strings.ToUpper(q)] = struct{}{}
	}

	var out []string
	for _, inst := range all {
		if len(typeSet) > 0 {
			if _, ok := typeSet[strings.ToUpper(inst.Type)]; !ok {
				continue
			}
		}
		parts := strings.Split(inst.Name, "_")
		if len(parts) == 2 {
			if _, ok := excludeQuote[strings.ToUpper(parts[1])]; ok {
				continue
			}
		}
		if f.ExcludeExotics && isExotic(inst.Name) {
			continue
		}
		out = append(out, inst.Name)
	}
	return out
}

func isExotic(name string) bool {
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		return false
	}
	majors := map[string]struct{}{
		"USD": {}, "EUR": {}, "GBP": {}, "JPY": {}, "AUD": {}, "NZD": {}, "CAD": {}, "CHF": {},
	}
	_, qMajor := majors[parts[1]]
	_, bMajor := majors[parts[0]]
	return !qMajor || !bMajor
}
