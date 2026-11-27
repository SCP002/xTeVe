// Package logger writes the log to the console and keeps the latest entries and notifications in memory for the web
// UI. It is safe for use by multiple goroutines.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The width of the label before the message, e.g. "Streaming Status:      " (see format).
const labelWidth = 23

// The number of notifications kept for the web UI.
const notificationLimit = 10

// Kind is the kind of a log entry.
type Kind int

// Kinds of log entries.
const (
	KindInfo Kind = iota
	KindDebug
	KindHighlight
	KindWarning
	KindError
)

// Console colors of the kinds.
var colors = map[Kind]string{
	KindInfo:      "\033[0m",
	KindDebug:     "\033[35m",
	KindHighlight: "\033[32m",
	KindWarning:   "\033[33m",
	KindError:     "\033[31m",
}

// Notification is a message shown in the notification area of the web UI.
type Notification struct {
	Headline string `json:"headline"`
	Message  string `json:"message"`
	New      bool   `json:"new"`
	Time     string `json:"time"`
	Type     string `json:"type"`
}

// Log is a copy of the entries kept for the web UI.
type Log struct {
	Errors   int      `json:"errors"`
	Log      []string `json:"log"`
	Warnings int      `json:"warnings"`
}

type entry struct {
	text string
	kind Kind
}

// Logger writes log entries to the console and keeps the latest of them in memory.
type Logger struct {
	name       atomic.Pointer[string]
	debugLevel atomic.Int32
	quiet      atomic.Bool

	console *log.Logger
	color   bool
	now     func() time.Time

	mu            sync.Mutex
	limit         int
	entries       []entry
	notifications []Notification
}

// New returns a logger that writes to out. Colors are used except on Windows.
func New(out io.Writer) *Logger {
	var l = &Logger{
		console: log.New(out, "", log.LstdFlags),
		color:   runtime.GOOS != "windows",
		now:     time.Now,
		limit:   500,
	}
	l.SetName("xTeVe")
	return l
}

var std = New(os.Stderr)

// Default returns the logger used by the package level functions.
func Default() *Logger { return std }

// SetName sets the application name shown in front of info, highlight, warning and error entries.
func (l *Logger) SetName(name string) { l.name.Store(&name) }

// SetDebugLevel sets the highest level of debug entries that are logged, 0 logs none.
func (l *Logger) SetDebugLevel(level int) { l.debugLevel.Store(int32(level)) }

// DebugLevel returns the level set with SetDebugLevel.
func (l *Logger) DebugLevel() int { return int(l.debugLevel.Load()) }

// SetQuiet turns info entries off (used while printing the system information).
func (l *Logger) SetQuiet(quiet bool) { l.quiet.Store(quiet) }

// SetLimit sets the number of entries kept in memory.
func (l *Logger) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = max(limit, 1)
	l.trim()
}

// Info logs "Label:message", the message is aligned after the label.
func (l *Logger) Info(str string) {
	if l.quiet.Load() {
		return
	}
	if msg, ok := format(str); ok {
		l.add(KindInfo, fmt.Sprintf("[%s] %s", *l.name.Load(), msg), true)
	}
}

// Debug logs "Label:message" if level is not higher than the debug level.
func (l *Logger) Debug(level int, str string) {
	if l.DebugLevel() < level {
		return
	}
	if msg, ok := format(str); ok {
		l.add(KindDebug, "[DEBUG] "+msg, true)
	}
}

// Highlight shows "Label:message" on the console and the message as a notification in the web UI.
func (l *Logger) Highlight(str string) {
	var msg, ok = format(str)
	if ok {
		l.print(KindHighlight, fmt.Sprintf("[%s] %s", *l.name.Load(), msg))
	}

	var message string
	if _, after, found := strings.Cut(str, ":"); found {
		message = after
	}
	l.Notify(Notification{Type: "info", Message: message})
}

// Warning logs a warning.
func (l *Logger) Warning(msg string) {
	l.add(KindWarning, fmt.Sprintf("[%s] [WARNING] %s", *l.name.Load(), msg), false)
}

// Error logs an error.
func (l *Logger) Error(msg string) {
	l.add(KindError, fmt.Sprintf("[%s] [ERROR] %s", *l.name.Load(), msg), false)
}

// Notify adds a notification for the web UI. Only the latest notifications are kept.
func (l *Logger) Notify(n Notification) {
	n.Time = strconv.FormatInt(l.now().UnixMilli(), 10)
	n.New = true
	if len(n.Headline) == 0 {
		n.Headline = strings.ToUpper(n.Type)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// The web UI gets the notifications by their time, so a newer one with the same time replaces the older one
	l.notifications = slices.DeleteFunc(l.notifications, func(old Notification) bool { return old.Time == n.Time })
	l.notifications = append(l.notifications, n)
	if len(l.notifications) > notificationLimit {
		l.notifications = slices.Delete(l.notifications, 0, len(l.notifications)-notificationLimit)
	}
}

// Notifications returns the kept notifications by their time.
func (l *Logger) Notifications() map[string]Notification {
	l.mu.Lock()
	defer l.mu.Unlock()

	var notifications = make(map[string]Notification, len(l.notifications))
	for _, n := range l.notifications {
		notifications[n.Time] = n
	}
	return notifications
}

// Log returns a copy of the kept entries with the number of errors and warnings among them.
func (l *Logger) Log() Log {
	l.mu.Lock()
	defer l.mu.Unlock()

	var result = Log{Log: make([]string, 0, len(l.entries))}
	for _, e := range l.entries {
		result.Log = append(result.Log, e.text)
		switch e.kind {
		case KindError:
			result.Errors++
		case KindWarning:
			result.Warnings++
		}
	}
	return result
}

// Clear removes the kept entries.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// add prints the entry and keeps it. Info and debug entries are shown with non-breaking spaces in the web UI, so
// that the aligned labels stay aligned.
func (l *Logger) add(kind Kind, msg string, nbsp bool) {
	l.print(kind, msg)

	if nbsp {
		msg = strings.ReplaceAll(msg, " ", "&nbsp;")
	}
	var text = l.now().Format("2006-01-02 15:04:05") + " " + msg

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry{text: text, kind: kind})
	l.trim()
}

// trim removes the oldest entries beyond the limit. l.mu must be held.
func (l *Logger) trim() {
	if len(l.entries) > l.limit {
		l.entries = slices.Delete(l.entries, 0, len(l.entries)-l.limit)
	}
}

func (l *Logger) print(kind Kind, msg string) {
	if !l.color {
		l.console.Println(msg)
		return
	}
	// One call, so that lines of different goroutines do not mix their colors
	l.console.Print(colors[kind] + msg + "\033[0m")
}

// format aligns "Label:message" to "Label:" padded to labelWidth, followed by the message. It reports false if str
// has no label.
func format(str string) (string, bool) {
	var label, msg, found = strings.Cut(str, ":")
	if !found {
		return "", false
	}
	return label + ":" + strings.Repeat(" ", max(labelWidth-len(label), 0)) + msg, true
}

// Info logs to the default logger, see Logger.Info.
func Info(str string) { std.Info(str) }

// Debug logs to the default logger, see Logger.Debug.
func Debug(level int, str string) { std.Debug(level, str) }

// Highlight logs to the default logger, see Logger.Highlight.
func Highlight(str string) { std.Highlight(str) }

// Warning logs to the default logger, see Logger.Warning.
func Warning(msg string) { std.Warning(msg) }

// Error logs to the default logger, see Logger.Error.
func Error(msg string) { std.Error(msg) }
