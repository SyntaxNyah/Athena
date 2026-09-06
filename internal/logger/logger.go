/* Athena - A server for Attorney Online 2 written in Go
Copyright (C) 2022 MangosArentLiterature <mango@transmenace.dev>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>. */

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/webhook"
)

// logBytePool hands out reusable byte slices for the log() framing path so
// each call does not allocate a fresh "<time>: <LEVEL>: <msg>\n" string.
// The slices are Put back after each log line. Starting capacity is sized
// for a typical line; append will grow it automatically for long messages.
var logBytePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 160)
		return &b
	},
}

type LogLevel int

const (
	Info LogLevel = iota
	Warning
	Error
	Fatal
)

// maxRecentLogLines bounds the recentLines ring buffer backing RecentLines.
const maxRecentLogLines = 2000

// levelToString maps a LogLevel to its display name.
// Indexed by the iota value so lookup is O(1) with no hash overhead.
var levelToString = [4]string{
	Info:    "INFO",
	Warning: "WARN",
	Error:   "ERROR",
	Fatal:   "FATAL",
}

var (
	LogPath              string
	LogStdOut            bool
	LogFile              bool
	CurrentLevel         LogLevel = Info
	outputLock           sync.Mutex
	EnableAreaLogging    bool
	EnableNetworkLogging bool
	areaLogLocks         sync.Map // Map of area names to their respective locks

	// TUITap is an optional hook invoked with every formatted log line. The
	// TUI installs its own callback so the dashboard can render a recent-log
	// pane without the logger writing to stdout directly. nil by default, so
	// there is zero overhead when the TUI is not in use. Never holds a lock
	// during the call, so callers must serialize their own state.
	TUITap func(string)

	// recentLines is a bounded in-memory ring of the most recently formatted
	// log lines, independent of LogStdOut/LogFile/TUITap. It backs the
	// in-game /terminal admin command so a moderator without shell access to
	// the host can still check on the server. Capped at maxRecentLogLines by
	// trimming the oldest entries in one slice re-copy rather than shifting
	// one element at a time.
	recentLinesMu sync.Mutex
	recentLines   []string

	// Persistent file handles – kept open between writes to avoid
	// the overhead of os.OpenFile + Close on every log message.
	serverLogMu       sync.Mutex
	serverLogFile     *os.File
	serverLogFilePath string

	auditLogMu       sync.Mutex
	auditLogFile     *os.File
	auditLogFilePath string

	networkLogMu       sync.Mutex
	networkLogFile     *os.File
	networkLogFilePath string

	// areaLogFiles stores the open file handle for each area, keyed by
	// sanitized area name. Access is serialised by the per-area mutex
	// returned by getAreaLock, so no additional lock is needed here.
	areaLogFiles sync.Map // map[string]*areaLogState
)

// areaLogState holds the open file handle and the path it was opened for.
// When either the LogPath or the calendar date changes the file is closed and
// reopened so that daily rotation and test isolation both work correctly.
type areaLogState struct {
	f        *os.File
	filePath string
}

// log writes a message to standard output and/or the log file if the level
// matches the server's set log level.
//
// Framing is built by appending into a pooled []byte to avoid the three
// allocations that the old fmt.Sprintf call produced per log line. The exact
// output format is preserved byte-for-byte: "<time.StampMilli>: LEVEL: s\n".
func log(level LogLevel, s string) {
	if level < CurrentLevel {
		return
	}
	bp := logBytePool.Get().(*[]byte)
	buf := (*bp)[:0]
	buf = time.Now().UTC().AppendFormat(buf, time.StampMilli)
	buf = append(buf, ": "...)
	buf = append(buf, levelToString[level]...)
	buf = append(buf, ": "...)
	buf = append(buf, s...)
	buf = append(buf, '\n')

	if LogStdOut {
		outputLock.Lock()
		os.Stdout.Write(buf)
		outputLock.Unlock()
	}

	// formatted is computed once and shared by WriteLog, TUITap and the
	// /terminal ring buffer; turning the byte slice back into a string here
	// is one unavoidable allocation, but it replaces up to three separate
	// conversions in the old path (one per consumer) and keeps WriteLog's
	// interface stable for area-log callers.
	formatted := string(buf)
	if LogFile {
		WriteLog(formatted)
	}
	if tap := TUITap; tap != nil {
		tap(formatted)
	}
	appendRecentLine(formatted)

	*bp = buf
	logBytePool.Put(bp)
}

// appendRecentLine stores a formatted log line (already newline-terminated)
// into the bounded ring buffer backing RecentLines.
func appendRecentLine(s string) {
	recentLinesMu.Lock()
	recentLines = append(recentLines, strings.TrimRight(s, "\n"))
	if len(recentLines) > maxRecentLogLines {
		// Drop the oldest excess in one re-copy rather than shifting one
		// element at a time on every single call past the cap.
		excess := len(recentLines) - maxRecentLogLines
		recentLines = append([]string(nil), recentLines[excess:]...)
	}
	recentLinesMu.Unlock()
}

