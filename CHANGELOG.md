# Changelog

## [1.0.16]

### Fixed

- **GO-2026-6443**, a remotely triggerable server panic in
  `google.golang.org/grpc` (missing `:authority`/`Host` header), was reachable
  here and shipped in 1.0.15. grpc is pinned to **v1.83.2**, which the
  advisory does not cover, and `govulncheck ./...` now reports no reachable
  findings.

  This could not be fixed when the other repos were, because MVS lets a
  dependent only raise a requirement, never lower it, and flnd 0.2.3 required
  the affected v1.84.0. It needed flnd 0.2.4 to carry the pin first.

  Earlier notes in this org called the finding unfixable, on the grounds that
  the only fix was an unreleased v1.85.0 development build. That misread
  govulncheck's `Fixed in:` line, which names the next fix above the version
  in use. The affected ranges are `[0, 1.82.2)`, `[1.83.0, 1.83.2)` and
  `[1.84.0-dev, 1.85.0-dev...)`.

### Changed

- `flnd` 0.2.3 -> **0.2.4**.

## [1.0.15]

### Changed

- Updated every flokiorg dependency to its current release: `flnd` v0.2.3,
  `go-flokicoin` v0.26.3, `walletd` v0.2.2 and `flokicoin-neutrino` v0.17.2.
- Built with Go 1.26.8, up from 1.26.5, which closes four reachable stdlib
  vulnerabilities (GO-2026-6218 `net/url`, GO-2026-6090 `crypto/tls`,
  GO-2026-5972 `encoding/asn1`, GO-2026-5026 `net/http`).
- The release now publishes a multi-arch container image to
  `ghcr.io/flokiorg/tWallet`, and every push to `main` publishes an `:edge`
  image.

### Known issue

- `GO-2026-6443`, a server panic in `google.golang.org/grpc` via a missing
  `:authority` or `Host` header, has no stable fix -- upstream's patch exists
  only in an unreleased v1.85.0 development build. Accepted and monitored
  rather than pinning a pre-release dependency.

### Changed

