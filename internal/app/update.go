// The --update flag (owner 2026-07-12: binary install on Arch, no brew,
// AUR registrations still closed — the binary must update itself).
//
// Flow: GET the latest release from the GitHub API → compare its tag to
// the running build's version → download the matching
// pito-tui_<ver>_<os>_<arch>.tar.gz asset → verify its sha256 against
// the release's checksums.txt → extract the binary → atomically replace
// os.Executable() (write a sibling .new file, rename over). A dev build
// (version "dev") refuses politely: it has no release identity to
// compare, and clobbering a source build with a release binary is never
// what the developer meant.
package app

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gmrdad82/pito-tui/internal/version"
)

const updateRepo = "gmrdad82/pito-tui"

// releaseInfo is the slice of the GitHub release payload Update reads.
type releaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Update runs the whole self-update against baseURL (the real GitHub API
// in production; an httptest server in tests). out receives progress
// lines — plain prints, this runs before the TUI ever starts.
func Update(out io.Writer, apiBase, downloadOverride string) error {
	current := version.String()
	if current == "dev" {
		return fmt.Errorf("this is a source build (version dev) — update by pulling the repo, or install a release binary first")
	}

	client := &http.Client{Timeout: 60 * time.Second}
	rel, err := latestRelease(client, apiBase)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if latest == strings.TrimPrefix(current, "v") {
		fmt.Fprintf(out, "already up to date (%s)\n", current)
		return nil
	}
	fmt.Fprintf(out, "updating %s → %s\n", current, latest)

	assetName := fmt.Sprintf("pito-tui_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	assetURL, sumsURL := "", ""
	for _, a := range rel.Assets {
		switch a.Name {
		case assetName:
			assetURL = a.BrowserDownloadURL
		case "checksums.txt":
			sumsURL = a.BrowserDownloadURL
		}
	}
	if assetURL == "" {
		return fmt.Errorf("release %s has no asset %s", rel.TagName, assetName)
	}
	if downloadOverride != "" { // tests point downloads at the fixture server
		assetURL = downloadOverride + "/" + assetName
		sumsURL = downloadOverride + "/checksums.txt"
	}

	archive, err := fetch(client, assetURL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", assetName, err)
	}
	if sumsURL != "" {
		sums, err := fetch(client, sumsURL)
		if err != nil {
			return fmt.Errorf("downloading checksums.txt: %w", err)
		}
		if err := verifyChecksum(archive, assetName, string(sums)); err != nil {
			return err
		}
		fmt.Fprintln(out, "checksum verified")
	}

	binary, err := extractBinary(archive)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	staged := self + ".new"
	if err := os.WriteFile(staged, binary, 0o755); err != nil {
		return fmt.Errorf("writing %s (permissions? try from a writable install location): %w", staged, err)
	}
	if err := os.Rename(staged, self); err != nil {
		os.Remove(staged)
		return fmt.Errorf("replacing %s (permissions? rerun with write access to the install dir): %w", self, err)
	}
	fmt.Fprintf(out, "updated %s → %s (%s)\n", current, latest, self)
	return nil
}

func latestRelease(client *http.Client, apiBase string) (*releaseInfo, error) {
	url := strings.TrimRight(apiBase, "/") + "/repos/" + updateRepo + "/releases/latest"
	body, err := fetch(client, url)
	if err != nil {
		return nil, fmt.Errorf("querying latest release: %w", err)
	}
	var rel releaseInfo
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release payload carried no tag")
	}
	return &rel, nil
}

func fetch(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

// verifyChecksum finds assetName's line in goreleaser's checksums.txt
// ("<sha256>  <name>") and compares.
func verifyChecksum(archive []byte, assetName, sums string) error {
	want := ""
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s", assetName)
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", assetName, got, want)
	}
	return nil
}

// extractBinary pulls the pito-tui file out of the release tar.gz.
func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, fmt.Errorf("opening archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if filepath.Base(hdr.Name) == "pito-tui" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
	return nil, fmt.Errorf("archive carried no pito-tui binary")
}
