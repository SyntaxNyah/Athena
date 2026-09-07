/* Athena - A server for Attorney Online 2 written in Go
   Nyathena fork addition: tests for /toggle global. */

package athena

import (
	"strings"
	"testing"

	"github.com/MangosArentLiterature/Athena/internal/area"
	"github.com/MangosArentLiterature/Athena/internal/packet"
)

// TestCmdToggleGlobalRoundTrips exercises the /toggle global handler
// end-to-end: it flips OOCHidden on, reports the new state, and flips it
// back off on a second call.
func TestCmdToggleGlobalRoundTrips(t *testing.T) {
	conn := &captureConn{}
	client := &Client{conn: conn, uid: 1, ipid: "ip-toggle"}

	if client.OOCHidden() {
		t.Fatal("expected OOC visible before any /toggle global call")
	}

	cmdToggle(client, []string{"global"}, "usage")
	if !client.OOCHidden() {
		t.Fatal("/toggle global did not hide OOC chat")
	}
	if !strings.Contains(conn.String(), "hidden") {
		t.Fatalf("expected confirmation to mention hidden, got %q", conn.String())
	}

	cmdToggle(client, []string{"global"}, "usage")
	if client.OOCHidden() {
		t.Fatal("second /toggle global did not restore OOC visibility")
	}
	if !strings.Contains(conn.String(), "visible again") {
		t.Fatalf("expected confirmation to mention visible again, got %q", conn.String())
	}
}

// TestCmdToggleUnknownArg makes sure a bad subcommand reports the usage
// string rather than silently toggling anything.
func TestCmdToggleUnknownArg(t *testing.T) {
	conn := &captureConn{}
	client := &Client{conn: conn, uid: 1, ipid: "ip-toggle-bad"}

	cmdToggle(client, []string{"bogus"}, "Usage: /toggle global")
	if client.OOCHidden() {
		t.Fatal("an unrecognized /toggle argument must not change state")
	}
	if !strings.Contains(conn.String(), "Invalid argument") {
		t.Fatalf("expected an invalid-argument message, got %q", conn.String())
	}
}

// TestBroadcastOOCToAreaSkipsToggledClients pins the area-OOC delivery
// contract: a client that ran /toggle global never receives an ordinary
// area OOC message, while everyone else in the area still does. It also
// confirms the toggle never affects what the OTHER client in the area sees.
func TestBroadcastOOCToAreaSkipsToggledClients(t *testing.T) {
	newTestClients(t)
	testArea := makeTestArea("ToggleTestArea")
	t.Cleanup(setupTestAreas([]*area.Area{testArea}))

	hiddenConn := &captureConn{}
	hiddenClient := &Client{conn: hiddenConn, uid: 1, ipid: "ip-hidden", area: testArea}
	hiddenClient.SetOOCHidden(true)

	visibleConn := &captureConn{}
	visibleClient := &Client{conn: visibleConn, uid: 2, ipid: "ip-visible", area: testArea}

	for _, c := range []*Client{hiddenClient, visibleClient} {
		clients.AddClient(c)
		clients.RegisterUID(c)
	}

	broadcastOOCToArea("ip-sender", false, testArea,
		&packet.CTToClient{Name: encode("Sender"), Message: encode("hello area"), IsFromServer: "0"})

	if strings.Contains(hiddenConn.String(), "hello area") {
		t.Errorf("client with /toggle global on should not receive area OOC; got %q", hiddenConn.String())
	}
	if !strings.Contains(visibleConn.String(), "hello area") {
		t.Errorf("client without the toggle should still receive area OOC; got %q", visibleConn.String())
	}
}

// TestBroadcastOOCToAllSkipsToggledClients mirrors the above for /global:
// a toggled-off client never receives it, an ordinary client still does.
func TestBroadcastOOCToAllSkipsToggledClients(t *testing.T) {
	newTestClients(t)

	hiddenConn := &captureConn{}
	hiddenClient := &Client{conn: hiddenConn, uid: 1, ipid: "ip-hidden-g"}
	hiddenClient.SetOOCHidden(true)

	visibleConn := &captureConn{}
	visibleClient := &Client{conn: visibleConn, uid: 2, ipid: "ip-visible-g"}

	for _, c := range []*Client{hiddenClient, visibleClient} {
		clients.AddClient(c)
		clients.RegisterUID(c)
	}

	broadcastOOCToAll(&packet.CTToClient{Name: encode("[GLOBAL] Sender"), Message: encode("hello global"), IsFromServer: "1"})

	if strings.Contains(hiddenConn.String(), "hello global") {
		t.Errorf("client with /toggle global on should not receive /global; got %q", hiddenConn.String())
	}
	if !strings.Contains(visibleConn.String(), "hello global") {
		t.Errorf("client without the toggle should still receive /global; got %q", visibleConn.String())
	}
}
