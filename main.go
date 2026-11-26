package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const Version = "0.1.0"

const usage = `gear - lightweight multi-device tool & version hub.

Quick start
  1. gear init [git-url]          create ~/.gear/config/gears.toml (or clone your config repo)
  2. gear add github:owner/repo   pin & install a tool into ~/.gear/bin
  3. gear install                 install/sync every tool to its pinned version

Commands
  gear status                     show declared vs installed versions (default when configured)
  gear install [name...]          install or switch tools to match gears.toml (alias: sync, up)
  gear add [name] <spec>          add & install a tool (github:owner/repo[@ver], npm:pkg[@ver], uv:pkg[@ver])
  gear rm <name...>               remove tool(s) from gears.toml and unlink binaries
  gear upgrade [name...]          upgrade tool(s) to latest upstream release & update gears.toml
  gear init [git-url]             initialize ~/.gear/config or clone/pull from a git repository
  gear pull                       git pull ~/.gear/config and run gear install
  gear push [message]             commit & push gears.toml + gears.lock in ~/.gear/config
  gear doctor                     check PATH, tool health, broken links, and config state
  gear path                       print ~/.gear/bin
  gear version                    print version

Flags for install:  --force (reinstall even if version matches)
Env:  GEAR_HOME (default ~/.gear), GEAR_CONFIG (path to gears.toml), GITHUB_TOKEN / GH_TOKEN
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

func run(args []string) int {
	if len(args) == 0 {
		if exists(manifestPath()) {
			return cmdStatus()
		}
		fmt.Print(usage)
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	case "-v", "--version", "version":
		fmt.Printf("gear v%s\n", Version)
		return 0
	case "path":
		fmt.Println(binDir())
		return 0
	case "init":
		return cmdInit(rest)
	case "status", "list", "ls":
		return cmdStatus()
	case "install", "sync", "up":
		return cmdInstall(rest)
	case "add":
		return cmdAdd(rest)
	case "rm", "remove":
		return cmdRemove(rest)
	case "upgrade", "update":
		return cmdUpgrade(rest)
	case "pull":
		return cmdPull()
	case "push":
		return cmdPush(rest)
	case "doctor":
		return cmdDoctor()
	}
	fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
	return 2
}

const defaultManifestTemplate = `# gears.toml - every CLI/tool you use across machines, pinned in one place.
# Backends:
#   github:owner/repo@version    prebuilt binary from GitHub Releases
#   npm:package@version          isolated npm CLI linked into ~/.gear/bin
#   uv:package@version           isolated Python CLI via uv linked into ~/.gear/bin

