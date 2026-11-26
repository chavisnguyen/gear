package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const Version = "0.2.2"

const usage = `gear - declarative multi-device CLI tool hub over brew, npm, and uv.

Quick start
  1. gear init [git-url]          create ~/.gear/config/gears.toml (or clone your config repo)
  2. gear add brew:owner/repo     add & install a tool via Homebrew, npm -g, or uv tool
  3. gear install                 install/upgrade every tool to match gears.toml

Commands
  gear status                     show declared vs installed versions (default when configured)
  gear install [name...]          install missing tools & upgrade outdated ones (alias: sync, up)
  gear add [name] <spec>          add & install a tool (brew:..., npm:..., uv:...)
  gear rm [--uninstall] <name...> remove tool(s) from gears.toml (optionally uninstall from system)
  gear upgrade [name...]          upgrade tool(s) to latest upstream release & update gears.toml
  gear init [git-url]             initialize ~/.gear/config or clone/pull from a git repository
  gear pull                       git pull ~/.gear/config and run gear install
  gear push [message]             commit & push gears.toml in ~/.gear/config
  gear doctor                     check package managers, config git sync, and version drift
  gear version                    print version

Env:  GEAR_HOME (default ~/.gear), GEAR_CONFIG (path to gears.toml)
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, red("error:"), err)
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

const defaultManifestTemplate = `# gears.toml - every CLI tool you use across machines, declared once.
# Backends:
#   brew:owner/repo/formula[@ver]   Homebrew tap + formula (or brew:formula[@ver] from homebrew/core)
#   npm:package[@ver]               Global npm CLI (npm install -g)
#   uv:package[@ver]                Isolated Python CLI (uv tool install)

[tools]
# mcps          = "brew:chavisnguyen/mcps/mcps@0.5.3"
# agys          = "brew:chavisnguyen/agys/agys@1.0.19"
# klik          = "brew:chavisnguyen/klik/klik@3.2.0"
# agent-desktop = "brew:agent-desktop@1.3.1"
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
		fmt.Println("created", tilde(p))
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
	fmt.Println("config ready:", tilde(dir))
	return 0
}

func cmdStatus() int {
	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	if len(m.Tools) == 0 {
		fmt.Printf("No tools declared in %s\nRun `gear add <brew|npm|uv>:<pkg>` to add one.\n", tilde(manifestPath()))
		return 0
	}
	snap := probeSystem(m.Tools)
	names := sortedKeys(m.Tools)
	wName, wSpec, wDecl, wInst := 4, 6, 8, 9
	type row struct {
		name, source, declared, installed, state string
	}
	var rows []row
	okCount := 0
	for _, n := range names {
		raw := m.Tools[n]
		sp, err := parseSpec(n, raw)
		if err != nil {
			rows = append(rows, row{bold(n), raw, yellow("?"), dim("-"), red("✗ invalid spec")})
			wName = max(wName, len(n))
			wSpec = max(wSpec, len(raw))
			continue
		}
		plainSrc := sp.Backend + ":" + sp.Package
		colSrc := formatSource(sp.Backend, sp.Package)
		decl := sp.Version
		if decl == "" {
			decl = "any"
		}
		instRaw := snap.installedVersion(sp)
		var colInst, status string
		if instRaw != "" {
			if sp.Version == "" || versionsMatch(instRaw, sp.Version) {
				colInst = green(instRaw)
				if svc, hasSvc := snap.brewService(sp); hasSvc {
					switch svc {
					case "started", "scheduled":
						okCount++
						status = green("● ok") + " " + cyan("(running)")
					case "error":
						status = red("✗ service error") + " " + dim("(run `gear install --force "+n+"`)")
					default:
						status = yellow("⚠ service stopped") + " " + dim("(run `gear install "+n+"`)")
					}
				} else {
					okCount++
					status = green("● ok")
				}
			} else {
				colInst = yellow(instRaw)
				status = yellow(fmt.Sprintf("↻ drift (%s → %s)", instRaw, sp.Version)) + " " + dim("run `gear install`")
			}
		} else {
			instRaw = "-"
			colInst = dim("-")
			status = dim("○ not installed")
		}
		wName = max(wName, len(n))
		wSpec = max(wSpec, len(plainSrc))
		wDecl = max(wDecl, len(decl))
		wInst = max(wInst, len(instRaw))
		rows = append(rows, row{bold(n), colSrc, dim(decl), colInst, status})
	}

	totalWidth := wName + wSpec + wDecl + wInst + 24
	fmt.Printf("%s %s\n", bold(cyan("⚙  GEAR STATUS")), dim(fmt.Sprintf("(%d/%d ready)", okCount, len(rows))))
	fmt.Printf("  %s\n", dim(strings.Repeat("─", totalWidth)))
	fmt.Printf("  %s  %s  %s  %s  %s\n",
		bold(pad("TOOL", wName)),
		bold(pad("SOURCE", wSpec)),
		bold(pad("DECLARED", wDecl)),
		bold(pad("INSTALLED", wInst)),
		bold("STATUS"),
	)
	fmt.Printf("  %s\n", dim(strings.Repeat("─", totalWidth)))
	for _, r := range rows {
		fmt.Printf("  %s  %s  %s  %s  %s\n",
			pad(r.name, wName),
			pad(r.source, wSpec),
			pad(r.declared, wDecl),
			pad(r.installed, wInst),
			r.state,
		)
	}
	return 0
}

func cmdAdd(args []string) int {
	if len(args) < 1 || len(args) > 2 {
		return fail(fmt.Errorf("usage: gear add [name] <brew:pkg[@ver] | npm:pkg[@ver] | uv:pkg[@ver]>"))
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
	fmt.Printf("%s saved %s = %s in %s\n", green("+"), bold(name), cyan(fmt.Sprintf("%q", sp.Format())), dim(tilde(manifestPath())))
	return cmdInstall([]string{name})
}

func cmdRemove(args []string) int {
	uninstall := false
	var names []string
	for _, a := range args {
		if a == "--uninstall" || a == "-u" {
			uninstall = true
		} else {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		return fail(fmt.Errorf("usage: gear rm [--uninstall] <name...>"))
	}
	m, err := loadManifest()
	if err != nil {
		return fail(err)
	}
	for _, name := range names {
		raw, had := m.Tools[name]
		delete(m.Tools, name)
		if uninstall && had {
			if sp, err := parseSpec(name, raw); err == nil {
				if err := uninstallSpec(sp); err != nil {
					fmt.Fprintf(os.Stderr, "  ! uninstall %s: %v\n", name, err)
				}
			}
		}
		fmt.Printf("%s removed %s\n", yellow("-"), bold(name))
	}
	if err := saveManifest(m); err != nil {
		return fail(err)
	}
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
		if sp.Version != "" && versionsMatch(sp.Version, latest) {
			fmt.Printf("  %s %-18s %s %s\n", green("="), bold(name), green(sp.Version), dim("(up to date)"))
			continue
		}
		fmt.Printf("  %s %-18s %s -> %s\n", cyan("↑"), bold(name), yellow(sp.Version), green(latest))
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
	add := exec.Command("git", "-C", dir, "add", "gears.toml")
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
