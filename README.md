# NetChecker

NetChecker is a Wails 3 desktop app for network monitoring. It collects ping and trace data, stores metrics in SQLite,
shows a live dashboard, and exports collected data to compressed CSV.

## Features

- Start and stop background network monitoring.
- Monitor configured targets and default gateway availability.
- Run trace checks on start, packet loss, or high RTT triggers.
- Export all collected metrics to `.csv.gz`.
- Upload exported data to AlfaDisk through the configured API shared link.
- Generate and persist a unique app instance ID on first launch.

## Runtime Data

The app stores runtime files in the OS user config directory:

- macOS: `~/Library/Application Support/netchecker`
- Windows: `%AppData%/netchecker`
- Linux: `~/.config/netchecker`

Important files:

- `config.json` - app configuration.
- `client_id` - generated unique app instance ID.
- `data/metrics.sqlite` - local metrics database.
- `netchecker.log` - app log.
- `exports/` - temporary export files used for AlfaDisk upload.

The generated `client_id` is used as the export file prefix. Example:

```text
NC-A1B2C3D4E5F6_20260618_123456.csv.gz
```

## AlfaDisk Configuration

AlfaDisk upload settings are read from `config.json` and are not displayed in the UI.

Example:

```json
{
  "alfaDisk": {
    "sharedLink": "https://alfadisk.alfabank.ru/shared-link/...",
    "password": "..."
  }
}
```

Do not commit real AlfaDisk credentials. For distribution, provide configuration per device or through your deployment
process.

## CLI Commands

Commands are handled through Wails single-instance mode. If the app is already running, a second launch passes the
command to the running instance.

```bash
netchecker start
netchecker stop
netchecker export /path/to/file.csv.gz
netchecker upload-alfadisk
```

`upload-alfadisk` exports current metrics to a `.csv.gz` file and uploads it to AlfaDisk using the configured link and
password.

On macOS, the app binary inside a packaged `.app` is usually:

```bash
./build/bin/netchecker.app/Contents/MacOS/netchecker upload-alfadisk
```

Check command results in the log:

```bash
tail -f "$HOME/Library/Application Support/netchecker/netchecker.log"
```

## Development

Install dependencies:

```bash
cd frontend
npm install
```

Run in development mode from the project root:

```bash
wails3 dev
```

Build frontend only:

```bash
cd frontend
npm run build
```

Build the desktop app:

```bash
wails3 build
```

Package the app for distribution:

```bash
wails3 package
```

## Project Structure

```text
frontend/              UI code built with Vite
internal/app/          Wails app service, export, AlfaDisk upload, app metadata
internal/config/       Configuration model and load/save helpers
internal/monitor/      Ping and trace monitoring logic
internal/storage/      SQLite storage and CSV export
build/                 Wails build configuration and platform assets
main.go                Wails app entry point and CLI command dispatcher
```

## Notes

- Wails version: `github.com/wailsapp/wails/v3`.
- Current app version is defined in `internal/app/version.go`.
- AlfaDisk uploads are limited to 60 MB per generated export file.
