<p align="center"><img src="web/icon-192.png" width="96" alt="Hiveloop"></p>

<h1 align="center">Hiveloop</h1>

<p align="center">A small, self-hosted space where a group of people organise themselves:<br>
shared calendar with attendance, group chats, documents and an optional AI assistant.</p>

---

Hiveloop is built for associations, volunteer groups, clubs and small teams. It is a single
binary (Go + SQLite, frontend embedded) that runs happily on an old PC with 1 GB of RAM, and it
is designed to be simple enough for people who are not used to apps.

## Features

- **Calendar** with month and agenda views, filters per group, detailed programmes written in a
  visual editor, and **attendance**: people mark when they will be there (whole event or a time slot).
- **Volunteer shifts**: an event can say how many people it needs and when (e.g. 10:00–13:00, 2
  volunteers); anyone can sign up, and the group is reminded 3 days, 1 day and 3 hours before a
  shift that is still short.
- **Private group events** for internal organisation, visible only to the group (and admins).
- **Public links** for an event or a whole day, readable without an account, with "add to calendar" (.ics).
- **Groups**, each with its own chat, documents, cover photo and an elected **group leader**
  (majority vote in the group chat) who can manage members and the group's documents.
- **Private groups** (created by admins): invite only and hidden from non-members; people join
  when an admin or the group leader adds them. Their events are private by default and are shown
  to everyone only when chosen event by event.
- **Chats** for every group plus a general one, and **private messages** between two people:
  @mentions, replies (swipe left on a phone), live updates.
- **Notifications** even when the app is closed: Web Push (standard library only, VAPID keys are
  generated on first use) and a periodic check in the Android app, no Google services needed.
- **Documents**: private space per person, sharing with groups or everyone, previews (PDF, images,
  video, text), a visual editor for Markdown documents and **version history** (last 10).
- **AI assistant** (optional): works with the user's own permissions; it can read documents, write
  and update Markdown documents, manage events and attendance, share links and search the web.
  Also available in chats with `@ai`. The super admin can turn it off, limit it to certain hours
  and days, and set daily message limits per person, per group and in total.
- **Personalisation per user**: light/dark theme, accent colour, icons-only or text-only buttons,
  zoom (80–175%), a high-readability mode, and a cartoon avatar editor.
- **Roles**: user, admin, super admin. Sign-up can be closed, open, or subject to approval.
- **Installable**: works as a web app on phones (iOS Web Clip profile included) and has a minimal
  Android app (WebView) you can build yourself.
- **Bilingual interface**: English (default) and Italian, with a built-in guide for every role.
- **Encrypted backups** built into the binary (age).

## Quick start

