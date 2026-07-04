package cable

import "testing"

func TestIdentifierExactShape(t *testing.T) {
	got := Identifier("3f1c")
	want := `{"channel":"TuiChannel","uuid":"3f1c"}`
	if got != want {
		t.Errorf("Identifier = %s, want %s", got, want)
	}
}

func TestConnStateString(t *testing.T) {
	cases := map[ConnState]string{
		StateConnecting:   "connecting",
		StateConnected:    "connected",
		StateDisconnected: "disconnected",
		ConnState(99):     "disconnected",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", state, got, want)
		}
	}
}
