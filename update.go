package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	latestReleaseURL = "https://api.github.com/repos/thomasjdelaney/TwitchLiveCLI/releases/latest"
	maxReleaseSize   = 200 << 20
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Digest             string `json:"digest"`
	} `json:"assets"`
}

func updateCommand() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic updates are not supported on %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("automatic updates are not supported on %s", runtime.GOARCH)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return err
	}
	if release.TagName == "" {
		return errors.New("GitHub returned a release without a version tag")
	}
	if release.TagName == version {
		fmt.Printf("twitch-live-cli %s is already the latest release.\n", version)
		return nil
	}

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}
	assetName := fmt.Sprintf("twitch-live-cli_%s_%s%s", runtime.GOOS, runtime.GOARCH, extension)
	archiveName := assetName + ".tar.gz"
	var asset *struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Digest             string `json:"digest"`
	}
	for i := range release.Assets {
		if release.Assets[i].Name == archiveName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil || asset.BrowserDownloadURL == "" {
		return fmt.Errorf("release %s has no archive for %s/%s", release.TagName, runtime.GOOS, runtime.GOARCH)
	}

	archive, err := downloadReleaseAsset(ctx, asset.BrowserDownloadURL, asset.Digest)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find installed executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolve installed executable path: %w", err)
	}
	currentInfo, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("inspect installed executable: %w", err)
	}
	staged, err := extractReleaseExecutable(archive, assetName, filepath.Dir(executable), currentInfo.Mode().Perm())
	if err != nil {
		return err
	}
	keepStaged := false
	defer func() {
		if !keepStaged {
			os.Remove(staged)
		}
	}()

	if runtime.GOOS == "windows" {
		if err := scheduleWindowsReplacement(staged, executable); err != nil {
			return err
		}
		keepStaged = true
		fmt.Printf("Update to %s is scheduled. The new version will be installed after this process exits.\n", release.TagName)
		return nil
	}
	if err := os.Rename(staged, executable); err != nil {
		return fmt.Errorf("replace %s: %w (download the release manually if the install directory is not writable)", executable, err)
	}
	fmt.Printf("Updated twitch-live-cli to %s.\n", release.TagName)
	return nil
}

func fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return githubRelease{}, fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("check for updates: GitHub returned %s", response.Status)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("read latest release information: %w", err)
	}
	return release, nil
}

func downloadReleaseAsset(ctx context.Context, url, digest string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(request)
	if err != nil {
		return nil, fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download update: GitHub returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read update archive: %w", err)
	}
	if len(data) > maxReleaseSize {
		return nil, errors.New("update archive exceeds the 200 MiB size limit")
	}
	if digest != "" {
		algorithm, expected, found := strings.Cut(digest, ":")
		if !found || algorithm != "sha256" {
			return nil, fmt.Errorf("release asset has an unsupported digest %q", digest)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != expected {
			return nil, errors.New("update archive SHA-256 digest does not match GitHub's release metadata")
		}
	}
	return data, nil
}

func extractReleaseExecutable(archive []byte, name, directory string, mode os.FileMode) (string, error) {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", fmt.Errorf("open update archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read update archive: %w", err)
		}
		if header.Name != name {
			continue
		}
		if !header.FileInfo().Mode().IsRegular() || header.Size <= 0 || header.Size > 100<<20 {
			return "", errors.New("update archive contains an invalid executable entry")
		}
		staged, err := os.CreateTemp(directory, ".twitch-live-cli-update-*")
		if err != nil {
			return "", fmt.Errorf("create update file beside the installed executable: %w", err)
		}
		stagedPath := staged.Name()
		if _, err := io.CopyN(staged, reader, header.Size); err != nil {
			staged.Close()
			os.Remove(stagedPath)
			return "", fmt.Errorf("extract update executable: %w", err)
		}
		if err := staged.Chmod(mode); err != nil {
			staged.Close()
			os.Remove(stagedPath)
			return "", fmt.Errorf("set update executable permissions: %w", err)
		}
		if err := staged.Close(); err != nil {
			os.Remove(stagedPath)
			return "", fmt.Errorf("write update executable: %w", err)
		}
		return stagedPath, nil
	}
	return "", fmt.Errorf("update archive does not contain %s", name)
}

func scheduleWindowsReplacement(staged, executable string) error {
	script, err := os.CreateTemp("", "twitch-live-cli-update-*.ps1")
	if err != nil {
		return fmt.Errorf("create Windows update helper: %w", err)
	}
	scriptPath := script.Name()
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	contents := fmt.Sprintf(`$parentPid = %d
while (Get-Process -Id $parentPid -ErrorAction SilentlyContinue) { Start-Sleep -Milliseconds 200 }
Move-Item -LiteralPath %s -Destination %s -Force
if (-not $?) { exit 1 }
Remove-Item -LiteralPath %s -Force -ErrorAction SilentlyContinue
`, os.Getpid(), quote(staged), quote(executable), quote(scriptPath))
	if _, err := io.WriteString(script, contents); err != nil {
		script.Close()
		os.Remove(scriptPath)
		return fmt.Errorf("write Windows update helper: %w", err)
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("finish Windows update helper: %w", err)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Start(); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("start Windows update helper: %w", err)
	}
	return nil
}
