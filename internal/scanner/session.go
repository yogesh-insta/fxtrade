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

func forceFlatHM(cfg config.ScannerConfig, cls string) (hour, min int, enabled bool) {
	spec := ""
	if cfg.ForceFlatUTC != nil {
		spec = strings.TrimSpace(cfg.ForceFlatUTC[cls])
	}
	if spec == "" || strings.EqualFold(spec, "off") || strings.EqualFold(spec, "none") {
		return 0, 0, false
	}
	h, m, err := parseHM(spec)
	if err != nil {
		return 0, 0, false
	}
	return h, m, true
}

func ForceFlatUTC(cfg config.ScannerConfig, cls string, day time.Time) (time.Time, bool) {
	h, m, ok := forceFlatHM(cfg, cls)
	if !ok {
		return time.Time{}, false
	}
	y, mo, d := day.UTC().Date()
	return time.Date(y, mo, d, h, m, 0, 0, time.UTC), true
}

func ShouldForceFlat(cfg config.ScannerConfig, instrument, instType string, now time.Time) bool {
	cls := assetClass(cfg, instrument, instType)
	at, ok := ForceFlatUTC(cfg, cls, now)
	if !ok {
		return false
	}
	return !now.UTC().Before(at)
}

// PastEntryCutoff blocks new entries within N minutes of force-flat for the asset class.
func PastEntryCutoff(cfg config.ScannerConfig, instrument, instType string, now time.Time) bool {
	mins := cfg.EntryCutoffBeforeForceFlatMinutes
	if mins <= 0 {
		return false
	}
	cls := assetClass(cfg, instrument, instType)
	at, ok := ForceFlatUTC(cfg, cls, now)
	if !ok {
		return false
	}
	cutoff := at.Add(-time.Duration(mins) * time.Minute)
	return !now.UTC().Before(cutoff)
}
