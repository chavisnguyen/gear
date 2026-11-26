package main

import (
	"path/filepath"
	"testing"
)

func TestParseSpecAndBrewTarget(t *testing.T) {
	cases := []struct {
		name, raw               string
		wantBackend, wantPkg, v string
	}{
		{"agys", "brew:chavisnguyen/agys/agys", "brew", "chavisnguyen/agys/agys", ""},
		{"legacy-tap", "brew:chavisnguyen/agys/agys@v1.0.19", "brew", "chavisnguyen/agys/agys", ""},
		{"fimake", "brew:chavisnguyen/FiMake/fimake@1.0.43", "brew", "chavisnguyen/FiMake/fimake", ""},
		{"agent-desktop", "brew:agent-desktop@1.3.1", "brew", "agent-desktop", ""},
		{"node20", "brew:node@20", "brew", "node@20", ""},
		{"py312", "brew:python@3.12", "brew", "python@3.12", ""},
		{"mobile-mcp", "npm:@mobilenext/mobile-mcp@1.0.20", "npm", "@mobilenext/mobile-mcp", "1.0.20"},
		{"jenkins", "uv:mcp-jenkins@3.6.0", "uv", "mcp-jenkins", "3.6.0"},
	}
	for _, tc := range cases {
		got, err := parseSpec(tc.name, tc.raw)
		if err != nil {
			t.Fatalf("parseSpec(%q) unexpected error: %v", tc.raw, err)
		}
		if got.Backend != tc.wantBackend || got.Package != tc.wantPkg || got.Version != tc.v {
			t.Fatalf("parseSpec(%q) = %+v", tc.raw, got)
		}
	}

	bt, err := parseBrewTarget("chavisnguyen/FiMake/fimake")
	if err != nil {
		t.Fatal(err)
	}
	if bt.TapName != "chavisnguyen/fimake" || bt.TapURL != "https://github.com/chavisnguyen/FiMake" || bt.FormulaRef != "chavisnguyen/fimake/fimake" || bt.ShortName != "fimake" {
		t.Fatalf("unexpected BrewTarget: %+v", bt)
	}

	core, err := parseBrewTarget("agent-desktop")
	if err != nil || core.TapName != "" || core.FormulaRef != "agent-desktop" {
		t.Fatalf("unexpected core BrewTarget: %+v (%v)", core, err)
	}
}

func TestVersionParsers(t *testing.T) {
	brewOut := "agys 1.0.18 1.0.19\nagent-desktop 1.3.1_1\nklik 3.2.0\n"
	bv := parseBrewVersions(brewOut)
	if bv["agys"] != "1.0.19" || !versionsMatch(bv["agent-desktop"], "1.3.1") || bv["klik"] != "3.2.0" {
		t.Fatalf("parseBrewVersions = %+v", bv)
	}

	npmJSON := []byte(`{"dependencies":{"@mobilenext/mobile-mcp":{"version":"1.0.20"}}}`)
	nv := parseNPMListJSON(npmJSON)
	if nv["@mobilenext/mobile-mcp"] != "1.0.20" {
		t.Fatalf("parseNPMListJSON = %+v", nv)
	}

	uvOut := "gdown v6.4.0\n- gdown\nmcp-jenkins v3.6.0\n- mcp-jenkins\n"
	uv := parseUVToolList(uvOut)
	if uv["mcp-jenkins"] != "3.6.0" || uv["gdown"] != "6.4.0" {
		t.Fatalf("parseUVToolList = %+v", uv)
	}

	svcOut := "Name      Status  User   File\nfimake    started chavis ~/Library/LaunchAgents/sh.brew.fimake.plist\nwame      none\nklik      error   chavis ~/Library/LaunchAgents/sh.brew.klik.plist\n"
	sv := parseBrewServices(svcOut)
	if sv["fimake"] != "started" || sv["wame"] != "none" || sv["klik"] != "error" || len(sv) != 3 {
		t.Fatalf("parseBrewServices = %+v", sv)
	}
}

func TestInitAndSaveManifest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GEAR_HOME", tmp)
	if code := run([]string{"init"}); code != 0 {
		t.Fatalf("gear init failed: %d", code)
	}
	m := &Manifest{Tools: map[string]string{
		"klik": "brew:chavisnguyen/klik/klik",
		"agys": "brew:chavisnguyen/agys/agys",
	}}
	if err := saveManifest(m); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadManifest()
	if err != nil || len(loaded.Tools) != 2 || loaded.Tools["klik"] != "brew:chavisnguyen/klik/klik" {
		t.Fatalf("loaded = %+v, err = %v (from %s)", loaded, err, filepath.Join(tmp, "config/gears.toml"))
	}
}
