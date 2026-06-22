# CoCA Skill Terminal (SSH)

`ssh` into a cyberhacker-style terminal, pick one of the ~106
[Church of Conceptual Art](https://whatisthe.churchofconceptualart.org) skills, and that
skill's `SKILL.md` becomes the system prompt for a chat. It is a **generic model wearing the
skill** — no CoCA persona, no doctrine framing — and it is **not an agent**: it never runs
tools, it just adopts the skill text and answers.

Same mechanic as `ssh terminal.shop` (Go + [Charm](https://charm.sh)'s Wish/Bubble Tea), but the
feel is a hacker session, not a shop.

```
ssh oracle.churchofconceptualart.org        # once deployed
```

## How it works

- **SSH app** — [Wish](https://github.com/charmbracelet/wish) runs a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  TUI in place of a shell. Any key is accepted (anonymous; the session is the identity).
- **Skills** — at startup it fetches the index `llms.txt` and each `{name}/SKILL.md` from the
  public repo [`previousdolphin/coca-skills`](https://github.com/previousdolphin/coca-skills)
  (configurable via `SKILLS_RAW_BASE`), cached in memory, refreshed every 30 min. Names are
  allowlisted against the parsed index — no arbitrary fetches.
- **Chat** — the selected skill (a neutral glue line + the verbatim `SKILL.md`) is sent as the
  Anthropic Messages API `system` block. Call shape mirrors the site's `functions/api/oracle.ts`.

## Files

| file | role |
|---|---|
| `main.go` | Wish SSH server, host key, anonymous auth, periodic skill refresh |
| `skills.go` | fetch + parse `llms.txt`, cache `SKILL.md`, build the skill-only system prompt |
| `oracle.go` | Anthropic Messages client (cached system block, `max_tokens` 800, refusal handling) |
| `tui.go` | Bubble Tea model: connect banner → category menu → skill list → chat |
| `ratelimit.go` | in-memory per-IP burst + daily limiter |

## Run locally

```bash
export ANTHROPIC_API_KEY=sk-ant-...        # required for chat replies
export ORACLE_MODEL=claude-haiku-4-5       # optional (this is the default)
go run .                                    # listens on :23234
```

In another terminal:

```bash
ssh -p 23234 localhost
```

> A real terminal is required (the app needs an interactive PTY). Piping stdin gives a 0×0
> window and renders blank — connect from an actual terminal.

### Tests

```bash
go test ./...                               # offline: parsing, menu flow, chat render, limiter
SMOKE=1 ANTHROPIC_API_KEY=sk-ant-... go test -run TestSmoke -v   # live: GitHub fetch + real model
```

## Environment

| var | default | notes |
|---|---|---|
| `ANTHROPIC_API_KEY` | — | required for replies |
| `ORACLE_MODEL` | `claude-haiku-4-5` | any Claude model id |
| `PORT` | `23234` | listen port |
| `HOST` | `0.0.0.0` | listen host |
| `SKILLS_RAW_BASE` | the repo's `main` raw URL | point at a fork/branch |
| `SSH_HOST_KEY` | — | base64 of an ed25519 PEM; stable identity across restarts |
| `HOST_KEY_PATH` | `.ssh/coca_oracle_ed25519` | used only when `SSH_HOST_KEY` is unset (Wish generates one) |

## Deploy (Fly.io)

Cloudflare can't bind raw port 22, so this runs on a small always-on Fly machine.

```bash
fly launch --no-deploy            # or: fly apps create coca-oracle-ssh
fly ips allocate-v4               # dedicated IPv4 (also -v6 if wanted)

# Stable host key so returning users don't get "host key changed":
ssh-keygen -t ed25519 -N "" -f ./hostkey
fly secrets set \
  ANTHROPIC_API_KEY=sk-ant-... \
  SSH_HOST_KEY="$(base64 < ./hostkey)" \
  ORACLE_MODEL=claude-haiku-4-5
rm ./hostkey ./hostkey.pub

fly deploy
```

Then point DNS `oracle.churchofconceptualart.org` (A/AAAA) at the Fly IPs from `fly ips list`,
and users connect with `ssh oracle.churchofconceptualart.org` (port 22, no `-p`).

## Modes

After the banner you pick a **channel** (mirrors the website Oracle):

- **Oracle · doctrine** — the Church's explanatory voice (no skill).
- **Oracle · after-voice** — the Oracle answering from doctrine, filtered through an `after-*`
  thinker's voice (you pick the thinker).
- **Open channel · skill** — a generic model wearing any one skill, no CoCA persona.

## Keys / controls

- **Banner** → any key to continue.
- **Channel** → `↑/↓` move, `⏎` select. Doctrine jumps straight to chat; the others open a picker.
- **Categories / Skills** → `↑/↓` move, `/` filter, `⏎` load, `esc` back, `q` disconnect.
- **Chat** → `⏎` send, `esc` back (resets the conversation), `ctrl+c` disconnect.

## Not in v1

- Streaming replies (token-by-token).
- Persistence / identity (hash the SSH key → save transcripts to the site Chronicle).
- Selecting distillations in addition to skills.
