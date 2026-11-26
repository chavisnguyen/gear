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
		svc, hasSvc := snap.brewService(sp)
		svcHealthy := !hasSvc || svc == "started" || svc == "scheduled"

		if !force && instVer != "" && (sp.Version == "" || versionsMatch(instVer, sp.Version)) {
			if svcHealthy {
				if hasSvc {
					fmt.Printf("  %s %s  %s %s\n", green("="), pad(bold(name), 18), green(instVer), cyan("("+sp.Backend+", running)"))
				} else {
					fmt.Printf("  %s %s  %s %s\n", green("="), pad(bold(name), 18), green(instVer), dim("("+sp.Backend+")"))
				}
				continue
			}
			// Version matches, but brew service is stopped or in error -> start/restart it!
			bt, _ := parseBrewTarget(sp.Package)
			svcVerb := "start"
			if svc == "error" {
				svcVerb = "restart"
			}
			fmt.Printf("  %s %s  starting background service (%s)...\n", cyan("▶"), pad(bold(name), 18), dim(bt.FormulaRef))
			if err := runBrew("services", svcVerb, bt.FormulaRef); err != nil {
				fmt.Fprintf(os.Stderr, "  %s %s  service %s FAILED: %v\n", red("✗"), pad(bold(name), 18), svcVerb, err)
				failures++
				continue
			}
			fmt.Printf("  %s %s  %s %s\n", green("+"), pad(bold(name), 18), green(instVer), cyan("(running)"))
			continue
		}

		action := "installing"
		if instVer != "" {
			action = fmt.Sprintf("updating %s →", yellow(instVer))
		}
		targetLabel := sp.Version
		if targetLabel == "" {
			targetLabel = "latest"
		}
		fmt.Printf("  %s %s  %s %s (%s)...\n", cyan("↓"), pad(bold(name), 18), action, green(targetLabel), formatSource(sp.Backend, sp.Package))

		if err := applySpec(sp, instVer != "", force, snap); err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s  %s %v\n", red("✗"), pad(bold(name), 18), red("FAILED:"), err)
			failures++
			continue
		}
		fmt.Printf("  %s %s  %s\n", green("+"), pad(bold(name), 18), green("ok ("+sp.Backend+")"))
	}

	if manifestDirty {
		_ = saveManifest(m)
	}
	if failures > 0 {
		return 1
	}
	return 0
}
