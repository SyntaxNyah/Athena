// Copyright (C) 2026 SyntaxNyah
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Tests that /area rename never forks the per-area daily log file into a
// second directory keyed off the new name -- see the WriteAreaLog call site
// in server.go and the matching comment in commands_area_rename.go.
package athena

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MangosArentLiterature/Athena/internal/logger"
)

// enableAreaLoggingForTest points area logging at a fresh temp directory and
// restores the previous LogPath/EnableAreaLogging afterward.
func enableAreaLoggingForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origPath := logger.LogPath
	origEnabled := logger.EnableAreaLogging
	logger.LogPath = dir
	logger.EnableAreaLogging = true
	t.Cleanup(func() {
		logger.LogPath = origPath
		logger.EnableAreaLogging = origEnabled
	})
	return dir
}

// TestAreaRenameKeepsWritingTheSameLogFile is the point of the fix: renaming
// an area mid-session must not start a new "<NewName>/<NewName>-<date>.txt"
// log file next to the original one. Every line -- before the rename,
// the rename event itself, and after the rename -- must land in the single
// file keyed by the area's configured (DefaultName) identity.
func TestAreaRenameKeepsWritingTheSameLogFile(t *testing.T) {
	courtroom, _ := setupRenameTest(t)
	dir := enableAreaLoggingForTest(t)
	cm, _ := newRenameClient(courtroom, 1)

	// Mirrors what NewServer does once at startup for every area: create the
	// log directory for the area's configured identity. WriteAreaLog opens
	// files with O_CREATE but never creates missing parent directories, and
	// with the rename fix, nothing creates one again later -- a rename must
	// be able to rely entirely on this startup-time directory.
	if err := logger.CreateAreaLogDirectory(courtroom.DefaultName()); err != nil {
		t.Fatalf("CreateAreaLogDirectory failed: %v", err)
	}

	addToBuffer(cm, "OOC", "hello before the rename", false)

	cmdAreaRename(cm, []string{"Miku", "Cafe"}, "usage")
	if courtroom.Name() != "Miku Cafe" {
		t.Fatalf("rename did not take effect, area is still called %q", courtroom.Name())
	}

	addToBuffer(cm, "OOC", "hello after the rename", false)

	// Exactly one area-log directory must exist, named for the area's
	// configured identity ("Courtroom 3"), never the post-rename display
	// name ("Miku Cafe").
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read log dir: %v", err)
	}
	var dirNames []string
	for _, e := range entries {
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
		}
	}
	if len(dirNames) != 1 {
		t.Fatalf("expected exactly 1 area-log directory, got %v", dirNames)
	}
	if dirNames[0] != "Courtroom 3" {
		t.Fatalf("area log directory = %q, want the configured name %q (a rename must never fork the log into a new folder)",
			dirNames[0], "Courtroom 3")
	}

	// Exactly one log file for today inside it, containing lines from both
	// before and after the rename.
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(dir, "Courtroom 3", "Courtroom 3-"+today+".txt")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected a single merged log file at %q: %v", logFile, err)
	}
	content := string(data)
	if !strings.Contains(content, "hello before the rename") {
		t.Error("log file is missing the line written before the rename")
	}
	if !strings.Contains(content, "hello after the rename") {
		t.Error("log file is missing the line written after the rename")
	}
	if !strings.Contains(content, "Renamed the area from") {
		t.Error("log file is missing the rename event itself (the CMD entry cmdAreaRename writes via addToBuffer)")
	}

	// And nothing under a "Miku Cafe" directory at all.
	if _, err := os.Stat(filepath.Join(dir, "Miku Cafe")); err == nil {
		t.Error("a second log directory named after the post-rename display name must never be created")
	}
}
