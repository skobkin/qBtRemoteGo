# Repository Guidelines

## Project Structure & Module Organization
- `cmd/qbtremotego`: desktop app entrypoint and committed Windows icon resources.
- `internal/app`: controller logic, add-dialog validation/filtering, and build metadata.
- `internal/ui`: Fyne windows, dialogs, torrent table widgets, tray behavior, and UI-focused tests.
- `internal/qbt`: qBittorrent Web API client, request/response models, and remote path/category/tag helpers.
- `internal/config`: config schema, defaults, normalization, and load/save helpers.
- `internal/platform`: OS-specific magnet handler, `.torrent` handler, and autostart integration.
- `internal/resources/assets`: packaged SVG assets used by the GUI and release icon generation.
- `.golangci.yml`, `.goreleaser.yml`, `.woodpecker/ci.yaml`, and `docs/windows.md`: CI/release files that should stay aligned with code and asset changes.

## Build, Test, and Development Commands
- `go build ./...`: build all packages and the GUI binary.
- `go run ./cmd/qbtremotego`: start the desktop app.
- `go test ./...`: run unit tests.
- `go vet ./...`: run baseline static checks used by CI.
- `golangci-lint run`: run the configured lint suite.
- `goreleaser check`: validate the release config.
- `goreleaser release --snapshot --clean`: build local snapshot release artifacts in `dist/`.
- If you need a disposable manual-test binary, build it into ignored `dist/`: `go build -trimpath -o dist/qbtremotego ./cmd/qbtremotego`.

## Windows Icon Regeneration
- Source icon: `internal/resources/assets/app_light.svg`.
- Regenerate ICO: `magick internal/resources/assets/app_light.svg -background none -define icon:auto-resize=64,48,32,24,16 cmd/qbtremotego/icon_windows.ico`
- Regenerate Windows resource object: `rsrc -arch amd64 -ico cmd/qbtremotego/icon_windows.ico -o cmd/qbtremotego/icon_windows_amd64.syso`
- Commit both files together when the app icon changes: `cmd/qbtremotego/icon_windows.ico` and `cmd/qbtremotego/icon_windows_amd64.syso`.

## Coding Style & Naming Conventions
- Language: Go (`go 1.26` in `go.mod`).
- Formatting is mandatory: run `gofmt -w` on changed Go files.
- Package names are short lowercase nouns (`ui`, `config`, `platform`, `qbt`).
- Exported identifiers use `PascalCase`; internal helpers use `camelCase`.
- Keep Fyne UI updates on the UI thread with `fyne.Do` or `fyne.DoAndWait` when work originates from goroutines.
- Use structured logging with `slog`; include actionable context such as remote URL, trigger, or integration target.
- Remote save paths, categories, and tags refer to the qBittorrent server, not the local desktop machine.
- Prefer graceful degradation: surface missing data or partial integration failures without crashing the app.

## Testing Guidelines
- Place tests next to code using `*_test.go`.
- Prefer table-driven tests for config normalization, add-request validation, torrent filtering/sorting, and qBittorrent client behavior.
- Run focused tests while iterating, then `go test ./...` before finishing.
- Coverage is pragmatic: new logic paths should include tests, especially around config persistence, request encoding, integration argument parsing, and error handling.
- GUI tests should avoid brittle timing dependencies; prefer controller/widget logic coverage over full interactive event-loop tests unless the UI behavior truly requires it.
- If you need local binaries for manual testing, put them in `dist/`, which is ignored by Git.

## Completion Checklist
- Before finishing work and saying it is done, run the same baseline checks as CI:
  - `gofmt -w` on changed Go files
  - `go vet ./...`
  - `golangci-lint run`
  - `go test ./...`
- If the task changes release automation, packaging, or Windows resources, also run:
  - `goreleaser check`
  - `goreleaser release --snapshot --clean`
- Do not state that work is done if any of the relevant checks above fail.

## Commit & Pull Request Guidelines
- Follow Conventional Commits used in history: `feat(ui): ...`, `fix(qbt): ...`, `chore(ci): ...`.
- Keep commits scoped and explain behavioral impact in the subject.
- PRs should include a clear summary of user-visible changes.
- PRs should include testing performed (`go test ./...`, manual GUI checks, release validation when relevant).
- PRs should include screenshots or GIFs for UI changes.
- If a PR changes protocol/file-handler registration, autostart behavior, or release/CI settings, call that out explicitly in the description.

## Configuration & Data Paths
- Runtime config is stored under `os.UserConfigDir()/qbtremotego/config.json`.
- The app currently logs through `slog` to standard output; there is no repo-managed log file path to keep in sync.
- Avoid hard-coding qBittorrent URLs, credentials, or user-specific filesystem paths in code or tests.
- Treat OS integration sync as best-effort: failures should be reported clearly without breaking the rest of the app.
