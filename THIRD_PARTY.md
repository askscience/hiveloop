# Third-party components

Hiveloop is licensed under the GNU AGPL-3.0 (see `LICENSE`). It includes or depends on the
following components, each under its own license.

## Bundled in `web/` (served to the browser)

| Component | Use | License |
|---|---|---|
| [Editor.js](https://editorjs.io) and its Header, List, Quote, Delimiter, Table and Inline Code tools | visual Markdown editor | Apache-2.0 |
| [PDF.js](https://mozilla.github.io/pdf.js/) 4.10 | PDF previews | Apache-2.0 |
| [marked](https://marked.js.org) 15 | Markdown rendering | MIT |
| [DOMPurify](https://github.com/cure53/DOMPurify) 3 | HTML sanitising | Apache-2.0 or MPL-2.0 |
| [DiceBear](https://www.dicebear.com) core (bundled in `vendor/avataaars.mjs`) | avatar rendering | MIT |
| [Avataaars](https://avataaars.com) design by Pablo Stanley (DiceBear `avataaars` style) | avatar artwork | free for personal and commercial use |
| [Roboto](https://fonts.google.com/specimen/Roboto) | interface font | SIL Open Font License 1.1 |
| [Instrument Serif](https://fonts.google.com/specimen/Instrument+Serif) | logo font | SIL Open Font License 1.1 |

The avatar licence texts are in `web/vendor/avataaars-license/`.

## Go modules (compiled into the server)

See `go.mod` for exact versions. The main ones:

| Module | Use | License |
|---|---|---|
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | SQLite in pure Go | BSD-3-Clause |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | bcrypt password hashing | BSD-3-Clause |
| [golang.org/x/net](https://pkg.go.dev/golang.org/x/net) | HTML parsing for web search | BSD-3-Clause |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) | image resizing, WebP decoding | BSD-3-Clause |
| [filippo.io/age](https://age-encryption.org) | encrypted backups | BSD-3-Clause |
| [github.com/ledongthuc/pdf](https://github.com/ledongthuc/pdf) | reading text from PDFs (for the assistant) | BSD-3-Clause |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | username normalisation | BSD-3-Clause |
