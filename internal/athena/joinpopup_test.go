/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork additions: tests for the join popup (joinpopup.go), the
   operator-authored welcome message shown once to a genuinely new IPID. */

package athena

import (
	"strings"
	"testing"

	"github.com/MangosArentLiterature/Athena/internal/settings"
)

// withJoinPopupConfig runs fn with a temporary config installed, mirroring
// withCaptchaConfig in joincaptcha_test.go.
func withJoinPopupConfig(t *testing.T, c settings.ServerConfig, fn func()) {
	t.Helper()
	orig := config
	defer func() { config = orig }()
	config = &settings.Config{ServerConfig: c}
	fn()
}

// TestJoinPopupMessageGating covers every reason the feature should report
// "nothing to show": the toggle off, no config at all, and a message that is
// blank or only whitespace -- each of which must behave as if join_popup
// were false rather than showing an empty dialog.
func TestJoinPopupMessageGating(t *testing.T) {
	if msg, ok := joinPopupMessage(); ok || msg != "" {
		t.Errorf("nil config should report no message; got (%q, %v)", msg, ok)
	}

	withJoinPopupConfig(t, settings.ServerConfig{JoinPopup: false, JoinPopupMessage: "Welcome!"}, func() {
		if msg, ok := joinPopupMessage(); ok || msg != "" {
			t.Errorf("join_popup = false should report no message; got (%q, %v)", msg, ok)
		}
	})

	withJoinPopupConfig(t, settings.ServerConfig{JoinPopup: true, JoinPopupMessage: ""}, func() {
		if msg, ok := joinPopupMessage(); ok || msg != "" {
			t.Errorf("a blank message should report no message even when enabled; got (%q, %v)", msg, ok)
		}
	})

	withJoinPopupConfig(t, settings.ServerConfig{JoinPopup: true, JoinPopupMessage: "   \n\t  "}, func() {
		if msg, ok := joinPopupMessage(); ok || msg != "" {
			t.Errorf("a whitespace-only message should report no message; got (%q, %v)", msg, ok)
		}
	})
}

// TestJoinPopupMessageTrimsWhitespace verifies the leading/trailing
// whitespace a triple-quoted TOML string tends to carry (a newline right
// after the opening """, one right before the closing """) doesn't end up in
// the OOC line or the popup box, while interior formatting -- the blank
// lines and links an operator actually wants -- survives untouched.
func TestJoinPopupMessageTrimsWhitespace(t *testing.T) {
	raw := "\nRead the rules: https://example.com/rules\n\nDiscord: https://discord.gg/example\n"
	withJoinPopupConfig(t, settings.ServerConfig{JoinPopup: true, JoinPopupMessage: raw}, func() {
		msg, ok := joinPopupMessage()
		if !ok {
			t.Fatal("a real message should report ok")
		}
		if strings.HasPrefix(msg, "\n") || strings.HasSuffix(msg, "\n") {
			t.Errorf("message should have outer whitespace trimmed; got %q", msg)
		}
		if !strings.Contains(msg, "https://example.com/rules") || !strings.Contains(msg, "https://discord.gg/example") {
			t.Errorf("trimming should not touch the actual content; got %q", msg)
		}
	})
}

// TestIssueJoinPopupOnlyForGenuinelyNewIPID exercises the full send path with
// a fake client, the same captureConn pattern used in punishment_audit_test.go.
// It asserts the popup (both the OOC copy and the BB dialog) reaches only a
// connection that is both configured for and flagged as genuinely new, and
// never anyone else -- disabled, blank message, or a returning IPID.
func TestIssueJoinPopupOnlyForGenuinelyNewIPID(t *testing.T) {
	const wantText = "Welcome! Read the rules: https://example.com/rules"

	cases := []struct {
		name      string
		cfg       settings.ServerConfig
		isNewIPID bool
		wantSent  bool
	}{
		{"new IPID, enabled with a message", settings.ServerConfig{JoinPopup: true, JoinPopupMessage: wantText}, true, true},
		{"returning IPID, enabled with a message", settings.ServerConfig{JoinPopup: true, JoinPopupMessage: wantText}, false, false},
		{"new IPID, feature disabled", settings.ServerConfig{JoinPopup: false, JoinPopupMessage: wantText}, true, false},
		{"new IPID, blank message", settings.ServerConfig{JoinPopup: true, JoinPopupMessage: ""}, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withJoinPopupConfig(t, tc.cfg, func() {
				conn := &captureConn{}
				client := &Client{conn: conn, uid: 1, ipid: "test-ipid", char: -1, isNewIPID: tc.isNewIPID}

				issueJoinPopup(client)

				out := conn.String()
				gotSent := strings.Contains(out, "Welcome") && strings.Contains(out, "example.com")
				if gotSent != tc.wantSent {
					t.Errorf("issueJoinPopup output containing the welcome text = %v, want %v (output: %q)", gotSent, tc.wantSent, out)
				}
				if tc.wantSent {
					// Sent via both the OOC packet (CT) and the popup (BB) --
					// the same "both channels" pattern the join captcha uses,
					// so the message survives even if the modal is dismissed
					// unread.
					if !strings.Contains(out, "CT#") {
						t.Errorf("expected an OOC (CT) copy of the message; got %q", out)
					}
					if !strings.Contains(out, "BB#") {
						t.Errorf("expected a popup (BB) copy of the message; got %q", out)
					}
				}
			})
		})
	}
}
