package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func inPath(dir string) bool {
	clean := filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == clean {
			return true
		}
	}
	return false
}

func printPathHint() {
	if !inPath(binDir()) {
		fmt.Printf("\n  note: %s is not in your $PATH yet. Add to ~/.zshrc:\n    export PATH=\"%s:$PATH\"\n", tilde(binDir()), tilde(binDir()))
	}
}

func cmdDoctor() int {
	issues := 0
	fmt.Printf("gear doctor (v%s, %s)\n\n", Version, platformKey())

	// 1. Check PATH
	if inPath(binDir()) {
		fmt.Printf("  🟢 PATH        %s is in $PATH\n", tilde(binDir()))
	} else {
		issues++
		fmt.Printf("  🟡 PATH        %s is NOT in $PATH\n", tilde(binDir()))
		fmt.Printf("                 fix: echo 'export PATH=\"$HOME/.gear/bin:$PATH\"' >> ~/.zshrc\n")
	}

	// 2. Check config
	p := manifestPath()
	if !exists(p) {
		issues++
		fmt.Printf("  🔴 CONFIG      missing %s (run `gear init`)\n", tilde(p))
		return 1
	}
	m, err := loadManifest()
	if err != nil {
		issues++
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

	// 3. Check required package managers for declared backends
	needNPM, needUV := false, false
	for _, raw := range m.Tools {
		if strings.HasPrefix(raw, "npm:") {
			needNPM = true
		}
		if strings.HasPrefix(raw, "uv:") {
			needUV = true
		}
	}
	if needNPM {
		if _, err := exec.LookPath("npm"); err != nil {
			issues++
			fmt.Printf("  🔴 NPM         npm not found in $PATH (needed by npm: tools)\n")
		} else {
			fmt.Printf("  🟢 NPM         available\n")
		}
	}
	if needUV {
		if _, err := exec.LookPath("uv"); err != nil {
			issues++
			fmt.Printf("  🔴 UV          uv not found in $PATH (needed by uv: tools)\n")
		} else {
			fmt.Printf("  🟢 UV          available\n")
		}
	}

	// 4. Check broken symlinks in ~/.gear/bin
	if entries, err := os.ReadDir(binDir()); err == nil {
		for _, e := range entries {
			full := filepath.Join(binDir(), e.Name())
			if _, err := os.Stat(full); err != nil {
				issues++
				fmt.Printf("  🔴 BIN         broken symlink: %s\n", tilde(full))
			}
		}
	}

	// 5. Check drift for each declared tool
	st := loadState()
	driftCount := 0
	for _, name := range sortedKeys(m.Tools) {
		sp, err := parseSpec(name, m.Tools[name])
		if err != nil {
			issues++
			driftCount++
			fmt.Printf("  🔴 TOOL        %s: %v\n", name, err)
			continue
		}
		inst, ok := st.Installed[name]
		if !ok || !binariesExist(inst.Bins) || normalizeVer(inst.Version) != normalizeVer(sp.Version) || inst.Backend != sp.Backend || inst.Package != sp.Package {
			issues++
			driftCount++
			fmt.Printf("  🟡 TOOL        %s needs sync (declared %s, installed %s) -> run `gear install`\n", name, sp.Version, inst.Version)
		}
	}
	if driftCount == 0 && len(m.Tools) > 0 {
		fmt.Printf("  🟢 TOOLS       all %d tool(s) installed at exact pinned versions\n", len(m.Tools))
	}

	if issues > 0 {
		return 1
	}
	return 0
}
