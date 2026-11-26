# gear

**Pin and sync every CLI tool you use across machines — from one tiny TOML file.**

Inspired by `mise`, stripped down to pure essentials: no runtime shims, no shell hooks, no bloat. Declare your own GitHub Release binaries and third-party `npm` / `uv` CLIs in `~/.gear/config/gears.toml`, lock SHA-256 checksums in `gears.lock`, and link real binaries straight into `~/.gear/bin`.

---

## Install

```bash
brew tap chavisnguyen/gear https://github.com/chavisnguyen/gear
brew install gear
```

No Homebrew?

```bash
curl -fsSL https://raw.githubusercontent.com/chavisnguyen/gear/master/install.sh | bash
```

Ensure `~/.gear/bin` is on your `PATH` (`~/.zshrc` or `~/.bashrc`):

```bash
export PATH="$HOME/.gear/bin:$PATH"
```

---

## Quick start

### 1. Create or clone your config

```bash
gear init                                              # new machine locally
gear init git@github.com:you/gear-config.git           # or sync from a private Git repo
```

### 2. Declare your tools (`~/.gear/config/gears.toml`)

```toml
[tools]
mcps          = "github:chavisnguyen/mcps@0.5.2"
agys          = "github:chavisnguyen/agys@1.0.18"
klik          = "github:chavisnguyen/klik@3.1.0"
wame          = "github:chavisnguyen/wame@2.1.0"
agent-desktop = "npm:agent-desktop@0.2.0"
mcp-jenkins   = "uv:mcp-jenkins@0.2.0"
```

Or add from the terminal (resolves & pins latest version automatically if `@version` is omitted):

```bash
gear add github:chavisnguyen/agys
gear add npm:agent-desktop
```

### 3. Sync any machine in one command

```bash
gear install
```

---

## Backends

| Prefix | Example | How it installs |
| :--- | :--- | :--- |
| `github:` | `github:owner/repo@1.2.0` | Downloads matching OS/Arch release asset (`.tar.gz`, `.zip`, or binary), verifies `SHA-256` against `gears.lock` & `checksums.txt`, links into `~/.gear/bin` (supports private repos via `gh` / `GITHUB_TOKEN`) |
| `npm:` | `npm:agent-desktop@0.2.0` | Installs isolated package under `~/.gear/packages/npm/` and links CLI binaries into `~/.gear/bin` |
| `uv:` | `uv:mcp-jenkins@0.2.0` | Installs isolated Python tool via `uv tool install` under `~/.gear/packages/uv/` and links into `~/.gear/bin` |

---

## Commands

| Command | Description |
| :--- | :--- |
| `gear` / `gear status` | Show declared vs installed versions |
| `gear install [name...]` | Install, upgrade, or downgrade tools to match `gears.toml` (aliases: `sync`, `up`; `--force` to reinstall) |
| `gear add [name] <spec>` | Pin a tool in `gears.toml` and install it immediately |
| `gear rm <name...>` | Remove tool(s) from `gears.toml` and unlink binaries |
| `gear upgrade [name...]` | Check upstream releases, bump versions in `gears.toml`, and install |
| `gear init [git-url]` | Initialize `~/.gear/config` or clone/pull from Git |
| `gear pull` / `gear push` | Pull + install, or commit + push `gears.toml` & `gears.lock` |
| `gear doctor` | Verify `$PATH`, config health, broken links, and version drift |
| `gear version` | Print installed `gear` version |

---

## License

[MIT](LICENSE)
