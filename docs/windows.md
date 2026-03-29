# Windows Build Notes

## Release icon resources

Windows executable icons are generated from `internal/resources/assets/app_light.svg`.

Committed files:

- `cmd/qbtremotego/icon_windows.ico`
- `cmd/qbtremotego/icon_windows_amd64.syso`

Regenerate them after changing the app icon:

```bash
magick internal/resources/assets/app_light.svg -background none -define icon:auto-resize=64,48,32,24,16 cmd/qbtremotego/icon_windows.ico
rsrc -arch amd64 -ico cmd/qbtremotego/icon_windows.ico -o cmd/qbtremotego/icon_windows_amd64.syso
```

GoReleaser and direct Windows builds rely on the committed `.syso` file to embed the executable icon. The Windows release build also uses `-H=windowsgui` so the GUI app does not open a console window.