[tools]
# mcps          = "github:chavisnguyen/mcps@0.5.2"
# agys          = "github:chavisnguyen/agys@1.0.18"
# klik          = "github:chavisnguyen/klik@3.1.0"
# agent-desktop = "npm:agent-desktop@0.2.0"
`

func cmdInit(args []string) int {
	if len(args) == 0 {
		p := manifestPath()
		if exists(p) {
			fmt.Println("config already exists:", tilde(p))
			return 0
		}
		if err := writeAtomic(p, []byte(defaultManifestTemplate), 0o644); err != nil {
			return fail(err)
		}
		_ = os.MkdirAll(binDir(), 0o755)
		fmt.Println("created", tilde(p))
		printPathHint()
		return 0
	}
	if len(args) != 1 {
		return fail(fmt.Errorf("usage: gear init [git-url]"))
	}
	dir := configDir()
	var c *exec.Cmd
	if exists(filepath.Join(dir, ".git")) {
		c = exec.Command("git", "-C", dir, "pull", "--ff-only")
	} else {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return fail(err)
		}
		c = exec.Command("git", "clone", args[0], dir)
	}
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fail(fmt.Errorf("git: %w", err))
	}
	_ = os.MkdirAll(binDir(), 0o755)
	fmt.Println("config ready:", tilde(dir))
	printPathHint()
	return 0
}

func cmdStatus() int {
	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	st := loadState()
	if len(m.Tools) == 0 {
		fmt.Printf("No tools declared in %s\nRun `gear add <backend>:<pkg>` to add one.\n", tilde(manifestPath()))
		return 0
	}
	names := sortedKeys(m.Tools)
	wName, wSpec, wVer := 4, 6, 9
	type row struct {
		name, source, declared, installed, state string
	}
	var rows []row
	for _, n := range names {
		raw := m.Tools[n]
		sp, err := parseSpec(n, raw)
		if err != nil {
			rows = append(rows, row{n, raw, "?", "-", "! invalid spec"})
			continue
		}
		src := sp.Backend + ":" + sp.Package
		inst, ok := st.Installed[n]
		instVer := "-"
		status := "○ not installed"
		if ok && binariesExist(inst.Bins) {
			instVer = inst.Version
			if normalizeVer(inst.Version) == normalizeVer(sp.Version) && inst.Backend == sp.Backend && inst.Package == sp.Package {
				status = "✓ ok"
			} else {
				status = "↻ drift (run `gear install`)"
			}
		}
		wName = max(wName, len(n))
		wSpec = max(wSpec, len(src))
		wVer = max(wVer, max(len(sp.Version), len(instVer)))
		rows = append(rows, row{n, src, sp.Version, instVer, status})
	}
	fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n", wName, "TOOL", wSpec, "SOURCE", wVer, "DECLARED", wVer, "INSTALLED", "STATUS")
	for _, r := range rows {
		fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n", wName, r.name, wSpec, r.source, wVer, r.declared, wVer, r.installed, r.state)
	}
	return 0
}

func cmdAdd(args []string) int {
	if len(args) < 1 || len(args) > 2 {
		return fail(fmt.Errorf("usage: gear add [name] <github:owner/repo[@ver] | npm:pkg[@ver] | uv:pkg[@ver]>"))
	}
	var name, raw string
	if len(args) == 1 {
		raw = args[0]
		inferred, err := inferName(raw)
		if err != nil {
			return fail(err)
		}
		name = inferred
	} else {
		name, raw = args[0], args[1]
	}
	sp, err := parseSpec(name, raw)
	if err != nil {
		return fail(err)
	}
	if sp.Version == "" || sp.Version == "latest" {
		fmt.Printf("→ resolving latest version for %s:%s...\n", sp.Backend, sp.Package)
		v, err := resolveLatestVersion(sp)
		if err != nil {
			return fail(err)
		}
		sp.Version = normalizeVer(v)
	}
	m, err := loadOrInitManifest()
	if err != nil {
		return fail(err)
	}
	m.Tools[name] = sp.Format()
	if err := saveManifest(m); err != nil {
		return fail(err)
	}
	fmt.Printf("+ saved %s = %q in %s\n", name, sp.Format(), tilde(manifestPath()))
	return cmdInstall([]string{name})
}

func cmdRemove(args []string) int {
	if len(args) == 0 {
		return fail(fmt.Errorf("usage: gear rm <name...>"))
	}
	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	lk := loadLock()
	st := loadState()
	for _, name := range args {
		delete(m.Tools, name)
		delete(lk.Tools, name)
		if inst, ok := st.Installed[name]; ok {
			removeInstalled(inst)
			delete(st.Installed, name)
		}
		fmt.Printf("- removed %s\n", name)
	}
	if err := saveManifest(m); err != nil {
		return fail(err)
	}
	_ = saveLock(lk)
	_ = saveState(st)
	return 0
}

func cmdUpgrade(args []string) int {
	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	targets := args
	if len(targets) == 0 {
		targets = sortedKeys(m.Tools)
	}
	var upgraded []string
	for _, name := range targets {
		raw, ok := m.Tools[name]
		if !ok {
			return fail(fmt.Errorf("unknown tool %q", name))
		}
		sp, err := parseSpec(name, raw)
		if err != nil {
			return fail(err)
		}
		latest, err := resolveLatestVersion(sp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: could not check latest: %v\n", name, err)
			continue
		}
		latest = normalizeVer(latest)
		if normalizeVer(sp.Version) == latest {
			fmt.Printf("  = %-16s %s (up to date)\n", name, sp.Version)
			continue
		}
		fmt.Printf("  ↑ %-16s %s -> %s\n", name, sp.Version, latest)
		sp.Version = latest
		m.Tools[name] = sp.Format()
		upgraded = append(upgraded, name)
	}
	if len(upgraded) == 0 {
		return 0
	}
	if err := saveManifest(m); err != nil {
		return fail(err)
	}
	return cmdInstall(upgraded)
}

func cmdPull() int {
	dir := configDir()
	if !exists(filepath.Join(dir, ".git")) {
		return fail(fmt.Errorf("%s is not a git repository (run `gear init <git-url>`)", tilde(dir)))
	}
	c := exec.Command("git", "-C", dir, "pull", "--rebase")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fail(err)
	}
	return cmdInstall(nil)
}

func cmdPush(args []string) int {
	dir := configDir()
	if !exists(filepath.Join(dir, ".git")) {
		return fail(fmt.Errorf("%s is not a git repository (run `gear init <git-url>`)", tilde(dir)))
	}
	msg := "chore: update gears"
	if len(args) > 0 {
		msg = strings.Join(args, " ")
	}
	add := exec.Command("git", "-C", dir, "add", "gears.toml", "gears.lock")
	_ = add.Run()
	diff := exec.Command("git", "-C", dir, "diff", "--cached", "--quiet")
	if diff.Run() != nil {
		commit := exec.Command("git", "-C", dir, "commit", "-m", msg)
		commit.Stdout, commit.Stderr = os.Stdout, os.Stderr
		if err := commit.Run(); err != nil {
			return fail(err)
		}
	}
	push := exec.Command("git", "-C", dir, "push")
	push.Stdout, push.Stderr = os.Stdout, os.Stderr
	if err := push.Run(); err != nil {
		return fail(err)
	}
	return 0
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
