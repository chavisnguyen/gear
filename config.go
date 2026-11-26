package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// ------------------------------------------------------------------- paths

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func gearHome() string {
	if h := os.Getenv("GEAR_HOME"); h != "" {
		return h
	}
	return filepath.Join(homeDir(), ".gear")
}

func configDir() string { return filepath.Join(gearHome(), "config") }

func manifestPath() string {
	if p := os.Getenv("GEAR_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(configDir(), "gears.toml")
}

func platformKey() string { return runtime.GOOS + "-" + runtime.GOARCH }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func tilde(p string) string {
	if h := homeDir(); h != "" && (p == h || strings.HasPrefix(p, h+string(os.PathSeparator))) {
		return "~" + p[len(h):]
	}
	return p
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gear-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ---------------------------------------------------------------- manifest

type Manifest struct {
	Tools map[string]string `toml:"tools"`
}

func loadManifest() (*Manifest, error) {
	p := manifestPath()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config at %s (run `gear init` first)", tilde(p))
		}
		return nil, err
	}
	return parseManifestBytes(b, p)
}

func parseManifestBytes(b []byte, label string) (*Manifest, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(b), &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	m := &Manifest{Tools: map[string]string{}}
	if t, ok := raw["tools"]; ok {
		sub, ok := t.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: [tools] must be a table", label, )
		}
		for k, v := range sub {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s: tools.%s must be a string spec", label, k)
			}
			m.Tools[k] = s
		}
		if len(raw) > 1 {
			for k := range raw {
				if k != "tools" {
					return nil, fmt.Errorf("%s: unknown top-level key %q (put tool entries under [tools])", label, k)
				}
			}
		}
		return m, nil
	}
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s: %q must be a string spec", label, k)
		}
		m.Tools[k] = s
	}
	return m, nil
}

func loadOrInitManifest() (*Manifest, error) {
	if exists(manifestPath()) {
		return loadManifest()
	}
	return &Manifest{Tools: map[string]string{}}, nil
}

func saveManifest(m *Manifest) error {
	var b strings.Builder
	b.WriteString("# gears.toml - managed by gear (https://github.com/chavisnguyen/gear)\n\n[tools]\n")
	names := sortedKeys(m.Tools)
	width := 0
	for _, n := range names {
		width = max(width, len(n))
	}
	for _, n := range names {
		key := n
		if strings.ContainsAny(n, " /:@.") {
			key = fmt.Sprintf("%q", n)
		}
		fmt.Fprintf(&b, "%-*s = %q\n", width, key, m.Tools[n])
	}
	return writeAtomic(manifestPath(), []byte(b.String()), 0o644)
}

// ------------------------------------------------------------------- specs

type Spec struct {
	Name    string
	Backend string // brew | npm | uv
	Package string // formula or owner/repo/formula or pkg or @scope/pkg
	Version string // semver without leading "v" (empty = any/latest)
}

func (s Spec) Format() string {
	if s.Version == "" {
		return s.Backend + ":" + s.Package
	}
	return s.Backend + ":" + s.Package + "@" + normalizeVer(s.Version)
}

func normalizeVer(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// Strip Homebrew revision suffix like "1.3.1_1" when comparing base semver if needed.
	return v
}

func versionsMatch(installed, declared string) bool {
	a := normalizeVer(installed)
	b := normalizeVer(declared)
	if a == b {
		return true
	}
	// Homebrew revisions look like "1.3.1_1" for upstream "1.3.1"
	if base, _, ok := strings.Cut(a, "_"); ok && base == b {
		return true
	}
	return false
}

func parseSpec(name, raw string) (Spec, error) {
	raw = strings.TrimSpace(raw)
	backend, rest, ok := strings.Cut(raw, ":")
	if !ok || rest == "" {
		return Spec{}, fmt.Errorf("%s: invalid spec %q (expected <brew|npm|uv>:<package>[@<version>])", name, raw)
	}
	switch backend {
	case "brew", "npm", "uv":
	default:
		return Spec{}, fmt.Errorf("%s: unsupported backend %q (supported: brew, npm, uv)", name, backend)
	}
	pkg, ver := splitPkgVersion(rest)
	if pkg == "" {
		return Spec{}, fmt.Errorf("%s: empty package in %q", name, raw)
	}
	if backend == "brew" {
		if _, err := parseBrewTarget(pkg); err != nil {
			return Spec{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	return Spec{
		Name:    name,
		Backend: backend,
		Package: pkg,
		Version: normalizeVer(ver),
	}, nil
}

// splitPkgVersion splits "pkg@1.2.3" and "@scope/pkg@1.2.3" cleanly.
func splitPkgVersion(s string) (pkg, ver string) {
	idx := strings.LastIndex(s, "@")
	if idx <= 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

func inferName(raw string) (string, error) {
	_, rest, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok || rest == "" {
		return "", fmt.Errorf("invalid spec %q (expected <brew|npm|uv>:<package>[@version])", raw)
	}
	pkg, _ := splitPkgVersion(rest)
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		pkg = pkg[i+1:]
	}
	pkg = strings.ToLower(strings.TrimSpace(pkg))
	if pkg == "" {
		return "", fmt.Errorf("cannot infer tool name from %q", raw)
	}
	return pkg, nil
}

// BrewTarget resolves "agent-desktop", "chavisnguyen/klik", or "chavisnguyen/FiMake/fimake".
type BrewTarget struct {
	TapName    string // e.g. "chavisnguyen/fimake" (empty for homebrew/core)
	TapURL     string // e.g. "https://github.com/chavisnguyen/FiMake" (empty if standard tap or core)
	GitHubRepo string // e.g. "chavisnguyen/FiMake" (empty for core)
	FormulaRef string // e.g. "chavisnguyen/fimake/fimake" or "agent-desktop"
	ShortName  string // e.g. "fimake" or "agent-desktop"
}

func parseBrewTarget(pkg string) (BrewTarget, error) {
	parts := strings.Split(strings.TrimSpace(pkg), "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return BrewTarget{}, fmt.Errorf("empty brew formula")
		}
		return BrewTarget{
			FormulaRef: parts[0],
			ShortName:  parts[0],
		}, nil
	case 2:
		owner, repo := parts[0], parts[1]
		if owner == "" || repo == "" {
			return BrewTarget{}, fmt.Errorf("invalid brew spec %q", pkg)
		}
		tapRepo := strings.TrimPrefix(strings.ToLower(repo), "homebrew-")
		tapName := strings.ToLower(owner) + "/" + tapRepo
		formula := tapRepo
		return BrewTarget{
			TapName:    tapName,
			TapURL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
			GitHubRepo: owner + "/" + repo,
			FormulaRef: tapName + "/" + formula,
			ShortName:  formula,
		}, nil
	case 3:
		owner, repo, formula := parts[0], parts[1], parts[2]
		if owner == "" || repo == "" || formula == "" {
			return BrewTarget{}, fmt.Errorf("invalid brew spec %q", pkg)
		}
		tapRepo := strings.TrimPrefix(strings.ToLower(repo), "homebrew-")
		tapName := strings.ToLower(owner) + "/" + tapRepo
		return BrewTarget{
			TapName:    tapName,
			TapURL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
			GitHubRepo: owner + "/" + repo,
			FormulaRef: tapName + "/" + formula,
			ShortName:  formula,
		}, nil
	default:
		return BrewTarget{}, fmt.Errorf("invalid brew spec %q (expected formula or owner/repo[/formula])", pkg)
	}
}
