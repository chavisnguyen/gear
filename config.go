package main

import (
	"encoding/json"
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

func lockPath() string {
	return filepath.Join(filepath.Dir(manifestPath()), "gears.lock")
}

func statePath() string    { return filepath.Join(gearHome(), "state.json") }
func binDir() string       { return filepath.Join(gearHome(), "bin") }
func packagesDir() string  { return filepath.Join(gearHome(), "packages") }
func platformKey() string  { return runtime.GOOS + "-" + runtime.GOARCH }

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
			return nil, fmt.Errorf("%s: [tools] must be a table", label)
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
	// Also accept flat top-level `name = "backend:pkg@ver"` files.
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
	Backend string // github | npm | uv
	Package string // owner/repo or pkg or @scope/pkg
	Version string // semver without leading "v" (empty = latest before resolve)
}

func (s Spec) Format() string {
	if s.Version == "" {
		return s.Backend + ":" + s.Package
	}
	return s.Backend + ":" + s.Package + "@" + normalizeVer(s.Version)
}

func normalizeVer(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func parseSpec(name, raw string) (Spec, error) {
	raw = strings.TrimSpace(raw)
	backend, rest, ok := strings.Cut(raw, ":")
	if !ok || rest == "" {
		return Spec{}, fmt.Errorf("%s: invalid spec %q (expected <github|npm|uv>:<package>@<version>)", name, raw)
	}
	switch backend {
	case "github", "npm", "uv":
	default:
		return Spec{}, fmt.Errorf("%s: unsupported backend %q (supported: github, npm, uv)", name, backend)
	}
	pkg, ver := splitPkgVersion(rest)
	if pkg == "" {
		return Spec{}, fmt.Errorf("%s: empty package in %q", name, raw)
	}
	if backend == "github" && strings.Count(pkg, "/") != 1 {
		return Spec{}, fmt.Errorf("%s: github spec must be github:owner/repo[@version], got %q", name, raw)
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
		return "", fmt.Errorf("invalid spec %q (expected <github|npm|uv>:<package>[@version])", raw)
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

// ---------------------------------------------------------------- lockfile

type PlatformAsset struct {
	Asset  string `toml:"asset,omitempty"`
	URL    string `toml:"url"`
	SHA256 string `toml:"sha256"`
}

type LockedTool struct {
	Backend   string                   `toml:"backend"`
	Package   string                   `toml:"package"`
	Version   string                   `toml:"version"`
	Platforms map[string]PlatformAsset `toml:"platforms,omitempty"`
}

type Lockfile struct {
	Tools map[string]LockedTool `toml:"tools"`
}

func loadLock() *Lockfile {
	lk := &Lockfile{Tools: map[string]LockedTool{}}
	if b, err := os.ReadFile(lockPath()); err == nil {
		_, _ = toml.Decode(string(b), lk)
		if lk.Tools == nil {
			lk.Tools = map[string]LockedTool{}
		}
	}
	return lk
}

func saveLock(lk *Lockfile) error {
	var b strings.Builder
	b.WriteString("# gears.lock - auto-generated by gear. Commit this file to lock checksums across machines.\n\n")
	if err := toml.NewEncoder(&b).Encode(lk); err != nil {
		return err
	}
	return writeAtomic(lockPath(), []byte(b.String()), 0o644)
}

// ------------------------------------------------------------------- state

type InstalledTool struct {
	Backend string   `json:"backend"`
	Package string   `json:"package"`
	Version string   `json:"version"`
	Dir     string   `json:"dir"`
	Bins    []string `json:"bins"`
}

type State struct {
	Installed map[string]InstalledTool `json:"installed"`
}

func loadState() *State {
	s := &State{Installed: map[string]InstalledTool{}}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, s)
		if s.Installed == nil {
			s.Installed = map[string]InstalledTool{}
		}
	}
	return s
}

func saveState(s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(statePath(), append(b, '\n'), 0o644)
}
