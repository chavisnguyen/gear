package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	ghTokenOnce sync.Once
	ghTokenVal  string
	httpClient  = &http.Client{Timeout: 20 * time.Second}
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

// ----------------------------------------------------------- system snapshot

type systemSnapshot struct {
	brewVersions map[string]string
	brewServices map[string]string
	brewTaps     map[string]bool
	npmVersions  map[string]string
	uvVersions   map[string]string
}

func probeSystem(declared map[string]string) *systemSnapshot {
	needBrew, needNPM, needUV := false, false, false
	for _, raw := range declared {
		switch {
		case strings.HasPrefix(raw, "brew:"):
			needBrew = true
		case strings.HasPrefix(raw, "npm:"):
			needNPM = true
		case strings.HasPrefix(raw, "uv:"):
			needUV = true
		}
	}
	s := &systemSnapshot{
		brewVersions: map[string]string{},
		brewServices: map[string]string{},
		brewTaps:     map[string]bool{},
		npmVersions:  map[string]string{},
		uvVersions:   map[string]string{},
	}
	if needBrew {
		if out, err := brewOutput("list", "--formula", "--versions"); err == nil {
			s.brewVersions = parseBrewVersions(out)
		}
		if out, err := brewOutput("services", "list"); err == nil {
			s.brewServices = parseBrewServices(out)
		}
		if out, err := brewOutput("tap"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				t := strings.ToLower(strings.TrimSpace(line))
				if t != "" {
					s.brewTaps[t] = true
				}
			}
		}
	}
	if needNPM {
		cmd := exec.Command("npm", "list", "-g", "--depth=0", "--json")
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		if out, err := cmd.Output(); err == nil {
			s.npmVersions = parseNPMListJSON(out)
		}
	}
	if needUV {
		cmd := exec.Command("uv", "tool", "list")
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		if out, err := cmd.Output(); err == nil {
			s.uvVersions = parseUVToolList(string(out))
		}
	}
	return s
}

func (s *systemSnapshot) installedVersion(sp Spec) string {
	switch sp.Backend {
	case "brew":
		bt, err := parseBrewTarget(sp.Package)
		if err != nil {
			return ""
		}
		return s.brewVersions[strings.ToLower(bt.ShortName)]
	case "npm":
		return s.npmVersions[sp.Package]
	case "uv":
		return s.uvVersions[strings.ToLower(sp.Package)]
	}
	return ""
}

func (s *systemSnapshot) brewService(sp Spec) (string, bool) {
	if sp.Backend != "brew" {
		return "", false
	}
	bt, err := parseBrewTarget(sp.Package)
	if err != nil {
		return "", false
	}
	st, ok := s.brewServices[strings.ToLower(bt.ShortName)]
	return st, ok
}

func parseBrewServices(out string) map[string]string {
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.ToLower(fields[0])
		if name == "name" && strings.EqualFold(fields[1], "status") {
			continue
		}
		res[name] = strings.ToLower(fields[1])
	}
	return res
}

func parseBrewVersions(out string) map[string]string {
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Last field is the latest installed cellar version.
			res[strings.ToLower(fields[0])] = normalizeVer(fields[len(fields)-1])
		}
	}
	return res
}

func parseNPMListJSON(b []byte) map[string]string {
	res := map[string]string{}
	var doc struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &doc); err == nil {
		for k, v := range doc.Dependencies {
			if v.Version != "" {
				res[k] = normalizeVer(v.Version)
			}
		}
	}
	return res
}

func parseUVToolList(out string) map[string]string {
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			res[strings.ToLower(fields[0])] = normalizeVer(fields[1])
		}
	}
	return res
}

// --------------------------------------------------------- install / upgrade

func brewEnv() []string {
	return append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1")
}

func brewOutput(args ...string) (string, error) {
	cmd := exec.Command("brew", args...)
	cmd.Env = brewEnv()
	out, err := cmd.Output()
	return string(out), err
}

