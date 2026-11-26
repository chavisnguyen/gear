package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	ghTokenOnce sync.Once
	ghTokenVal  string
	httpClient  = &http.Client{Timeout: 2 * time.Minute}
	githubAPI   = "https://api.github.com"
)

func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	ghTokenOnce.Do(func() {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err == nil {
			ghTokenVal = strings.TrimSpace(string(out))
		}
	})
	return ghTokenVal
}

// ------------------------------------------------------------ version lookup

func resolveLatestVersion(sp Spec) (string, error) {
	switch sp.Backend {
	case "github":
		rel, err := fetchGitHubRelease(sp.Package, "")
		if err != nil {
			return "", err
		}
		return normalizeVer(rel.TagName), nil
	case "npm":
		out, err := exec.Command("npm", "view", sp.Package, "version").Output()
		if err != nil {
			return "", fmt.Errorf("npm view %s: %w", sp.Package, err)
		}
		return normalizeVer(string(out)), nil
	case "uv":
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://pypi.org/pypi/%s/json", sp.Package), nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("pypi lookup %s: HTTP %d", sp.Package, resp.StatusCode)
		}
		var doc struct {
			Info struct {
				Version string `json:"version"`
			} `json:"info"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			return "", err
		}
		return normalizeVer(doc.Info.Version), nil
	}
	return "", fmt.Errorf("unsupported backend %q", sp.Backend)
}

// ------------------------------------------------------------ github backend

type ghAsset struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func githubGet(url, accept string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gear-cli/"+Version)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return httpClient.Do(req)
}

func fetchGitHubRelease(repo, version string) (*ghRelease, error) {
	var urls []string
	if version == "" || version == "latest" {
		urls = []string{fmt.Sprintf("%s/repos/%s/releases/latest", githubAPI, repo)}
	} else {
		v := normalizeVer(version)
		urls = []string{
			fmt.Sprintf("%s/repos/%s/releases/tags/v%s", githubAPI, repo, v),
			fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPI, repo, v),
		}
	}
	var lastStatus int
	for _, u := range urls {
		resp, err := githubGet(u, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 200 {
			var rel ghRelease
			err := json.NewDecoder(resp.Body).Decode(&rel)
			resp.Body.Close()
			return &rel, err
		}
		lastStatus = resp.StatusCode
		resp.Body.Close()
	}
	return nil, fmt.Errorf("github release for %s@%s not found (HTTP %d)", repo, version, lastStatus)
}

func selectAsset(assets []ghAsset, goos, goarch string) (*ghAsset, error) {
	var best *ghAsset
	bestScore := -1
	for i := range assets {
		a := &assets[i]
		score := scoreAssetName(a.Name, goos, goarch)
		if score > bestScore {
			bestScore = score
			best = a
		}
	}
	if best == nil || bestScore <= 0 {
		names := make([]string, len(assets))
		for i, a := range assets {
			names[i] = a.Name
		}
		return nil, fmt.Errorf("no matching release asset for %s/%s (available: %s)", goos, goarch, strings.Join(names, ", "))
	}
	return best, nil
}

func scoreAssetName(name, goos, goarch string) int {
	low := strings.ToLower(name)
	for _, bad := range []string{".sha256", ".sha512", ".sig", ".asc", ".sbom", "checksum", ".deb", ".rpm", ".apk", ".msi", ".dmg", ".pkg"} {
		if strings.Contains(low, bad) {
			return -1
		}
	}
	osMatch := false
	switch goos {
	case "darwin":
		if containsAny(low, "linux", "windows", "win32", "win64", "freebsd", "netbsd", "openbsd") {
			return -1
		}
		osMatch = containsAny(low, "darwin", "macos", "apple", "osx")
	case "linux":
		if containsAny(low, "darwin", "macos", "apple", "osx", "windows", "win32", "win64", "freebsd") {
			return -1
		}
		osMatch = strings.Contains(low, "linux")
	default:
		osMatch = strings.Contains(low, goos)
	}
	if !osMatch {
		return -1
	}

	archScore := 0
	switch goarch {
	case "arm64":
		if containsAny(low, "amd64", "x86_64", "x64", "386", "i386", "i686", "armv7", "armv6") {
			return -1
		}
		if containsAny(low, "arm64", "aarch64") {
			archScore = 20
		} else if containsAny(low, "universal", "all") {
			archScore = 10
		} else {
			return -1
		}
	case "amd64":
		if containsAny(low, "arm64", "aarch64", "386", "i386", "i686", "armv7", "armv6") {
			return -1
		}
		if containsAny(low, "amd64", "x86_64", "x64") {
			archScore = 20
		} else if containsAny(low, "universal", "all") {
			archScore = 10
		} else {
			return -1
		}
	default:
		if !strings.Contains(low, goarch) {
			return -1
		}
		archScore = 20
	}

	extScore := 5 // raw binary
	switch {
	case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
		extScore = 15
	case strings.HasSuffix(low, ".zip"):
		extScore = 12
	}
	return 10 + archScore + extScore
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func downloadAsset(downloadURL string) (tmpPath string, sum string, err error) {
	accept := ""
	if strings.Contains(downloadURL, "/releases/assets/") {
		accept = "application/octet-stream"
	}
	resp, err := githubGet(downloadURL, accept)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("download %s: HTTP %d", downloadURL, resp.StatusCode)
	}
	if err := os.MkdirAll(packagesDir(), 0o755); err != nil {
		return "", "", err
	}
	tmp, err := os.CreateTemp(packagesDir(), ".dl-*")
	if err != nil {
		return "", "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", "", err
	}
	return tmp.Name(), hex.EncodeToString(h.Sum(nil)), nil
}

func installGitHub(sp Spec, prevLock LockedTool) (InstalledTool, LockedTool, error) {
	plat := platformKey()
	locked := LockedTool{
		Backend:   sp.Backend,
		Package:   sp.Package,
		Version:   sp.Version,
		Platforms: map[string]PlatformAsset{},
	}
	if prevLock.Backend == sp.Backend && prevLock.Package == sp.Package && normalizeVer(prevLock.Version) == normalizeVer(sp.Version) {
		for k, v := range prevLock.Platforms {
			locked.Platforms[k] = v
		}
	}

	var dlURL, fallbackAPIURL, assetName, expectedSHA string
	if pa, ok := locked.Platforms[plat]; ok && pa.URL != "" {
		dlURL = pa.URL
		expectedSHA = pa.SHA256
		assetName = pa.Asset
		if assetName == "" {
			assetName = filepath.Base(dlURL)
		}
	} else {
		rel, err := fetchGitHubRelease(sp.Package, sp.Version)
		if err != nil {
			return InstalledTool{}, LockedTool{}, err
		}
		asset, err := selectAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return InstalledTool{}, LockedTool{}, err
		}
		assetName = asset.Name
		dlURL = asset.BrowserDownloadURL
		fallbackAPIURL = asset.URL
		if dlURL == "" {
			dlURL = asset.URL
		}
		expectedSHA = fetchReleaseChecksum(rel.Assets, asset.Name)
	}

	tmpFile, actualSHA, err := downloadAsset(dlURL)
	if err != nil && fallbackAPIURL != "" && fallbackAPIURL != dlURL {
		dlURL = fallbackAPIURL
		tmpFile, actualSHA, err = downloadAsset(dlURL)
	}
	if err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	defer os.Remove(tmpFile)

	if expectedSHA != "" && !strings.EqualFold(expectedSHA, actualSHA) {
		return InstalledTool{}, LockedTool{}, fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", assetName, expectedSHA, actualSHA)
	}

	destDir := filepath.Join(packagesDir(), "github", safeSlug(sp.Package), sp.Version)
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return InstalledTool{}, LockedTool{}, err
	}

	if err := extractOrPlace(tmpFile, assetName, sp.Name, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return InstalledTool{}, LockedTool{}, err
	}

	execs, err := findExecutables(destDir, sp.Name)
	if err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	linked, err := linkBinaries(execs)
	if err != nil {
		return InstalledTool{}, LockedTool{}, err
	}

	locked.Platforms[plat] = PlatformAsset{Asset: assetName, URL: dlURL, SHA256: actualSHA}
	return InstalledTool{
		Backend: sp.Backend,
		Package: sp.Package,
		Version: sp.Version,
		Dir:     destDir,
		Bins:    linked,
	}, locked, nil
}

func fetchReleaseChecksum(assets []ghAsset, targetName string) string {
	for _, a := range assets {
		low := strings.ToLower(a.Name)
		if low == "checksums.txt" || low == "sha256sums" || low == "sha256sums.txt" {
			u := a.BrowserDownloadURL
			if u == "" {
				u = a.URL
			}
			resp, err := githubGet(u, "application/octet-stream")
			if (err != nil || resp.StatusCode != 200) && a.URL != "" && a.URL != u {
				if resp != nil {
					resp.Body.Close()
				}
				resp, err = githubGet(a.URL, "application/octet-stream")
			}
			if err != nil {
				return ""
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return ""
			}
			for _, line := range strings.Split(string(b), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == targetName && len(fields[0]) == 64 {
					return strings.ToLower(fields[0])
				}
			}
		}
	}
	return ""
}

func extractOrPlace(archivePath, assetName, toolName, destDir string) error {
	low := strings.ToLower(assetName)
	switch {
	case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
		return extractTarGz(archivePath, destDir)
	case strings.HasSuffix(low, ".zip"):
		return extractZip(archivePath, destDir)
	default:
		dst := filepath.Join(destDir, toolName)
		in, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}

func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) && filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("unsafe path in tar archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func extractZip(src, destDir string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, zf := range zr.File {
		target := filepath.Join(destDir, zf.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("unsafe path in zip archive: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		mode := zf.Mode() & 0o777
		if mode == 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func findExecutables(destDir, toolName string) ([]string, error) {
	// 1. Exact match by toolName at root or bin/<toolName> or single nested dir.
	var exact []string
	var candidates []string
	_ = filepath.Walk(destDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		low := strings.ToLower(base)
		if strings.HasPrefix(base, ".") ||
			strings.HasSuffix(low, ".md") ||
			strings.HasSuffix(low, ".txt") ||
			strings.HasSuffix(low, ".json") ||
			strings.HasSuffix(low, ".css") ||
			strings.HasSuffix(low, ".html") ||
			strings.HasSuffix(low, ".js") ||
			strings.HasSuffix(low, ".png") ||
			strings.HasSuffix(low, ".svg") ||
			low == "license" {
			return nil
		}
		if strings.EqualFold(base, toolName) {
			exact = append(exact, path)
			return nil
		}
		if info.Mode()&0o111 != 0 {
			candidates = append(candidates, path)
		}
		return nil
	})
	if len(exact) > 0 {
		return exact[:1], nil
	}
	if len(candidates) > 0 {
		return candidates, nil
	}
	return nil, fmt.Errorf("no executable binary found in extracted package %s", destDir)
}

// --------------------------------------------------------------- npm backend

func installNPM(sp Spec) (InstalledTool, LockedTool, error) {
	if _, err := exec.LookPath("npm"); err != nil {
		return InstalledTool{}, LockedTool{}, fmt.Errorf("npm is not installed (required for npm:%s)", sp.Package)
	}
	destDir := filepath.Join(packagesDir(), "npm", safeSlug(sp.Package), sp.Version)
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	pkgSpec := sp.Package + "@" + sp.Version
	cmd := exec.Command("npm", "install", "--no-save", "--no-audit", "--no-fund", "--prefix", destDir, pkgSpec)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(destDir)
		return InstalledTool{}, LockedTool{}, fmt.Errorf("npm install %s: %v\n%s", pkgSpec, err, strings.TrimSpace(string(out)))
	}
	npmBin := filepath.Join(destDir, "node_modules", ".bin")
	entries, err := os.ReadDir(npmBin)
	if err != nil || len(entries) == 0 {
		return InstalledTool{}, LockedTool{}, fmt.Errorf("npm package %s exposed no CLI binaries in node_modules/.bin", sp.Package)
	}
	var targets []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		targets = append(targets, filepath.Join(npmBin, e.Name()))
	}
	linked, err := linkBinaries(targets)
	if err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	return InstalledTool{
			Backend: sp.Backend,
			Package: sp.Package,
			Version: sp.Version,
			Dir:     destDir,
			Bins:    linked,
		}, LockedTool{
			Backend: sp.Backend,
			Package: sp.Package,
			Version: sp.Version,
		}, nil
}

// ---------------------------------------------------------------- uv backend

func installUV(sp Spec) (InstalledTool, LockedTool, error) {
	if _, err := exec.LookPath("uv"); err != nil {
		return InstalledTool{}, LockedTool{}, fmt.Errorf("uv is not installed (required for uv:%s)", sp.Package)
	}
	destDir := filepath.Join(packagesDir(), "uv", safeSlug(sp.Package), sp.Version)
	uvBin := filepath.Join(destDir, "bin")
	_ = os.RemoveAll(destDir)
	if err := os.MkdirAll(uvBin, 0o755); err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	pkgSpec := sp.Package + "==" + sp.Version
	cmd := exec.Command("uv", "tool", "install", "--force", pkgSpec)
	cmd.Env = append(os.Environ(),
		"UV_TOOL_DIR="+filepath.Join(destDir, "tools"),
		"UV_TOOL_BIN_DIR="+uvBin,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(destDir)
		return InstalledTool{}, LockedTool{}, fmt.Errorf("uv tool install %s: %v\n%s", pkgSpec, err, strings.TrimSpace(string(out)))
	}
	entries, err := os.ReadDir(uvBin)
	if err != nil || len(entries) == 0 {
		return InstalledTool{}, LockedTool{}, fmt.Errorf("uv package %s exposed no CLI binaries", sp.Package)
	}
	var targets []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		targets = append(targets, filepath.Join(uvBin, e.Name()))
	}
	linked, err := linkBinaries(targets)
	if err != nil {
		return InstalledTool{}, LockedTool{}, err
	}
	return InstalledTool{
			Backend: sp.Backend,
			Package: sp.Package,
			Version: sp.Version,
			Dir:     destDir,
			Bins:    linked,
		}, LockedTool{
			Backend: sp.Backend,
			Package: sp.Package,
			Version: sp.Version,
		}, nil
}
