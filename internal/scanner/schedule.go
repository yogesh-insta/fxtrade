package scanner

import (
	"strings"
	"time"
)

func trendLabel(bias int) string {
	switch bias {
	case 1:
		return "bullish"
	case -1:
		return "bearish"
	default:
		return "neutral"
	}
}

func parseScheduleUTC(spec string, now time.Time) (time.Time, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return time.Time{}, false
	}
	parts := strings.Fields(spec)
	hm := spec
	weekday := -1
	if len(parts) == 2 {
		weekday = weekdayNum(parts[0])
		hm = parts[1]
	}
	hour, min, err := parseHM(hm)
	if err != nil {
		return time.Time{}, false
	}
	y, mo, d := now.UTC().Date()
	t := time.Date(y, mo, d, hour, min, 0, 0, time.UTC)
	if weekday >= 0 && int(now.UTC().Weekday()) != weekday {
		return time.Time{}, false
	}
	return t, true
}

func weekdayNum(s string) int {
	switch strings.ToLower(s) {
	case "sun", "sunday":
		return 0
	case "mon", "monday":
		return 1
	case "tue", "tuesday":
		return 2
	case "wed", "wednesday":
		return 3
	case "thu", "thursday":
		return 4
	case "fri", "friday":
		return 5
	case "sat", "saturday":
		return 6
	default:
		return -1
	}
}

func shouldFireScheduled(last time.Time, spec string, now time.Time, window time.Duration) bool {
	at, ok := parseScheduleUTC(spec, now)
	if !ok {
		return false
	}
	if now.Before(at) || now.Sub(at) > window {
		return false
	}
	if last.IsZero() {
		return true
	}
	if strings.Contains(strings.ToLower(spec), "fri") {
		return last.Before(at)
	}
	return last.UTC().Format("2006-01-02") != now.UTC().Format("2006-01-02")
}