You need [Go](https://go.dev/dl/) 1.26 or newer.

```sh
git clone https://github.com/askscience/hiveloop
cd hiveloop
go build -o hiveloop .
HIVELOOP_SECURE_COOKIE=0 ./hiveloop
```

Open <http://127.0.0.1:8090>. On first start a **super admin** is created with a random password,
saved in `data/initial-credentials.txt` (readable only by the server's user). Sign in, change the
password from *Profile* and delete the file. Set `HIVELOOP_ADMIN_PASS` to choose it yourself.

`HIVELOOP_SECURE_COOKIE=0` is only for plain-HTTP testing on your own computer. In production
always serve Hiveloop over HTTPS (see below) and leave it at the default.

### With Docker

The image expects HTTPS in front of it (see *Running in production*); for a quick local try, add
`-e HIVELOOP_SECURE_COOKIE=0`.

```sh
docker build -t hiveloop .
docker run -d --name hiveloop -p 127.0.0.1:8090:8090 -v hiveloop-data:/data hiveloop
docker exec hiveloop cat /data/initial-credentials.txt   # initial super admin password
```

## Configuration

Everything is configured with environment variables. `.env.example` lists them all; Hiveloop does
not read that file itself, so load it with your service manager (`EnvironmentFile=` in systemd,
`--env-file` in Docker) or your shell.

| Variable | Default | Meaning |
|---|---|---|
| `HIVELOOP_ADDR` | `127.0.0.1:8090` | address and port to listen on |
| `HIVELOOP_DATA` | `./data` | folder for the database and uploaded files |
| `HIVELOOP_PUBLIC_URL` | (from the request) | public address, e.g. `https://hiveloop.example.org/` (used in shared links and app downloads) |
| `HIVELOOP_SECURE_COOKIE` | `1` | `1` = cookies only over HTTPS; set `0` only for local HTTP testing |
| `HIVELOOP_TZ` | `UTC` | time zone for days, the calendar and assistant schedules, e.g. `Europe/Rome` |
| `HIVELOOP_ADMIN_USER` | `superadmin` | username of the first super admin |
| `HIVELOOP_ADMIN_PASS` | (random) | password of the first super admin (only used when it is created) |
| `HIVELOOP_AI_PROVIDER` | `ollama` | `ollama`, or `openai` for any OpenAI-compatible API |
| `HIVELOOP_AI_URL` | `http://127.0.0.1:11434` | AI endpoint, e.g. `https://api.openai.com/v1`, `https://api.deepseek.com` |
| `HIVELOOP_MODEL` | `llama3.1` | model name |
| `HIVELOOP_AI_KEY` | | API key for the AI provider |
| `HIVELOOP_BRAVE_KEY` | | [Brave Search](https://brave.com/search/api/) key for the assistant's web search (can also be set by the super admin in *Manage*) |

Storage per user, document creation and uploads, and the assistant's limits are set by the super
admin in *Manage*.

### AI assistant

The assistant is optional: without a reachable AI endpoint the rest of Hiveloop works normally,
and the super admin can hide it completely.

The super admin can also configure it from *Manage → AI model*: provider (Ollama or any
OpenAI-compatible API), address, model and API key, with a **Test** button that checks the settings
and lists the available models before saving. Values saved there override the environment
variables, which act as the starting point. Environment examples:

```sh
# local, free: Ollama
HIVELOOP_AI_PROVIDER=ollama HIVELOOP_AI_URL=http://127.0.0.1:11434 HIVELOOP_MODEL=llama3.1

# hosted, OpenAI-compatible
HIVELOOP_AI_PROVIDER=openai HIVELOOP_AI_URL=https://api.openai.com/v1 HIVELOOP_MODEL=gpt-4o-mini HIVELOOP_AI_KEY=...
```

Models with good tool calling give the best results. Web pages the assistant reads are treated as
untrusted data, and it cannot reach private network addresses.

## Running in production

Hiveloop listens on plain HTTP; put it behind something that provides HTTPS:

- a reverse proxy such as [Caddy](https://caddyserver.com) (automatic certificates):
  ```
  hiveloop.example.org {
      reverse_proxy 127.0.0.1:8090
  }
  ```
- or [Tailscale Funnel](https://tailscale.com/kb/1223/funnel) if the machine has no public address:
  `tailscale funnel --bg 8090`.

Hiveloop trusts the `X-Forwarded-For` header only from the local machine, so keep the proxy on
the same host. A systemd unit example is in `docs/hiveloop.service`.

### Backups

```sh
./hiveloop backup-keygen > backup-key.txt         # keep this file OFF the server
./hiveloop backup -data ./data -r age1... -o hiveloop-$(date +%F).tar.gz.age
./hiveloop restore -i backup-key.txt -o ./restored hiveloop-2026-01-01.tar.gz.age
```

The snapshot is consistent while Hiveloop runs and contains the database and all uploaded files.

### Phones

- **Android**: build the app with GitHub Actions, see `android/README.md`.
- **iPhone/iPad**: the sign-up and profile pages offer a profile that adds Hiveloop to the Home
  Screen, or use *Add to Home Screen* in Safari.

## Development

```sh
go vet ./... && go test ./...
HIVELOOP_SECURE_COOKIE=0 go run .
```

The frontend is plain JavaScript modules in `web/` with no build step; it is embedded in the
binary at compile time. `tools/loadtest` contains a load test that simulates many users.

See `CONTRIBUTING.md` for how to contribute and `SECURITY.md` to report vulnerabilities.

## License

Hiveloop is free software under the [GNU Affero General Public License v3.0](LICENSE): you can use,
study, change and share it; if you run a modified version for others over a network, you must
offer them its source code. Bundled third-party components are listed in `THIRD_PARTY.md`.