- Built with Go 1.26.5. (#3)

## [1.0.14-beta]

### Fixes

- **`flnd/client.go`**: `PublishTransaction` passed a dynamic string as a format string to `fmt.Errorf`; switched to `errors.New` since no formatting is needed.
- **Test helper**: `createTestTempDir` had its real `os.MkdirTemp` implementation restored (it had been replaced with a hardcoded absolute path from a developer machine).
- **Test naming**: a function named `TestMain` but taking `*testing.T` (not `*testing.M`) never actually served as the package's test-entry-point hook — it just ran as a regular, very heavy, real-daemon-launching test under a confusing name. Renamed to `TestManualFlndTestnetLaunch`.
- **Integration tests**: all 4 test files in `flnd/` spawn a real `flnd` daemon and/or need live testnet connectivity. Gated behind `//go:build integration` so a plain `go test ./...` (used in CI) skips them cleanly; still runnable explicitly with `-tags=integration`.

### CI

- Added `.github/workflows/ci.yaml`: runs `go build`, `go vet`, and `go test` on push to `main` and on pull requests.

## [1.0.13-beta]

Dependency-only update: brings in `flnd` v0.2.0-beta, `go-flokicoin` v0.26.0-alpha, and `walletd` v0.2.0-beta. No code changes in this repo.

See the [flnd v0.2.0-beta](https://github.com/flokiorg/flnd/releases/tag/v0.2.0-beta), [go-flokicoin v0.26.0-alpha](https://github.com/flokiorg/go-flokicoin/releases/tag/v0.26.0-alpha), and walletd v0.2.0-beta release notes for what's new upstream — notably walletd's coin-selection and transaction-confirmation fixes.

## [1.0.12-beta]

### Dependency Updates

- Updated core dependencies to align with `flnd v0.1.21-beta`, which includes Taproot channel support and fixes for 32-bit platforms.
- Routine `go mod tidy` cleanup.

## [1.0.11-beta]

#### Lightning Peer Port

- **Default Peer Port**: Corrected the default Lightning P2P peer port in the internal `flnd` service description from the Bitcoin Lightning value (`9735`) to the Flokicoin Lightning value (`5521`).

### Dependency Updates

- Updated dependencies to align with `flnd v0.1.20-beta` and `go-flokicoin v0.25.13-alpha`.
- Routine `go mod tidy` cleanup.

## [1.0.9-beta]

This release integrates a dedicated internal Lightning connector and adds a configuration view for node management.

Notable Changes
===============

Internal Lightning Connector
----------------------------
The wallet now implements an internal connector strategy. The connection management and service logic have been moved internally to `tWallet`, decoupling the application from the external `FLND` repository. This provides a stable, application-specific interface for all Lightning operations.

Configuration Updates
---------------------
- Added mapping for additional `flnd` configuration options and synchronized default values to ensure consistency with the core Lightning implementation.

New: Lightning Configuration Modal
----------------------------------
A new configuration modal is accessible via the `Ctrl+N` shortcut.

- **Connection Details**: Displays RPC Address, Peer Address, and Identity PubKey.
- **Credentials**: Exports Macaroon and TLS Certificate in hex format for configuring external tools like `LokiHub`, `flncli`, or web dashboards.
- **Clipboard**: Implements a new cross-platform copy mechanism with robust fallback support (OSC 52, native OS commands) for seamless use in local and remote environments.

First Run Auto-Unlock
---------------------
To support automated deployments and Ops workflows, a new `autounlock` configuration option has been added.
- **Automated Startup**: When enabled alongside `defaultpassword`, the wallet silently unlocks on the initial application run without requiring user interaction.
- **Stealth UI**: The unlock form is hidden during the automatic process, displaying only a status indicator.
- **Security**: This behavior is restricted to the first run only; subsequent manual locking/unlocking operations retain the standard password prompt for security.

Crash Reporting
---------------
Added automatic crash logging to help quickly diagnose unexpected application exits. If the wallet encounters a critical error, it now safely captures and saves the detailed error information to a `crash.log` file across all platforms, making it much easier to troubleshoot and resolve issues.

## [1.0.8-beta.2]

- removed tui resize guard
- fixed some keyboard and window resizing issues on windows (via tcell upgrade)
- bumped flnd and tcell dependencies
- updated app version to 1.0.8-beta.2

## [1.0.8-beta]

- Startup and recovery
  - Added a guided recovery path that clears Neutrino cache files and restarts the wallet service when boot fails (hotkey available during splash).
  - Startup now blocks launch until the service reports a healthy state; down/EOF conditions surface clear instructions to recover or exit.
- Restore and onboarding
  - Restore flow now monitors recovery progress, shows toast updates, and waits for RPC readiness before entering the main app.
  - Seed create/restore transitions avoid premature navigation by tracking restore state and waiting for wallet availability.
- Wallet UI
  - Switches/buttons gained configurable colors and maintain active styling in sync with state.
  - Cipher copy now confirms via toast; receive flow messaging is clearer.
  - TUI now skips draws when the terminal reports zero dimensions and clears the screen once a valid size returns, avoiding resize panics and stale frames.
  - Balance header always shows unconfirmed funds even when locked funds are present, keeping pending exposure consistent.
- Configuration
  - Sample config documents `transactiondisplaylimit`, clarifying it caps the number of transactions shown without changing how many are fetched.
- Stability and caching
  - Load cache now tracks balances and chain tip height; notifications update tip on new blocks/transactions.
  - Recovery monitoring retries until the wallet RPC is ready and reports recovery completion.
- Defaults and dependencies
  - CLI defaults set a transaction display limit and clarify fee URL guidance; sample config documents expected fee API response.
  - Dependencies bumped: flnd v0.1.8-beta, walletd v0.1.5-beta; VERSION set to 1.0.8-beta.

## [1.0.7-beta]

- This is a **pre-release** for testing and feedback.
- Developers and early adopters are encouraged to **report issues**.

## [1.0.6-beta]

This is a **pre-release** for testing and feedback.

### Changes

- add configurable logging level and restructure loggers to write to file only
- show `flnd.log` tail inside the wallet view with Ctrl+L/Ctrl+T shortcuts
- refine notifications, balance header, and footer messaging with structured logging
- refresh unlock/change/onboard dialogs and improve table styling
- update splash screen branding and version metadata
- replace legacy CLI bootstrap with new config package and helper script

## [1.0.5-beta]

### Changes

#### Dependencies
- Updated `flokicoin-neutrino` → **v0.16.3-beta**

#### Fixes
- Depends on neutrino with params‑sourced filter checkpoints.

### Notes
- This release aligns with the latest upstream library updates
- Users are encouraged to upgrade for better stability and compatibility

## [1.0.4-beta]

### Changes

#### Dependencies
- Updated `go-flokicoin` → **v0.25.7-beta**
- Updated `flokicoin-neutrino` → **v0.16.2-beta**
- Updated `walletd` → **v0.1.3-beta**

#### Fixes
- General bug fixes and stability improvements
- Improved reliability of status handling during sync/init

### Notes
- This release aligns with the latest upstream library updates
- Users are encouraged to upgrade for better stability and compatibility

## [1.0.3-alpha]

## Changelog

- Registered the WalletKit gRPC service (`walletrpc.WalletKit`) so WalletKit endpoints are now available.

## [1.0.2-alpha]

## Changelog

#### Added
- Support for custom TLS/IP config:
  - `tlsextraip`, `tlsextradomain`, `tlsautorefresh`
- Network listeners:
  - `rpclisten`, `restlisten`, `listen`
- CORS support for REST API via `restcors`

#### Fixed
- Async-safe page switching
- Detailed boot error reporting

## [1.0.1-alpha]

This is a **pre-release** for testing and feedback. Developers and early adopters are encouraged to **report issues**.

## Changelog

- Migrated backend from **Electrum** to **Neutrino**
- Integrated **Flokicoin Lightning Node (LN)** with full LN stack support
- Enables **native Lightning development** on the Flokicoin network
- Added configurable address types: `segwit`, `nested-segwit`, `taproot`
- Introduced secure wallet locking/unlocking and password change UI
- Improved transaction builder with live fee estimation
- Enhanced **balance display** with confirmed and unconfirmed amounts
- Enhanced terminal UI with sync status, LN health, and live updates
- Major internal refactor for modular config, stability, and future expansion

## [0.1.1-alpha]

- Fee configuration options: `feeslow`, `feemedium`, `feefast`
- Renamed app data directory from `flcwallet` to `twallet`

## [0.1.0-alpha]

- This is a **pre-release** for testing and feedback.
- Developers and early adopters are encouraged to **report issues**.
