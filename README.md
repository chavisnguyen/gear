# gear

**Declare every CLI tool you use across machines in one tiny TOML file — installed natively via `brew`, `npm`, and `uv`.**

No custom `bin` directories, no `$PATH` edits, no duplicate downloads. `gear` keeps a clean manifest in `~/.gear/config/gears.toml` (synced via Git) and delegates installation & upgrades straight to your native package managers.

---

## Install

```bash
brew tap chavisnguyen/gear https://github.com/chavisnguyen/gear
brew install gear
```

---

## Quick start

### 1. Create or clone your config

```bash
gear init                                              # local machine
gear init git@github.com:you/gear-config.git           # or sync from a private Git repo
```

### 2. Declare your tools (`~/.gear/config/gears.toml`)

```toml
[tools]
mcps          = "brew:chavisnguyen/mcps/mcps"
agys          = "brew:chavisnguyen/agys/agys"
klik          = "brew:chavisnguyen/klik/klik"
wame          = "brew:chavisnguyen/wame/wame"
agent-desktop = "brew:agent-desktop"
mobile-mcp    = "npm:@mobilenext/mobile-mcp@1.0.20"
mcp-jenkins   = "uv:mcp-jenkins@3.6.0"
```

Or add from the terminal (`brew:` versions are managed natively by Homebrew; `npm:` and `uv:` pin the latest version automatically if `@version` is omitted):

```bash
gear add brew:chavisnguyen/agys/agys
gear add brew:agent-desktop
```

### 3. Sync any machine in one command

```bash
gear install
```

---

## Backends

| Prefix | Example | What `gear` runs |
| :--- | :--- | :--- |
| `brew:` | `brew:chavisnguyen/klik/klik`<br>`brew:agent-desktop` | Auto-taps `https://github.com/<owner>/<repo>` if needed, delegates version management to `brew` (`brew install` / `brew upgrade`), and manages `brew services` |
| `npm:` | `npm:@mobilenext/mobile-mcp@1.0.20` | `npm install -g <pkg>@<version>` |
| `uv:` | `uv:mcp-jenkins@3.6.0` | `uv tool install <pkg>==<version>` |

---

## Commands

| Command | Description |
| :--- | :--- |
| `gear` / `gear status` | Show declared vs installed versions across `brew`, `npm`, and `uv` |
| `gear install [name...]` | Install missing tools and upgrade outdated ones to match `gears.toml` (aliases: `sync`, `up`) |
| `gear add [name] <spec>` | Pin a tool in `gears.toml` and install it immediately |
| `gear rm [--uninstall] <name...>` | Remove tool(s) from `gears.toml` (`-u` to also uninstall from system) |
| `gear upgrade [name...]` | Check upstream releases, bump versions in `gears.toml`, and upgrade |
| `gear init [git-url]` | Initialize `~/.gear/config` or clone/pull from Git |
| `gear pull` / `gear push` | Pull + install, or commit + push `gears.toml` |
| `gear doctor` | Check `brew`/`npm`/`uv`, config git sync, and version alignment |
| `gear version` | Print installed `gear` version |

---

## License

[MIT](LICENSE)
