package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdDoctor() int {
	issues := 0
	fmt.Printf("%s %s\n\n", bold(cyan("🩺 gear doctor")), dim(fmt.Sprintf("(v%s, %s)", Version, platformKey())))

	// 1. Check config
	p := manifestPath()
	if !exists(p) {
		fmt.Printf("  %s %-11s missing %s (run `gear init`)\n", red("✗"), bold("CONFIG"), tilde(p))
		return 1
	}
	m, err := loadManifest()
	if err != nil {
		fmt.Printf("  %s %-11s %v\n", red("✗"), bold("CONFIG"), err)
		return 1
	}
	fmt.Printf("  %s %-11s %s %s\n", green("✓"), bold("CONFIG"), tilde(p), dim(fmt.Sprintf("(%d tools)", len(m.Tools))))

	if exists(filepath.Join(configDir(), ".git")) {
		out, _ := exec.Command("git", "-C", configDir(), "status", "--porcelain").Output()
		if strings.TrimSpace(string(out)) == "" {
			fmt.Printf("  %s %-11s %s %s\n", green("✓"), bold("GIT SYNC"), tilde(configDir()), green("clean"))
		} else {
			fmt.Printf("  %s %-11s %s %s\n", yellow("!"), bold("GIT SYNC"), tilde(configDir()), yellow("uncommitted changes (run `gear push`)"))
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
			fmt.Printf("  %s %-11s %s\n", red("✗"), bold(mgr.name), red(mgr.bin+" not found in $PATH"))
		} else {
			fmt.Printf("  %s %-11s %s\n", green("✓"), bold(mgr.name), dim(path))
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
			fmt.Printf("  %s %-11s %s: %v\n", red("✗"), bold("TOOL"), bold(name), err)
			continue
		}
		inst := snap.installedVersion(sp)
		if inst == "" {
			issues++
			driftCount++
			fmt.Printf("  %s %-11s %s is not installed %s\n", yellow("!"), bold("TOOL"), bold(name), dim("-> run `gear install "+name+"`"))
		} else if sp.Version != "" && !versionsMatch(inst, sp.Version) {
			issues++
			driftCount++
			fmt.Printf("  %s %-11s %s drift (%s != %s) %s\n", yellow("!"), bold("TOOL"), bold(name), yellow(inst), green(sp.Version), dim("-> run `gear install "+name+"`"))
		} else if svc, hasSvc := snap.brewService(sp); hasSvc && svc != "started" && svc != "scheduled" {
			issues++
			driftCount++
			fmt.Printf("  %s %-11s %s (%s) service is %s %s\n", yellow("!"), bold("SERVICE"), bold(name), inst, yellow(svc), dim("-> run `gear install "+name+"`"))
		}
	}
	if driftCount == 0 && len(m.Tools) > 0 {
		fmt.Printf("  %s %-11s %s\n", green("✓"), bold("TOOLS"), green(fmt.Sprintf("all %d tool(s) installed & background services running", len(m.Tools))))
	}

	if issues > 0 {
		return 1
	}
	return 0
}