func runBrew(args ...string) error {
	cmd := exec.Command("brew", args...)
	cmd.Env = brewEnv()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func refreshBrewTap(bt BrewTarget, snap *systemSnapshot) {
	if bt.TapName == "" || snap == nil || !snap.brewTaps[strings.ToLower(bt.TapName)] {
		return
	}
	if repoDir, err := brewOutput("--repository", bt.TapName); err == nil {
		dir := strings.TrimSpace(repoDir)
		if dir != "" {
			_ = exec.Command("git", "-C", dir, "pull", "--ff-only").Run()
		}
	}
}

func applySpec(sp Spec, alreadyInstalled, force bool, snap *systemSnapshot) error {
	switch sp.Backend {
	case "brew":
		if _, err := exec.LookPath("brew"); err != nil {
			return fmt.Errorf("brew is not installed")
		}
		bt, err := parseBrewTarget(sp.Package)
		if err != nil {
			return err
		}
		if bt.TapName != "" {
			if !snap.brewTaps[strings.ToLower(bt.TapName)] {
				if err := runBrew("tap", bt.TapName, bt.TapURL); err != nil {
					return fmt.Errorf("brew tap %s: %w", bt.TapName, err)
				}
				snap.brewTaps[strings.ToLower(bt.TapName)] = true
			} else if alreadyInstalled {
				refreshBrewTap(bt, snap)
			}
		}
		var errInstall error
		if force && alreadyInstalled {
			errInstall = runBrew("reinstall", bt.FormulaRef)
		} else if alreadyInstalled {
			errInstall = runBrew("upgrade", bt.FormulaRef)
		} else {
			errInstall = runBrew("install", bt.FormulaRef)
		}
		if errInstall != nil {
			return errInstall
		}
		// Refresh brew services list so newly installed formulae with a service block are detected.
		if out, err := brewOutput("services", "list"); err == nil {
			snap.brewServices = parseBrewServices(out)
		}
		if svc, hasSvc := snap.brewServices[strings.ToLower(bt.ShortName)]; hasSvc {
			if alreadyInstalled && (svc == "started" || svc == "scheduled" || force) {
				return runBrew("services", "restart", bt.FormulaRef)
			}
			return runBrew("services", "start", bt.FormulaRef)
		}
		return nil

	case "npm":
		if _, err := exec.LookPath("npm"); err != nil {
			return fmt.Errorf("npm is not installed")
		}
		pkgSpec := sp.Package
		if sp.Version != "" {
			pkgSpec += "@" + sp.Version
		}
		cmd := exec.Command("npm", "install", "-g", "--no-audit", "--no-fund", pkgSpec)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()

	case "uv":
		if _, err := exec.LookPath("uv"); err != nil {
			return fmt.Errorf("uv is not installed")
		}
		pkgSpec := sp.Package
		if sp.Version != "" {
			pkgSpec += "==" + sp.Version
		}
		cmd := exec.Command("uv", "tool", "install", "--force", pkgSpec)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("unsupported backend %q", sp.Backend)
}

func uninstallSpec(sp Spec) error {
	switch sp.Backend {
	case "brew":
		bt, err := parseBrewTarget(sp.Package)
		if err != nil {
			return err
		}
		return runBrew("uninstall", bt.ShortName)
	case "npm":
		cmd := exec.Command("npm", "uninstall", "-g", sp.Package)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	case "uv":
		cmd := exec.Command("uv", "tool", "uninstall", sp.Package)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	return nil
}

// ------------------------------------------------------------ version lookup

func resolveLatestVersion(sp Spec) (string, error) {
	switch sp.Backend {
	case "brew":
		bt, err := parseBrewTarget(sp.Package)
		if err != nil {
			return "", err
		}
		if out, err := brewOutput("info", "--json=v2", bt.FormulaRef); err == nil {
			var doc struct {
				Formulae []struct {
					Versions struct {
						Stable string `json:"stable"`
					} `json:"versions"`
				} `json:"formulae"`
			}
			if json.Unmarshal([]byte(out), &doc) == nil && len(doc.Formulae) > 0 && doc.Formulae[0].Versions.Stable != "" {
				return normalizeVer(doc.Formulae[0].Versions.Stable), nil
			}
		}
		if bt.GitHubRepo != "" {
			if v, err := fetchLatestGitHubTag(bt.GitHubRepo); err == nil && v != "" {
				return v, nil
			}
		}
		return "", fmt.Errorf("could not resolve brew version for %s", bt.FormulaRef)

	case "npm":
		out, err := exec.Command("npm", "view", sp.Package, "version").Output()
		if err != nil {
			return "", fmt.Errorf("npm view %s: %w", sp.Package, err)
		}
		return normalizeVer(string(out)), nil

	case "uv":
		resp, err := httpClient.Get(fmt.Sprintf("https://pypi.org/pypi/%s/json", sp.Package))
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

func fetchLatestGitHubTag(repo string) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/releases/latest", githubAPI, repo), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gear-cli/"+Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return normalizeVer(rel.TagName), nil
}