// RecentLines returns up to n of the most recently logged lines, oldest
// first, regardless of whether LogStdOut/LogFile are enabled. Backs the
// in-game /terminal admin command. Returns nil if nothing has been logged
// yet or n <= 0.
func RecentLines(n int) []string {
	recentLinesMu.Lock()
	defer recentLinesMu.Unlock()
	if n <= 0 || len(recentLines) == 0 {
		return nil
	}
	if n > len(recentLines) {
		n = len(recentLines)
	}
	out := make([]string, n)
	copy(out, recentLines[len(recentLines)-n:])
	return out
}

// LogInfo prints an info message to stdout. Arguments are handled in the manner of fmt.Print.
func LogInfo(s string) {
	log(Info, s)
}

// LogInfof prints an info message to stdout. Arguments are handled in the manner of fmt.Printf.
func LogInfof(format string, v ...interface{}) {
	if Info < CurrentLevel {
		return
	}
	log(Info, fmt.Sprintf(format, v...))
}

// LogWarning prints a warning message to stdout. Arguments are handled in the manner of fmt.Print.
func LogWarning(s string) {
	log(Warning, s)
}

// LogWarningf prints a warning message to stdout. Arguments are handled in the manner of fmt.Printf.
func LogWarningf(format string, v ...interface{}) {
	if Warning < CurrentLevel {
		return
	}
	log(Warning, fmt.Sprintf(format, v...))
}

// LogError prints an error message to stdout. Arguments are handled in the manner of fmt.Print.
func LogError(s string) {
	log(Error, s)
}

// LogErrorf prints an error message to stdout. Arguments are handled in the manner of fmt.Printf.
func LogErrorf(format string, v ...interface{}) {
	if Error < CurrentLevel {
		return
	}
	log(Error, fmt.Sprintf(format, v...))
}

// LogFatal prints a fatal error message to stdout. Arguments are handled in the manner of fmt.Print.
func LogFatal(s string) {
	log(Fatal, s)
}

// LogFatalf prints a fatal error message to stdout. Arguments are handled in the manner of fmt.Printf.
func LogFatalf(format string, v ...interface{}) {
	log(Fatal, fmt.Sprintf(format, v...))
}

// WriteReport flushes a given area buffer to a report file.
func WriteReport(name string, buffer []string) {
	auditLogMu.Lock()
	defer auditLogMu.Unlock()
	fname := fmt.Sprintf("report-%v-%v.log", time.Now().UTC().Format("2006-01-02T150405Z"), name)
	fcontents := []byte(strings.Join(buffer, "\n"))
	err := webhook.PostReport(fname, string(fcontents))
	if err != nil {
		LogError(err.Error())
		return
	}
	err = os.WriteFile(LogPath+"/"+fname, fcontents, 0755)
	if err != nil {
		LogError(err.Error())
		return
	}
}

