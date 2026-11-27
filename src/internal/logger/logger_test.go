package logger

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLogger() (*Logger, *bytes.Buffer) {
	var out bytes.Buffer
	var l = New(&out)
	l.color = false
	var now = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	l.now = func() time.Time {
		now = now.Add(time.Millisecond)
		return now
	}
	return l, &out
}

func TestFormat(t *testing.T) {
	tests := map[string]string{
		"Streaming Status:Tuner 1 / 1":    "Streaming Status:       Tuner 1 / 1",
		"URL:http://a:1/b":                "URL:                    http://a:1/b",
		"A label longer than the width:x": "A label longer than the width:x",
	}
	for in, want := range tests {
		if got, ok := format(in); !ok || got != want {
			t.Errorf("format(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := format("no label"); ok {
		t.Error(`format("no label") reported a label`)
	}
}

func TestEntries(t *testing.T) {
	var l, out = newTestLogger()
	l.SetName("App")
	l.SetDebugLevel(1)

	l.Info("Status:ok")
	l.Info("without label") // not logged
	l.Debug(1, "Debug:shown")
	l.Debug(2, "Debug:hidden")
	l.Warning("careful")
	l.Error("broken")

	var got = l.Log()
	var pad = func(label string) string { return strings.Repeat("&nbsp;", labelWidth-len(label)) }
	var want = []string{
		"2026-01-02 03:04:05 [App]&nbsp;Status:" + pad("Status") + "ok",
		"2026-01-02 03:04:05 [DEBUG]&nbsp;Debug:" + pad("Debug") + "shown",
		"2026-01-02 03:04:05 [App] [WARNING] careful",
		"2026-01-02 03:04:05 [App] [ERROR] broken",
	}
	if strings.Join(got.Log, "\n") != strings.Join(want, "\n") {
		t.Errorf("Log =\n%s\nwant\n%s", strings.Join(got.Log, "\n"), strings.Join(want, "\n"))
	}
	if got.Errors != 1 || got.Warnings != 1 {
		t.Errorf("Errors = %d, Warnings = %d, want 1, 1", got.Errors, got.Warnings)
	}
	if lines := strings.Count(out.String(), "\n"); lines != 4 {
		t.Errorf("console lines = %d, want 4:\n%s", lines, out)
	}

	l.Clear()
	if got := l.Log(); len(got.Log) != 0 || got.Errors != 0 || got.Warnings != 0 {
		t.Errorf("after Clear: %+v", got)
	}
}

func TestQuiet(t *testing.T) {
	var l, out = newTestLogger()
	l.SetQuiet(true)
	l.Info("Status:ok")
	l.Error("still logged")
	if len(l.Log().Log) != 1 || strings.Contains(out.String(), "Status") {
		t.Errorf("quiet logger logged info: %v", l.Log().Log)
	}
}

// The newest entries are kept, and the counts only cover kept entries.
func TestLimit(t *testing.T) {
	var l, _ = newTestLogger()
	l.SetLimit(3)

	l.Error("e1")
	l.Warning("w1")
	for i := range 6 {
		l.Info(fmt.Sprintf("Entry:%d", i))
	}

	var got = l.Log()
	if len(got.Log) != 3 || !strings.HasSuffix(got.Log[2], ";5") || !strings.HasSuffix(got.Log[0], ";3") {
		t.Errorf("Log = %v, want entries 3, 4 and 5", got.Log)
	}
	if got.Errors != 0 || got.Warnings != 0 {
		t.Errorf("Errors = %d, Warnings = %d, want 0 for dropped entries", got.Errors, got.Warnings)
	}
}

// Only the newest notifications are kept.
func TestNotifications(t *testing.T) {
	var l, _ = newTestLogger()
	for i := range notificationLimit + 5 {
		l.Notify(Notification{Type: "info", Message: fmt.Sprint(i)})
	}

	var got = l.Notifications()
	if len(got) != notificationLimit {
		t.Fatalf("notifications = %d, want %d", len(got), notificationLimit)
	}
	var messages = make(map[string]bool)
	for _, n := range got {
		messages[n.Message] = true
		if n.Headline != "INFO" || !n.New {
			t.Errorf("notification = %+v", n)
		}
	}
	for i := 5; i < notificationLimit+5; i++ {
		if !messages[fmt.Sprint(i)] {
			t.Errorf("notification %d is missing, kept: %v", i, messages)
		}
	}
}

func TestHighlight(t *testing.T) {
	var l, out = newTestLogger()
	l.Highlight("Restore:Backup was restored")
	l.Highlight("no label")

	if !strings.Contains(out.String(), "Restore:") || len(l.Log().Log) != 0 {
		t.Errorf("console = %q, log = %v", out, l.Log().Log)
	}
	var messages []string
	for _, n := range l.Notifications() {
		messages = append(messages, n.Message)
	}
	if len(messages) != 2 || !strings.Contains(strings.Join(messages, "|"), "Backup was restored") {
		t.Errorf("notifications = %q", messages)
	}
}

// Run with -race.
func TestConcurrentUse(t *testing.T) {
	var l, _ = newTestLogger()
	var now = time.Now
	l.now = now
	l.SetLimit(50)
	l.SetDebugLevel(3)

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				l.Info(fmt.Sprintf("Goroutine %d:%d", g, i))
				l.Debug(1, "Debug:x")
				l.Error("e")
				l.Notify(Notification{Type: "info"})
				_ = l.Log()
				_ = l.Notifications()
				if i%50 == 0 {
					l.Clear()
					l.SetLimit(40 + g)
				}
			}
		})
	}
	wg.Wait()

	if n := len(l.Log().Log); n > 47 {
		t.Errorf("kept %d entries, more than the limit", n)
	}
}
