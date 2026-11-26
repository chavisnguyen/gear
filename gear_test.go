package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseSpecAndScopedNPM(t *testing.T) {
	cases := []struct {
		name, raw               string
		wantBackend, wantPkg, v string
	}{
		{"agys", "github:chavisnguyen/agys@v1.0.18", "github", "chavisnguyen/agys", "1.0.18"},
		{"agent-desktop", "npm:agent-desktop@0.2.0", "npm", "agent-desktop", "0.2.0"},
		{"mobile-mcp", "npm:@mobilenext/mobile-mcp@0.1.5", "npm", "@mobilenext/mobile-mcp", "0.1.5"},
		{"scoped-latest", "npm:@mobilenext/mobile-mcp", "npm", "@mobilenext/mobile-mcp", ""},
		{"jenkins", "uv:mcp-jenkins@0.2.0", "uv", "mcp-jenkins", "0.2.0"},
	}
	for _, tc := range cases {
		got, err := parseSpec(tc.name, tc.raw)
		if err != nil {
			t.Fatalf("parseSpec(%q) unexpected error: %v", tc.raw, err)
		}
		if got.Backend != tc.wantBackend || got.Package != tc.wantPkg || got.Version != tc.v {
			t.Fatalf("parseSpec(%q) = %+v", tc.raw, got)
		}
	}

	if got, err := inferName("npm:@mobilenext/mobile-mcp@0.1.5"); err != nil || got != "mobile-mcp" {
		t.Fatalf("inferName scoped npm = %q, %v", got, err)
	}
	if got, err := inferName("github:chavisnguyen/FiMake@1.0.43"); err != nil || got != "fimake" {
		t.Fatalf("inferName github = %q, %v", got, err)
	}
}

func TestSelectAssetScoring(t *testing.T) {
	assets := []ghAsset{
		{Name: "checksums.txt"},
		{Name: "klik-extension.zip"},
		{Name: "klik-darwin-arm64.tar.gz"},
		{Name: "klik-darwin-amd64.tar.gz"},
		{Name: "klik-linux-amd64.tar.gz"},
	}
	got, err := selectAsset(assets, "darwin", "arm64")
	if err != nil || got.Name != "klik-darwin-arm64.tar.gz" {
		t.Fatalf("darwin/arm64 got %+v, err %v", got, err)
	}
	got, err = selectAsset(assets, "linux", "amd64")
	if err != nil || got.Name != "klik-linux-amd64.tar.gz" {
		t.Fatalf("linux/amd64 got %+v, err %v", got, err)
	}

	// Underscore / x86_64 / aarch64 style (like mcps / Rust releases)
	rustAssets := []ghAsset{
		{Name: "cmob-aarch64-apple-darwin.tar.gz"},
		{Name: "cmob-x86_64-unknown-linux-gnu.tar.gz"},
	}
	got, err = selectAsset(rustAssets, "darwin", "arm64")
	if err != nil || got.Name != "cmob-aarch64-apple-darwin.tar.gz" {
		t.Fatalf("rust darwin/arm64 got %+v, err %v", got, err)
	}
	got, err = selectAsset(rustAssets, "linux", "amd64")
	if err != nil || got.Name != "cmob-x86_64-unknown-linux-gnu.tar.gz" {
		t.Fatalf("rust linux/amd64 got %+v, err %v", got, err)
	}
}

func makeFakeTarGz(t *testing.T, binName, version string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	script := fmt.Sprintf("#!/bin/sh\necho \"%s %s\"\n", binName, version)
	hdr := &tar.Header{
		Name: binName,
		Mode: 0o755,
		Size: int64(len(script)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	// Companion file (like style.css or extension/manifest.json)
	extra := "body { color: black; }\n"
	if err := tw.WriteHeader(&tar.Header{Name: "style.css", Mode: 0o644, Size: int64(len(extra))}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte(extra))
	_ = tw.Close()
	_ = gz.Close()

	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestEndToEndGitHubInstallUpgradeAndLock(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("GEAR_HOME", tmpHome)
	t.Setenv("GITHUB_TOKEN", "test-token")

	assetName := fmt.Sprintf("demo-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	v1Bytes, v1SHA := makeFakeTarGz(t, "demo", "1.0.0")
	v2Bytes, v2SHA := makeFakeTarGz(t, "demo", "2.0.0")

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/demo/releases/tags/v1.0.0":
			_ = json.NewEncoder(w).Encode(ghRelease{
				TagName: "v1.0.0",
				Assets: []ghAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/dl/v1/" + assetName},
					{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/dl/v1/checksums.txt"},
				},
			})
		case "/repos/acme/demo/releases/latest", "/repos/acme/demo/releases/tags/v2.0.0":
			_ = json.NewEncoder(w).Encode(ghRelease{
				TagName: "v2.0.0",
				Assets: []ghAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/dl/v2/" + assetName},
					{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/dl/v2/checksums.txt"},
				},
			})
		case "/dl/v1/" + assetName:
			_, _ = w.Write(v1Bytes)
		case "/dl/v1/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", v1SHA, assetName)
		case "/dl/v2/" + assetName:
			_, _ = w.Write(v2Bytes)
		case "/dl/v2/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", v2SHA, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origAPI := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = origAPI }()

	if code := run([]string{"init"}); code != 0 {
		t.Fatalf("gear init failed: %d", code)
	}
	if code := run([]string{"add", "github:acme/demo@1.0.0"}); code != 0 {
		t.Fatalf("gear add v1.0.0 failed: %d", code)
	}

	binPath := filepath.Join(tmpHome, "bin", "demo")
	out, err := exec.Command(binPath).Output()
	if err != nil || strings.TrimSpace(string(out)) != "demo 1.0.0" {
		t.Fatalf("expected 'demo 1.0.0', got %q (%v)", string(out), err)
	}

	// Verify companion file preserved in package dir
	destStyle := filepath.Join(tmpHome, "packages", "github", "acme__demo", "1.0.0", "style.css")
	if !exists(destStyle) {
		t.Fatalf("expected companion style.css at %s", destStyle)
	}

	// Verify lockfile recorded exact SHA256
	lk := loadLock()
	if got := lk.Tools["demo"].Platforms[platformKey()].SHA256; got != v1SHA {
		t.Fatalf("lockfile sha = %q, want %q", got, v1SHA)
	}

	// Upgrade to latest (v2.0.0)
	if code := run([]string{"upgrade"}); code != 0 {
		t.Fatalf("gear upgrade failed: %d", code)
	}
	out, err = exec.Command(binPath).Output()
	if err != nil || strings.TrimSpace(string(out)) != "demo 2.0.0" {
		t.Fatalf("expected 'demo 2.0.0' after upgrade, got %q (%v)", string(out), err)
	}

	// Remove tool and ensure symlink is cleaned up
	if code := run([]string{"rm", "demo"}); code != 0 {
		t.Fatalf("gear rm failed: %d", code)
	}
	if exists(binPath) {
		t.Fatalf("expected %s to be removed", binPath)
	}
}
