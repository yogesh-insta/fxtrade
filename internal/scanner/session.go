package scanner

import (
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func assetClass(cfg config.ScannerConfig, instrument string, instType string) string {
	cls := oanda.InstrumentClass(instrument, instType)
	if cls == "CRYPTO" {
		return "CRYPTO"
	}
	if _, ok := cfg.AssetClasses[cls]; ok {
		return cls
	}
	if strings.Contains(strings.ToUpper(instrument), "JPY") {
		if _, ok := cfg.AssetClasses["JPY"]; ok {
			return "JPY"
		}
	}
	return "FX"
}

func sessionSpec(cfg config.ScannerConfig, instrument string, instType string) string {
	cls := assetClass(cfg, instrument, instType)
	if ac, ok := cfg.AssetClasses[cls]; ok && ac.SessionUTC != "" {
		return ac.SessionUTC
	}
	return "08:00-17:00"
}

func parseHM(s string) (hour, min int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q", s)
	}
	if _, err = fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, 0, err
	}
	if _, err = fmt.Sscanf(parts[1], "%d", &min); err != nil {
		return 0, 0, err
	}
	return hour, min, nil
}

func sessionBoundsUTC(spec string, day time.Time) (start, end time.Time, always bool, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "24/7" {
		y, mo, d := day.UTC().Date()
		start = time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
		end = start.Add(24 * time.Hour)
		return start, end, true, nil
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, false, fmt.Errorf("invalid session %q", spec)
	}
	sh, sm, err := parseHM(strings.TrimSpace(parts[0]))
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	eh, em, err := parseHM(strings.TrimSpace(parts[1]))
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	y, mo, d := day.UTC().Date()
	start = time.Date(y, mo, d, sh, sm, 0, 0, time.UTC)
	end = time.Date(y, mo, d, eh, em, 0, 0, time.UTC)
	if !end.After(start) {
		end = end.Add(24 * time.Hour)
	}
	return start, end, false, nil
}

func InSession(cfg config.ScannerConfig, instrument, instType string, now time.Time) bool {
	start, end, _, err := sessionBoundsUTC(sessionSpec(cfg, instrument, instType), now)
	if err != nil {
		return false
	}
	t := now.UTC()
	return !t.Before(start) && t.Before(end)
}

func SessionOpen(cfg config.ScannerConfig, instrument, instType string, now time.Time) time.Time {
	start, _, _, err := sessionBoundsUTC(sessionSpec(cfg, instrument, instType), now)
	if err != nil {
		return time.Time{}
	}
	return start
}

func ForceFlatUTC(cls string, day time.Time) (time.Time, bool) {
	y, mo, d := day.UTC().Date()
	switch cls {
	case "CRYPTO":
		return time.Time{}, false
	case "INDEX", "ENERGY":
		return time.Date(y, mo, d, 20, 0, 0, 0, time.UTC), true
	default:
		return time.Date(y, mo, d, 21, 30, 0, 0, time.UTC), true
	}
}

func ShouldForceFlat(cfg config.ScannerConfig, instrument, instType string, now time.Time) bool {
	cls := assetClass(cfg, instrument, instType)
	at, ok := ForceFlatUTC(cls, now)
	if !ok {
		return false
	}
	return now.UTC().After(at) || now.UTC().Equal(at)
}
