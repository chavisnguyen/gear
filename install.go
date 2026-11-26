package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	var force bool
	fs.BoolVar(&force, "force", false, "reinstall even if version matches")
	fs.BoolVar(&force, "f", false, "reinstall even if version matches")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	targets := fs.Args()

	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(binDir(), 0o755); err != nil {
		return fail(err)
	}

	lk := loadLock()
	st := loadState()

	// Prune tools removed from gears.toml when running a full sync.
	if len(targets) == 0 {
		for name, inst := range st.Installed {
			if _, ok := m.Tools[name]; !ok {
				removeInstalled(inst)
				delete(st.Installed, name)
				delete(lk.Tools, name)
				fmt.Printf("  - %-16s pruned (removed from gears.toml)\n", name)
			}
		}
		targets = sortedKeys(m.Tools)
	}

	if len(targets) == 0 {
		fmt.Println("Nothing to install.")
		return 0
	}

	failures := 0
	manifestDirty := false
	for _, name := range targets {
		raw, ok := m.Tools[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "  ! %-16s not defined in %s\n", name, tilde(manifestPath()))
			failures++
			continue
		}
		sp, err := parseSpec(name, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %-16s %v\n", name, err)
			failures++
			continue
		}
		if sp.Version == "" || sp.Version == "latest" {
			v, err := resolveLatestVersion(sp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ! %-16s resolve version: %v\n", name, err)
				failures++
				continue
			}
			sp.Version = normalizeVer(v)
			m.Tools[name] = sp.Format()
			manifestDirty = true
		}

		cur, hasCur := st.Installed[name]
		if !force && hasCur &&
			cur.Backend == sp.Backend &&
			cur.Package == sp.Package &&
			normalizeVer(cur.Version) == normalizeVer(sp.Version) &&
			binariesExist(cur.Bins) {
			fmt.Printf("  = %-16s %s (%s)\n", name, sp.Version, sp.Backend)
			ensureLockEntry(lk, sp)
			continue
		}

		fmt.Printf("  ↓ %-16s installing %s (%s:%s)...\n", name, sp.Version, sp.Backend, sp.Package)
		inst, locked, err := installSpec(sp, lk.Tools[name])
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %-16s FAILED: %v\n", name, err)
			failures++
			continue
		}
		// Clean up old links/dir if switching version or package.
		if hasCur && cur.Dir != inst.Dir {
			removeInstalled(cur)
		}
		st.Installed[name] = inst
		lk.Tools[name] = locked
		fmt.Printf("  + %-16s %s -> %s\n", name, inst.Version, strings.Join(shortBins(inst.Bins), ", "))
	}

	if manifestDirty {
		_ = saveManifest(m)
	}
	if err := saveLock(lk); err != nil {
		return fail(err)
	}
	if err := saveState(st); err != nil {
		return fail(err)
	}
	printPathHint()
	if failures > 0 {
		return 1
	}
	return 0
}

func ensureLockEntry(lk *Lockfile, sp Spec) {
	cur, ok := lk.Tools[sp.Name]
	if !ok || cur.Version != sp.Version || cur.Backend != sp.Backend || cur.Package != sp.Package {
		lk.Tools[sp.Name] = LockedTool{
			Backend:   sp.Backend,
			Package:   sp.Package,
			Version:   sp.Version,
			Platforms: cur.Platforms,
		}
	}
}

func installSpec(sp Spec, prevLock LockedTool) (InstalledTool, LockedTool, error) {
	switch sp.Backend {
	case "github":
		return installGitHub(sp, prevLock)
	case "npm":
		return installNPM(sp)
	case "uv":
		return installUV(sp)
	default:
		return InstalledTool{}, LockedTool{}, fmt.Errorf("unsupported backend %q", sp.Backend)
	}
}

func linkBinaries(targets []string) ([]string, error) {
	if err := os.MkdirAll(binDir(), 0o755); err != nil {
		return nil, err
	}
	var linked []string
	for _, target := range targets {
		if err := os.Chmod(target, 0o755); err != nil && !os.IsPermission(err) {
			return nil, err
		}
		dst := filepath.Join(binDir(), filepath.Base(target))
		_ = os.Remove(dst)
		if err := os.Symlink(target, dst); err != nil {
			return nil, err
		}
		linked = append(linked, dst)
	}
	return linked, nil
}

func binariesExist(bins []string) bool {
	if len(bins) == 0 {
		return false
	}
	for _, b := range bins {
		fi, err := os.Stat(b) // follows symlink
		if err != nil || fi.IsDir() {
			return false
		}
	}
	return true
}

func removeInstalled(inst InstalledTool) {
	for _, b := range inst.Bins {
		// Only remove symlink if it still points inside inst.Dir or is broken.
		if dest, err := os.Readlink(b); err == nil {
			if inst.Dir == "" || strings.HasPrefix(dest, inst.Dir) {
				_ = os.Remove(b)
			}
		}
	}
	if inst.Dir != "" && strings.HasPrefix(inst.Dir, packagesDir()) {
		_ = os.RemoveAll(inst.Dir)
	}
}

func shortBins(bins []string) []string {
	out := make([]string, len(bins))
	for i, b := range bins {
		out[i] = filepath.Base(b)
	}
	return out
}

func safeSlug(s string) string {
	r := strings.NewReplacer("/", "__", "@", "", ":", "_")
	return r.Replace(s)
}
