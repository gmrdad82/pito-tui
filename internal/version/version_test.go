package version

import "testing"

func TestStringDefaultsToDev(t *testing.T) {
	if String() != Version {
		t.Errorf("String() = %q, want %q", String(), Version)
	}
	if Version == "" {
		t.Error("Version must never be empty")
	}
}
