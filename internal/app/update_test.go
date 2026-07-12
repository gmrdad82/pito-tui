package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// buildArchive tars a fake pito-tui binary the way goreleaser does.
func buildArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "pito-tui", Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestUpdateRefusesDevBuilds(t *testing.T) {
	// version.String() is "dev" under test builds (no ldflags) — the
	// refusal IS the contract for source builds.
	err := Update(&bytes.Buffer{}, "http://127.0.0.1:1", "")
	if err == nil || !strings.Contains(err.Error(), "source build") {
		t.Fatalf("dev build must refuse to self-update, got %v", err)
	}
}

func TestUpdateHelpersVerifyAndExtract(t *testing.T) {
	binary := []byte("#!/fake-binary v2.0.1")
	archive := buildArchive(t, binary)
	name := fmt.Sprintf("pito-tui_2.0.1_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"

	if err := verifyChecksum(archive, name, sums); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := verifyChecksum(append(archive, 'x'), name, sums); err == nil {
		t.Fatal("corrupted archive must fail the checksum")
	}
	got, err := extractBinary(archive)
	if err != nil || !bytes.Equal(got, binary) {
		t.Fatalf("extract = %q, %v", got, err)
	}
}

func TestLatestReleaseParsesTheAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/gmrdad82/pito-tui/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v2.0.1","assets":[{"name":"checksums.txt","browser_download_url":"http://x/c"}]}`)
	}))
	defer srv.Close()
	rel, err := latestRelease(srv.Client(), srv.URL)
	if err != nil || rel.TagName != "v2.0.1" || len(rel.Assets) != 1 {
		t.Fatalf("latestRelease = %+v, %v", rel, err)
	}
}
