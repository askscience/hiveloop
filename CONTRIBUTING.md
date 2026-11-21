# Contributing

Thanks for wanting to help! Bug reports, translations, documentation and code are all welcome.

## Before you start

- For bugs, open an issue with steps to reproduce and what you expected.
- For bigger changes, open an issue first to discuss the idea.
- Security problems: see `SECURITY.md` (do not open a public issue).

## Development

```sh
go vet ./... && go test ./...
HIVELOOP_SECURE_COOKIE=0 go run .
```

- **Backend**: Go standard library (`net/http`) and pure-Go SQLite; no CGO. Keep it light: Hiveloop
  must keep running well on a 1 GB machine.
- **Frontend**: plain JavaScript modules in `web/`, no build step and no external CDNs. Text shown
  to people goes through `web/i18n.js` (English and Italian); the built-in guide is `web/help.js`.
- **Database changes**: add them as additive migrations in `db.go` (never break existing data).
- Write code, comments and commit messages in English.

## Pull requests

- Keep each pull request focused on one change and describe what and why.
- Make sure `go vet ./...` and `go test ./...` pass, and try the change in a browser, on a phone
  width as well.
- By contributing you agree that your contribution is licensed under the AGPL-3.0.

## Translations

Add a language by copying the `en` block in `web/i18n.js` and the `en` texts in `web/help.js`,
then adding it to the language switch in `web/app.js`.
