package scip

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	ScipTypescriptVersion = "0.4.0"
	ScipPythonVersion     = "0.6.6"

	scipReleasesAPI     = "https://api.github.com/repos/scip-code/scip/releases"
	scipPinnedMinor     = "0.8"
	scipReleaseFallback = "v0.8.1"
	minScipMajor        = 0
	minScipMinor        = 8
	minScipPatch        = 0
)

var versionRe = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

type scipVersion struct {
	major, minor, patch int
}

func (v scipVersion) atLeast(major, minor, patch int) bool {
	if v.major != major {
		return v.major > major
	}
	if v.minor != minor {
		return v.minor > minor
	}
	return v.patch >= patch
}

func parseVersion(output string) (scipVersion, bool) {
	m := versionRe.FindStringSubmatch(output)
	if m == nil {
		return scipVersion{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return scipVersion{maj, min, pat}, true
}

func cachedScipPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "scip-cli", "bin", "scip")
}

func scipVersionAt(path string) (scipVersion, bool) {
	cmd := exec.Command(path, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return scipVersion{}, false
	}
	return parseVersion(string(out))
}

func pathScipBinary() string {
	p, err := exec.LookPath("scip")
	if err != nil {
		return ""
	}
	return p
}

func latestReleaseTag() string {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(scipReleasesAPI)
	if err != nil {
		return scipReleaseFallback
	}
	defer resp.Body.Close()

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return scipReleaseFallback
	}
	prefix := "v" + scipPinnedMinor + "."
	for _, r := range releases {
		if strings.HasPrefix(r.TagName, prefix) {
			return r.TagName
		}
	}
	return scipReleaseFallback
}

func platformArchive() (string, error) {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("scip-darwin-%s.tar.gz", arch), nil
	case "linux":
		return fmt.Sprintf("scip-linux-%s.tar.gz", arch), nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func downloadScipBinary(dest string) error {
	tag := latestReleaseTag()
	archiveName, err := platformArchive()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/scip-code/scip/releases/download/%s/%s", tag, archiveName)

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Downloading scip %s from GitHub releases...\n", tag)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("invalid gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}
		base := filepath.Base(hdr.Name)
		if hdr.Typeflag == tar.TypeReg && (base == "scip" || hdr.Name == "scip") {
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			return nil
		}
	}
	return fmt.Errorf("scip archive did not contain a scip binary")
}

// EnsureScipBinary returns the path to a working scip binary, downloading if needed.
func EnsureScipBinary() (string, error) {
	cached := cachedScipPath()

	if info, err := os.Stat(cached); err == nil && !info.IsDir() {
		if v, ok := scipVersionAt(cached); ok && v.atLeast(minScipMajor, minScipMinor, minScipPatch) {
			return cached, nil
		}
	}

	if p := pathScipBinary(); p != "" {
		if v, ok := scipVersionAt(p); ok && v.atLeast(minScipMajor, minScipMinor, minScipPatch) {
			return p, nil
		}
		if _, ok := scipVersionAt(p); !ok {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				fmt.Fprintln(os.Stderr, "Warning: 'scip' on PATH is not the SCIP indexer (brew install scip installs an unrelated solver). Downloading the correct binary...")
			}
		}
	}

	os.Remove(cached)

	if err := downloadScipBinary(cached); err != nil {
		return "", fmt.Errorf("scip CLI not found and auto-download failed. Install manually from https://github.com/scip-code/scip/releases. Reason: %w", err)
	}

	if v, ok := scipVersionAt(cached); !ok || !v.atLeast(minScipMajor, minScipMinor, minScipPatch) {
		return "", fmt.Errorf("downloaded scip binary failed version check")
	}

	return cached, nil
}
