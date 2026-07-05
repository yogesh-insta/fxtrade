package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/health"
	"github.com/ym/fxtrade/internal/notify"
)

const (
	alertSubject   = "fxtrade: unhealthy"
	recoverSubject = "fxtrade: recovered"
	timerSubject   = "fxtrade: scheduled job failed"
)

var timerServices = map[string]string{
	"nifty":   "nifty-pulse.service",
	"afl":     "afl-pulse.service",
	"pregame": "afl-pulse-pregame.service",
}

type watchState struct {
	HealthIncident string               `json:"health_incident,omitempty"`
	HealthAlerted  bool                 `json:"health_alerted,omitempty"`
	Timers         map[string]timerState `json:"timers,omitempty"`
}

type timerState struct {
	Incident string `json:"incident,omitempty"`
	Alerted  bool   `json:"alerted,omitempty"`
}

func main() {
	credentials := flag.String("credentials", ".credentials", "path to credentials JSON")
	healthURL := flag.String("health-url", "http://127.0.0.1:8080/health", "fxtrade health endpoint")
	stateFile := flag.String("state-file", "", "alert dedupe state file (default: <install>/data/health-watch.state.json)")
	staleTicks := flag.Duration("stale-ticks", health.DefaultStaleTickAge, "max age for last price tick during FX hours")
	checkTimers := flag.String("check-timers", "", "check systemd job failure: nifty, afl, or pregame")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentials)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if !cfg.Email.Enabled() {
		slog.Error("email not configured (need smtp_host, username, password, alert_to)")
		os.Exit(1)
	}

	path := *stateFile
	if path == "" {
		path = defaultStateFile(*credentials)
	}
	state := loadState(path)
	notifier := notify.New(cfg.Email)

	if *checkTimers != "" {
		if err := runTimerCheck(*checkTimers, notifier, state, path); err != nil {
			slog.Error("timer check", "job", *checkTimers, "error", err)
			os.Exit(1)
		}
		return
	}

	if err := runHealthCheck(*healthURL, notifier, state, path, *staleTicks); err != nil {
		slog.Error("health check", "error", err)
		os.Exit(1)
	}
}

func defaultStateFile(credentialsPath string) string {
	dir := filepath.Dir(credentialsPath)
	if dir == "." {
		dir = "data"
	} else {
		dir = filepath.Join(dir, "data")
	}
	return filepath.Join(dir, "health-watch.state.json")
}

func runHealthCheck(url string, notifier notify.Notifier, state *watchState, statePath string, staleAge time.Duration) error {
	now := time.Now()
	st, fetchErr := fetchHealth(url)
	var issues []string
	if fetchErr != nil {
		issues = []string{fmt.Sprintf("health endpoint unreachable (%s): %v", url, fetchErr)}
	} else {
		var ok bool
		ok, issues = health.Evaluate(st, now, staleAge)
		if ok {
			issues = nil
		}
	}

	incident := "ok"
	if len(issues) > 0 {
		incident = health.IncidentKey(issues)
	}

	if incident == "ok" {
		if state.HealthAlerted {
			body := fmt.Sprintf("The fxtrade daemon is healthy again.\n\nEndpoint: %s\nChecked: %s\n", url, now.UTC().Format(time.RFC3339))
			notifier.Send(context.Background(), recoverSubject, body)
			state.HealthIncident = ""
			state.HealthAlerted = false
			return saveState(statePath, state)
		}
		slog.Info("health ok", "url", url)
		return nil
	}

	if state.HealthAlerted && state.HealthIncident == incident {
		slog.Info("health still unhealthy, alert already sent", "incident", incident)
		return nil
	}

	body := fmt.Sprintf("The fxtrade watchdog detected a problem.\n\nEndpoint: %s\nChecked: %s\n\nIssues:\n- %s\n",
		url, now.UTC().Format(time.RFC3339), strings.Join(issues, "\n- "))
	notifier.Send(context.Background(), alertSubject, body)
	state.HealthIncident = incident
	state.HealthAlerted = true
	slog.Warn("health alert sent", "incident", incident)
	return saveState(statePath, state)
}

func runTimerCheck(job string, notifier notify.Notifier, state *watchState, statePath string) error {
	service, ok := timerServices[job]
	if !ok {
		return fmt.Errorf("unknown job %q (want nifty, afl, or pregame)", job)
	}

	failed, detail, err := systemdFailed(service)
	if err != nil {
		return err
	}
	if state.Timers == nil {
		state.Timers = make(map[string]timerState)
	}
	prev := state.Timers[service]

	if !failed {
		if prev.Alerted {
			body := fmt.Sprintf("%s is healthy again.\n\nChecked: %s\n", service, time.Now().UTC().Format(time.RFC3339))
			notifier.Send(context.Background(), "fxtrade: scheduled job recovered", body)
		}
		delete(state.Timers, service)
		slog.Info("timer ok", "service", service)
		return saveState(statePath, state)
	}

	incident := detail
	if incident == "" {
		incident = "failed"
	}
	if prev.Alerted && prev.Incident == incident {
		slog.Info("timer still failed, alert already sent", "service", service)
		return nil
	}

	body := fmt.Sprintf("A scheduled fxtrade job failed on the VM.\n\nService: %s\nChecked: %s\n\nDetail:\n%s\n",
		service, time.Now().UTC().Format(time.RFC3339), detail)
	notifier.Send(context.Background(), timerSubject, body)
	state.Timers[service] = timerState{Incident: incident, Alerted: true}
	slog.Warn("timer alert sent", "service", service)
	return saveState(statePath, state)
}

func fetchHealth(url string) (health.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return health.Status{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return health.Status{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return health.Status{}, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		return health.Status{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var st health.Status
	if err := json.Unmarshal(body, &st); err != nil {
		return health.Status{}, err
	}
	return st, nil
}

func systemdFailed(service string) (bool, string, error) {
	cmd := exec.Command("systemctl", "is-failed", service)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, "", nil
		}
		return false, "", fmt.Errorf("systemctl is-failed %s: %w", service, err)
	}

	detail, err := journalTail(service, 15)
	if err != nil {
		detail = "systemd reports failed (journal unavailable)"
	}
	return true, detail, nil
}

func journalTail(service string, lines int) (string, error) {
	out, err := exec.Command("journalctl", "-u", service, "-n", fmt.Sprintf("%d", lines), "--no-pager", "-o", "short-iso").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func loadState(path string) *watchState {
	data, err := os.ReadFile(path)
	if err != nil {
		return &watchState{}
	}
	var st watchState
	if err := json.Unmarshal(data, &st); err != nil {
		return &watchState{}
	}
	if st.Timers == nil {
		st.Timers = make(map[string]timerState)
	}
	return &st
}

func saveState(path string, state *watchState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
