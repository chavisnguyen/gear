package main

import (
	"os"
	"regexp"
	"strings"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func paint(code, s string) string {
	if !colorEnabled() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string    { return paint("1", s) }
func dim(s string) string     { return paint("2", s) }
func green(s string) string   { return paint("92", s) }
func yellow(s string) string  { return paint("93", s) }
func red(s string) string     { return paint("91", s) }
func cyan(s string) string    { return paint("96", s) }
func magenta(s string) string { return paint("95", s) }

func visibleLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

func pad(s string, width int) string {
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}

func formatSource(backend, pkg string) string {
	switch backend {
	case "brew":
		return cyan("brew") + dim(":") + pkg
	case "npm":
		return yellow("npm") + dim(":") + pkg
	case "uv":
		return magenta("uv") + dim(":") + pkg
	default:
		return backend + ":" + pkg
	}
}
