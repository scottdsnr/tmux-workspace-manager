package main

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
)

const updateRepo = "scottdsnr/tmux-workspace-manager"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// latestReleaseTag returns the tag name of the latest GitHub release, e.g. "v0.1.4".
func latestReleaseTag() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checking for updates: unexpected status %s", resp.Status)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("checking for updates: latest release has no tag")
	}
	return rel.TagName, nil
}

func downloadToFile(url, dest string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s for %s", resp.Status, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// runSelfUpdate checks GitHub for a newer release than the running binary
// and, unless dry is set, downloads and swaps it in.
func runSelfUpdate(dry, assumeYes bool) {
	if (runtime.GOOS != "linux" && runtime.GOOS != "darwin") || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		fatal("%sNo prebuilt binary for %s/%s; only linux/amd64, linux/arm64, darwin/amd64, and darwin/arm64 are published.", sym("error"), runtime.GOOS, runtime.GOARCH)
	}

	fmt.Printf("%sChecking for updates...\n", sym("info"))
	tag, err := latestReleaseTag()
	if err != nil {
		fatal("%s%v", sym("error"), err)
	}

	if version != "dev" && normalizeTag(version) == normalizeTag(tag) {
		fmt.Printf("%sAlready up to date (%s).\n", sym("check"), version)
		return
	}
	if version == "dev" {
		fmt.Printf("%sRunning a dev build; latest release is %s.\n", sym("info"), tag)
	} else {
		fmt.Printf("%sUpdate available: %s -> %s\n", sym("sparkle"), version, tag)
	}

	if dry {
		fmt.Printf("%s[dry-run] would download and install %s\n", sym("dry"), tag)
		return
	}

	if !assumeYes && interactive() {
		answer := prompt(fmt.Sprintf("%sInstall %s now? (y/N): ", sym("warn"), tag))
		if strings.ToLower(answer) != "y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	exePath, err := os.Executable()
	if err != nil {
		fatal("%sCould not determine the running binary's path: %v", sym("error"), err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	asset := fmt.Sprintf("tmux-workspace-manager_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", updateRepo, tag)

	workDir, err := os.MkdirTemp("", "tmux-workspace-update-*")
	if err != nil {
		fatal("%sCould not create a temp directory: %v", sym("error"), err)
	}
	defer os.RemoveAll(workDir)

	archivePath := filepath.Join(workDir, asset)
	fmt.Printf("%sDownloading %s (%s)...\n", sym("package"), asset, tag)
	if err := downloadToFile(baseURL+"/"+asset, archivePath); err != nil {
		fatal("%sDownload failed: %v", sym("error"), err)
	}

	fmt.Printf("%sVerifying checksum...\n", sym("info"))
	checksumsPath := filepath.Join(workDir, "checksums.txt")
	if err := downloadToFile(baseURL+"/checksums.txt", checksumsPath); err != nil {
		fmt.Printf("%sCould not fetch checksums.txt; skipping verification.\n", sym("warn"))
	} else if err := verifyChecksum(archivePath, checksumsPath, asset); err != nil {
		fatal("%s%v", sym("error"), err)
	}

	fmt.Printf("%sExtracting...\n", sym("info"))
	newBinary, err := extractBinaryFromTarGz(archivePath, filepath.Base(exePath))
	if err != nil {
		// Release assets always contain a binary named "tmux-workspace",
		// regardless of what the user renamed their local install to.
		newBinary, err = extractBinaryFromTarGz(archivePath, "tmux-workspace")
	}
	if err != nil {
		fatal("%s%v", sym("error"), err)
	}

	// Write alongside the target and rename into place so the swap is a
	// single atomic operation on the same filesystem, safe even though the
	// old file is the binary currently running this code.
	tmpTarget := exePath + ".new"
	if err := os.WriteFile(tmpTarget, newBinary, 0755); err != nil {
		fatal("%sCould not write updated binary: %v", sym("error"), err)
	}
	if err := os.Rename(tmpTarget, exePath); err != nil {
		os.Remove(tmpTarget)
		fatal("%sCould not replace the running binary at %s: %v", sym("error"), exePath, err)
	}

	fmt.Printf("%sUpdated to %s -> %s\n", sym("sparkle"), tag, exePath)
}

func normalizeTag(v string) string {
	return "v" + strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func verifyChecksum(archivePath, checksumsPath, asset string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	var expected string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		fmt.Printf("%sNo checksum entry found for %s; skipping verification.\n", sym("warn"), asset)
		return nil
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if actual := hex.EncodeToString(h.Sum(nil)); actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset, expected, actual)
	}
	return nil
}

// extractBinaryFromTarGz returns the contents of the first regular file in
// the archive whose base name matches name.
func extractBinaryFromTarGz(archivePath, name string) ([]byte, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == name {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("archive did not contain a %q binary", name)
}
