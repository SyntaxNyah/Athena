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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeAreaName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Normal Area", "Normal Area"},
		{"Area/With/Slashes", "Area_With_Slashes"},
		{"Area\\With\\Backslashes", "Area_With_Backslashes"},
		{"Area:With:Colons", "Area_With_Colons"},
		{"Area*With*Stars", "Area_With_Stars"},
		{"Area?With?Questions", "Area_With_Questions"},
		{"Area\"With\"Quotes", "Area_With_Quotes"},
		{"Area<With>Brackets", "Area_With_Brackets"},
		{"Area|With|Pipes", "Area_With_Pipes"},
		// Path traversal: neither "." nor ".." contains any character the
		// replacer above strips, so filepath.Join(LogPath, sanitizeAreaName(name))
		// would otherwise resolve outside LogPath entirely.
		{".", "_"},
		{"..", "_"},
	}

	for _, tt := range tests {
		result := sanitizeAreaName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeAreaName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestCreateAreaLogDirectoryRejectsTraversal is the end-to-end version of the
// "." / ".." cases above: confirms a directory is never created outside
// LogPath even if sanitizeAreaName's substitution were ever weakened.
func TestCreateAreaLogDirectoryRejectsTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	oldLogPath, oldEnabled := LogPath, EnableAreaLogging
	LogPath = tmpDir
	EnableAreaLogging = true
	t.Cleanup(func() { LogPath, EnableAreaLogging = oldLogPath, oldEnabled })

	for _, name := range []string{".", ".."} {
		if err := CreateAreaLogDirectory(name); err != nil {
			t.Fatalf("CreateAreaLogDirectory(%q): %v", name, err)
		}
	}

	parent := filepath.Dir(tmpDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read parent dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() == ".." || strings.HasPrefix(e.Name(), "..-") {
			t.Fatalf("CreateAreaLogDirectory escaped LogPath: found %q in %q", e.Name(), parent)
		}
	}
}

func TestCreateAreaLogDirectory(t *testing.T) {
	// Setup temporary test directory
	tempDir := t.TempDir()
	LogPath = tempDir

	// Test when area logging is disabled
	EnableAreaLogging = false
	err := CreateAreaLogDirectory("Test Area")
	if err != nil {
		t.Errorf("CreateAreaLogDirectory should not error when logging is disabled: %v", err)
	}

	// Test when area logging is enabled
	EnableAreaLogging = true
	err = CreateAreaLogDirectory("Test Area")
	if err != nil {
		t.Errorf("CreateAreaLogDirectory failed: %v", err)
	}

	// Check if directory was created
	expectedDir := filepath.Join(tempDir, "Test Area")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Area log directory was not created at %s", expectedDir)
	}

	// Test with special characters
	err = CreateAreaLogDirectory("Area/With/Slashes")
	if err != nil {
		t.Errorf("CreateAreaLogDirectory failed with special characters: %v", err)
	}

	sanitizedDir := filepath.Join(tempDir, "Area_With_Slashes")
	if _, err := os.Stat(sanitizedDir); os.IsNotExist(err) {
		t.Errorf("Sanitized area log directory was not created at %s", sanitizedDir)
	}
}

func TestWriteAreaLog(t *testing.T) {
	// Setup temporary test directory
	tempDir := t.TempDir()
	LogPath = tempDir
	EnableAreaLogging = true

	areaName := "Test Courtroom"
	err := CreateAreaLogDirectory(areaName)
	if err != nil {
		t.Fatalf("Failed to create area log directory: %v", err)
	}

	// Write a log entry
	logEntry := "[12:34:56] | IC | Phoenix Wright | ipid123 | hdid456 | Phoenix | TestUser | \"Objection!\""
	WriteAreaLog(areaName, logEntry)

	// Check if log file was created with today's date
	today := time.Now().Format("2006-01-02")
	expectedFile := filepath.Join(tempDir, areaName, areaName+"-"+today+".txt")

	// Wait a moment for file to be written
	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := strings.TrimSpace(string(content))
	if contentStr != logEntry {
		t.Errorf("Log content mismatch.\nExpected: %q\nGot: %q", logEntry, contentStr)
	}
}

func TestWriteAreaLogMultipleEntries(t *testing.T) {
	// Setup temporary test directory
	tempDir := t.TempDir()
	LogPath = tempDir
	EnableAreaLogging = true

	areaName := "Busy Courtroom"
	err := CreateAreaLogDirectory(areaName)
	if err != nil {
		t.Fatalf("Failed to create area log directory: %v", err)
	}

	// Write multiple log entries
	entries := []string{
		"[12:00:00] | IC | Phoenix Wright | ipid1 | hdid1 | Phoenix | User1 | \"First message\"",
		"[12:00:01] | OOC | Maya Fey | ipid2 | hdid2 | Maya | User2 | \"Second message\"",
		"[12:00:02] | AREA | Miles Edgeworth | ipid3 | hdid3 | Edgeworth | User3 | \"Joined area.\"",
	}

	for _, entry := range entries {
		WriteAreaLog(areaName, entry)
	}

	// Read and verify log file
	today := time.Now().Format("2006-01-02")
	expectedFile := filepath.Join(tempDir, areaName, areaName+"-"+today+".txt")

	time.Sleep(100 * time.Millisecond)

	content, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != len(entries) {
		t.Errorf("Expected %d log entries, got %d", len(entries), len(lines))
	}

	for i, entry := range entries {
		if i < len(lines) && lines[i] != entry {
			t.Errorf("Entry %d mismatch.\nExpected: %q\nGot: %q", i, entry, lines[i])
		}
	}
}

func TestWriteAreaLogDisabled(t *testing.T) {
	// Setup temporary test directory
	tempDir := t.TempDir()
	LogPath = tempDir
	EnableAreaLogging = false

	areaName := "Test Area"

	// Try to write a log entry when logging is disabled
	logEntry := "[12:34:56] | IC | Test | test | test | Test | Test | \"Test\""
	WriteAreaLog(areaName, logEntry)

	// Verify no log file was created
	today := time.Now().Format("2006-01-02")
	expectedFile := filepath.Join(tempDir, areaName, areaName+"-"+today+".txt")

	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(expectedFile); !os.IsNotExist(err) {
		t.Errorf("Log file should not exist when area logging is disabled")
	}
}