// WriteNetworkLog writes a packet log entry (HDID, IPID, direction, content) to the network log file.
// Each entry includes a timestamp so that packet sequences can be reconstructed for incident review.
// The file handle is kept open between calls to avoid per-write open/close syscall overhead.
func WriteNetworkLog(ipid, hdid, direction, content string) {
	if !EnableNetworkLogging {
		return
	}
	networkLogMu.Lock()
	defer networkLogMu.Unlock()

	target := LogPath + "/network.log"
	if networkLogFile == nil || networkLogFilePath != target {
		if networkLogFile != nil {
			networkLogFile.Close()
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			LogError(err.Error())
			return
		}
		networkLogFile = f
		networkLogFilePath = target
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := fmt.Fprintf(networkLogFile, "[%v] %v | IPID:%v | HDID:%v | %v\n", timestamp, direction, ipid, hdid, content); err != nil {
		LogError(err.Error())
		networkLogFile.Close()
		networkLogFile = nil
	}
}

// WriteAudit writes a line to the server's audit log.
// The file handle is kept open between calls to avoid per-write open/close syscall overhead.
func WriteAudit(s string) {
	auditLogMu.Lock()
	defer auditLogMu.Unlock()

	target := LogPath + "/audit.log"
	if auditLogFile == nil || auditLogFilePath != target {
		if auditLogFile != nil {
			auditLogFile.Close()
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0755)
		if err != nil {
			LogError(err.Error())
			return
		}
		auditLogFile = f
		auditLogFilePath = target
	}

	if _, err := fmt.Fprintf(auditLogFile, "[%v] %v\n", time.Now().UTC().Format("2006/01/02"), s); err != nil {
		LogError(err.Error())
		auditLogFile.Close()
		auditLogFile = nil
	}
}

// WriteLog writes a line to the server's log file.
// The file handle is kept open between calls to avoid per-write open/close syscall overhead.
func WriteLog(s string) {
	serverLogMu.Lock()
	defer serverLogMu.Unlock()

	target := LogPath + "/server.log"
	if serverLogFile == nil || serverLogFilePath != target {
		if serverLogFile != nil {
			serverLogFile.Close()
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0755)
		if err != nil {
			LogFile = false // prevents infinite recursion if log file cannot be opened
			LogError(err.Error())
			return
		}
		serverLogFile = f
		serverLogFilePath = target
	}

	if _, err := serverLogFile.WriteString(s); err != nil {
		LogFile = false
		serverLogFile.Close()
		serverLogFile = nil
		LogError(err.Error())
	}
}

// CloseLogFiles flushes and closes all persistently-open log file handles.
// Call this during a clean server shutdown to ensure all pending writes are committed.
func CloseLogFiles() {
	serverLogMu.Lock()
	if serverLogFile != nil {
		serverLogFile.Close()
		serverLogFile = nil
		serverLogFilePath = ""
	}
	serverLogMu.Unlock()

	auditLogMu.Lock()
	if auditLogFile != nil {
		auditLogFile.Close()
		auditLogFile = nil
		auditLogFilePath = ""
	}
	auditLogMu.Unlock()

	networkLogMu.Lock()
	if networkLogFile != nil {
		networkLogFile.Close()
		networkLogFile = nil
		networkLogFilePath = ""
	}
	networkLogMu.Unlock()

	// Close all open area log file handles.
	areaLogFiles.Range(func(key, value any) bool {
		if state, ok := value.(*areaLogState); ok && state.f != nil {
			state.f.Close()
		}
		areaLogFiles.Delete(key)
		return true
	})
}

// areaNameReplacer is a pre-compiled replacer for converting area names to
// filesystem-safe folder names. Defined at package level so it is built once
// and reused across every sanitizeAreaName call rather than being re-created
// on each invocation (which would happen on every WriteAreaLog call).
var areaNameReplacer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

// sanitizeAreaName converts an area name to a safe folder name.
//
// Defense in depth against path traversal: areaNameReplacer strips "/" and
// "\" from a name, but a name that is exactly "." or ".." contains neither,
// and filepath.Join(LogPath, "..") escapes LogPath entirely -- the primary
// guard against this lives in athena's areaNameRejection (the /area rename
// validator), but this function is also reachable from areas.toml-configured
// names that never go through that validator, so it must hold the same
// invariant on its own.
func sanitizeAreaName(name string) string {
	safe := areaNameReplacer.Replace(name)
	if safe == "." || safe == ".." {
		return "_"
	}
	return safe
}

// CreateAreaLogDirectory creates a log directory for an area if it doesn't exist
func CreateAreaLogDirectory(areaName string) error {
	if !EnableAreaLogging {
		return nil
	}
	safeAreaName := sanitizeAreaName(areaName)
	dirPath := filepath.Join(LogPath, safeAreaName)
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create area log directory: %w", err)
	}
	return nil
}

// getAreaLock returns or creates a mutex for the specified area
func getAreaLock(areaName string) *sync.Mutex {
	lock, _ := areaLogLocks.LoadOrStore(areaName, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// WriteAreaLog writes a log entry to an area's daily log file.
// The file handle is kept open between calls to avoid per-write open/close syscall
// overhead. The handle is automatically closed and reopened when the calendar date
// or LogPath changes (daily rotation and test isolation).
func WriteAreaLog(areaName, logEntry string) {
	if !EnableAreaLogging {
		return
	}

	safeAreaName := sanitizeAreaName(areaName)
	lock := getAreaLock(safeAreaName)
	lock.Lock()
	defer lock.Unlock()

	// Generate daily log file name.
	today := time.Now().Format("2006-01-02")
	filename := filepath.Join(LogPath, safeAreaName, fmt.Sprintf("%s-%s.txt", safeAreaName, today))

	// Load the cached file state for this area (if any).
	var state *areaLogState
	if v, ok := areaLogFiles.Load(safeAreaName); ok {
		state = v.(*areaLogState)
	}

	// Reopen the file if the path has changed (new day or LogPath changed).
	if state == nil || state.filePath != filename {
		if state != nil && state.f != nil {
			state.f.Close()
		}
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			LogErrorf("Failed to open area log file %s: %v", filename, err)
			return
		}
		state = &areaLogState{f: f, filePath: filename}
		areaLogFiles.Store(safeAreaName, state)
	}

	// Write the log entry.
	if _, err := state.f.WriteString(logEntry + "\n"); err != nil {
		LogErrorf("Failed to write to area log file %s: %v", filename, err)
		state.f.Close()
		state.f = nil
		areaLogFiles.Delete(safeAreaName)
	}
}
