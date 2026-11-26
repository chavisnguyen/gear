package main

import (
	"flag"
	"fmt"
	"os"
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
	if len(targets) == 0 {
		targets = sortedKeys(m.Tools)
	}
	if len(targets) == 0 {
		fmt.Println("Nothing to install.")
		return 0
	}

	snap := probeSystem(m.Tools)
	failures := 0
	manifestDirty := false

	for _, name := range targets {
		raw, ok := m.Tools[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "  ! %-18s not defined in %s\n", name, tilde(manifestPath()))
			failures++
			continue
		}
		sp, err := parseSpec(name, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %-18s %v\n", name, err)
			failures++
			continue
		}
		if sp.Version == "latest" {
			v, err := resolveLatestVersion(sp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ! %-18s resolve version: %v\n", name, err)
				failures++
				continue
			}
			sp.Version = normalizeVer(v)
			m.Tools[name] = sp.Format()
			manifestDirty = true
		}

		instVer := snap.installedVersion(sp)
		if !force && instVer != "" && (sp.Version == "" || versionsMatch(instVer, sp.Version)) {
			fmt.Printf("  = %-18s %s (%s)\n", name, instVer, sp.Backend)
			continue
		}

		action := "installing"
		if instVer != "" {
			action = fmt.Sprintf("updating %s ->", instVer)
		}
		targetLabel := sp.Version
		if targetLabel == "" {
			targetLabel = "latest"
		}
		fmt.Printf("  ↓ %-18s %s %s (%s:%s)...\n", name, action, targetLabel, sp.Backend, sp.Package)

		if err := applySpec(sp, instVer != "", force, snap); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %-18s FAILED: %v\n", name, err)
			failures++
			continue
		}
		fmt.Printf("  + %-18s ok (%s)\n", name, sp.Backend)
	}

	if manifestDirty {
		_ = saveManifest(m)
	}
	if failures > 0 {
		return 1
	}
	return 0
}
