package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdDoctor() int {
	issues := 0
	fmt.Printf("gear doctor (v%s, %s)\n\n", Version, platformKey())

	// 1. Check config
	p := manifestPath()
	if !exists(p) {
		fmt.Printf("  🔴 CONFIG      missing %s (run `gear init`)\n", tilde(p))
		return 1
	}
	m, err := loadManifest()
	if err != nil {
		fmt.Printf("  🔴 CONFIG      %v\n", err)
		return 1
	}
	fmt.Printf("  🟢 CONFIG      %s (%d tool(s))\n", tilde(p), len(m.Tools))

	if exists(filepath.Join(configDir(), ".git")) {
		out, _ := exec.Command("git", "-C", configDir(), "status", "--porcelain").Output()
		if strings.TrimSpace(string(out)) == "" {
			fmt.Printf("  🟢 GIT SYNC    %s is clean\n", tilde(configDir()))
		} else {
			fmt.Printf("  🟡 GIT SYNC    %s has uncommitted changes (run `gear push`)\n", tilde(configDir()))
		}
	}

	// 2. Check package managers used by manifest
	needBrew, needNPM, needUV := false, false, false
	for _, raw := range m.Tools {
		switch {
		case strings.HasPrefix(raw, "brew:"):
			needBrew = true
		case strings.HasPrefix(raw, "npm:"):
			needNPM = true
		case strings.HasPrefix(raw, "uv:"):
			needUV = true
		}
	}
	for _, mgr := range []struct {
		need bool
		bin  string
		name string
	}{
		{needBrew, "brew", "BREW"},
		{needNPM, "npm", "NPM"},
		{needUV, "uv", "UV"},
	} {
		if !mgr.need {
			continue
		}
		if path, err := exec.LookPath(mgr.bin); err != nil {
			issues++
			fmt.Printf("  🔴 %-11s %s not found in $PATH\n", mgr.name, mgr.bin)
		} else {
			fmt.Printf("  🟢 %-11s %s\n", mgr.name, path)
		}
	}

	// 3. Check declared vs installed versions
	snap := probeSystem(m.Tools)
	driftCount := 0
	for _, name := range sortedKeys(m.Tools) {
		sp, err := parseSpec(name, m.Tools[name])
		if err != nil {
			issues++
			driftCount++
			fmt.Printf("  🔴 TOOL        %s: %v\n", name, err)
			continue
		}
		inst := snap.installedVersion(sp)
		if inst == "" {
			issues++
			driftCount++
			fmt.Printf("  🟡 TOOL        %s is not installed -> run `gear install %s`\n", name, name)
		} else if sp.Version != "" && !versionsMatch(inst, sp.Version) {
			issues++
			driftCount++
			fmt.Printf("  🟡 TOOL        %s drift (installed %s, declared %s) -> run `gear install %s`\n", name, inst, sp.Version, name)
		} else if svc, hasSvc := snap.brewService(sp); hasSvc && svc != "started" && svc != "scheduled" {
			issues++
			driftCount++
			fmt.Printf("  🟡 SERVICE     %s is installed (%s) but service is %s -> run `gear install %s`\n", name, inst, svc, name)
		}
	}
	if driftCount == 0 && len(m.Tools) > 0 {
		fmt.Printf("  🟢 TOOLS       all %d tool(s) installed & background services running\n", len(m.Tools))
	}

	if issues > 0 {
		return 1
	}
	return 0
}
