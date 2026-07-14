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

func TestIsRelease(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	cases := []struct {
		version string
		want    bool
	}{
		{"dev", false},
		{"1.0.0", true},
		{"2.6.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			Version = tc.version
			if got := IsRelease(); got != tc.want {
				t.Errorf("IsRelease() with Version=%q = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}
