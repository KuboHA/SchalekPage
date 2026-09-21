# SchalekPage

> **Status:** Early alpha. This repository is a work-in-progress prototype and does not represent the final product.

SchalekPage is a web dashboard that integrates with the EduPage platform to give students a consolidated view of their school life: notifications, timetable, meal plans, substitutions and grades — wrapped in a modern UI with rich attachment previews and threaded conversations.

It is written in **Go**, with no third-party dependencies, and compiles to a single self-contained binary with the templates and stylesheets embedded.

## ✨ Features

- **EduPage authentication** against the current RPC/token login flow, with optional two-factor support (emailed code or device confirmation).
- **Dashboard overview** of recent notifications, grouped threads, confirmation counts, and rich formatting.
- **Attachment handling** with automatic URL normalization, downloadable files, and inline image previews for `.jpg`/`.jpeg` assets.
- **Timetable viewer** with date navigation and automatic next-day rollover after 14:30.
- **Meals module** listing ordered snacks, lunches, and afternoon snacks.
- **Substitution changes** filtered to the logged-in student's class and normalized for friendly display.
- **Grades overview** (term-based) with newest grades first.

## 📁 Project Structure

```
SchalekPage/
├── cmd/schalekpage/     # main package: flags, wiring, graceful shutdown
├── internal/
│   ├── edupage/         # EduPage API client (login, timeline, timetable,
│   │                    # lunches, grades, substitutions)
│   └── web/             # HTTP handlers, session store, view models
│       ├── templates/   # html/template views (embedded)
│       └── static/css/  # stylesheets (embedded)
└── build.sh             # build, test, check and cross-compile
```

The EduPage client is a port of the Python [edupage-api](https://github.com/EdupageAPI/edupage-api) reference implementation.

## ✅ Requirements

- Go 1.26+ (build only — the resulting binary has no runtime dependencies)
- EduPage credentials for a valid student account

No database, cache or external service is required.

## 🛠️ Setup & Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/KuboHA/SchalekPage.git
   cd SchalekPage
   ```

2. **Build**
   ```bash
   ./build.sh
   ```
   This writes a `./schalekpage` binary stamped with the current git version.

### Build script

| Command | Does |
| --- | --- |
| `./build.sh` or `./build.sh build` | compile for the host platform |
| `./build.sh run -addr :8080` | build, then run with the given arguments |
| `./build.sh test` | run the test suite |
| `./build.sh check` | gofmt + `go vet` + tests — what CI should run |
| `./build.sh fmt` | rewrite sources with gofmt |
| `./build.sh dist` | cross-compile static release binaries into `dist/` |
| `./build.sh clean` | remove build output |

`dist` builds Linux, macOS and Windows binaries for amd64 and arm64 with
`CGO_ENABLED=0`, so each one is fully static and needs no libc at runtime.

Plain `go build ./cmd/schalekpage` works too; you only lose the version stamp.

## ▶️ Running the App

```bash
./schalekpage            # listens on :5000
./schalekpage -addr :8080
./schalekpage -version
```

Open the printed URL in your browser and log in with your EduPage credentials.

### Two-Factor Authentication
If your EduPage account uses 2FA, you are redirected to `/two_factor` to supply the verification code.

## 💡 Usage Tips

- **Notifications:** Confirmations ("likes") are counted heuristically. Attachments are grouped under the parent notification, and replies are nested based on explicit parent IDs or ID prefixes.
- **Attachments:**
  - Relative file paths are automatically prefixed with `https://{subdomain}.edupage.org`.
  - `.jpg`/`.jpeg` files open a rich preview with zoom, pan, and download controls.
  - Other files download directly when clicked.
- **Timetable:** After 14:30, the dashboard automatically shows the next day's timetable to help students plan ahead.
- **Substitutions:** Only substitutions relevant to the logged-in student's class are shown. Actions are normalized to lowercase strings (`change`, `add`, `remove`).

## 🔐 Security Notes

- Sessions are **server-side**. The browser only ever holds an opaque, random session id in an `HttpOnly`, `SameSite=Lax` cookie; the EduPage password never leaves the login handler and is never stored. This is a deliberate departure from the previous Flask implementation, which kept the plaintext password in a client-side cookie session and re-authenticated on every request.
- Sessions expire after a period of inactivity and are reaped in the background.
- The session store is in-memory, so restarting the server logs everyone out.

## 🔀 Differences from the Python version

The Flask implementation (`app.py`, plus its Jinja2 templates) has been removed; it remains in git history if you need to compare. Source comments still refer to `app.py` where they explain why a piece of code behaves as it does.

The port is faithful to it and to the [edupage-api](https://github.com/EdupageAPI/edupage-api) reference except where that behaviour was plainly broken. The deliberate divergences:

- **Sessions are server-side** (see Security Notes above) instead of a plaintext password in a client-side cookie.
- **Grades averages** are computed in Go before rendering. The Jinja template referenced `total_avg` before it was ever `{% set %}`, which raises `UndefinedError` on any non-empty grade list.
- **Date navigation** uses `time.AddDate` rather than `date.replace(day=day + i)`, which broke at month boundaries.
- **Inline image previews** flag an attachment as an image from its name *or*, failing that, its URL path extension. The Python checked only the display name, so an unambiguous `/elearning/…/sheet.jpeg` shown as "Worksheet" silently lost its preview. Still scoped to `.jpg`/`.jpeg`, since that is what the preview markup supports.
- **Grade `Verbal`/`Percent`** follow straightforward rules rather than reproducing the reference's `try`/`except` edge cases.
- **Meal `CanBeChanged`** is a parsed boolean rather than an unparsed deadline string; it defaults to `false` when the timestamp cannot be parsed.

The request-compression envelope was verified against the Python implementation: the emitted DEFLATE bytes differ from zlib's (Go makes different Huffman choices) but decompress to a byte-identical payload under `zlib.decompress(raw, -15)`, and the response decode path matches exactly.

**Known fragility, inherited from the reference:** the timetable, lunches, grades and substitution modules scrape JSON out of HTML/JS responses. That is how the Python library works, so the port matches it, but it will break if EduPage changes its markup.

## 🧪 Testing

```bash
./build.sh check     # gofmt + go vet + tests
./build.sh test      # tests only
```

The suite covers the offline logic: the request compression envelope, the login-payload parsing, the defensive JSON helpers, response parsing for each data module, the session store lifecycle, date handling and the 14:30 rollover, and that every template parses.

Routes that talk to EduPage are not covered end-to-end; that needs live credentials.

## 🛣️ Roadmap & Ideas

- Persistent session storage so restarts don't log users out.
- Pagination or lazy loading for long notification lists.
- Support additional attachment preview types (PDF, PNG, DOCX).
- Role-based access (e.g., teachers vs. students).

## 📄 License

This project is distributed under the terms of the [MIT License](LICENSE).

---
Feel free to open issues or pull requests if you find bugs or want to contribute enhancements!
