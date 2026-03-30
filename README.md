# tWallet

tWallet is a terminal UI wallet for Flokicoin focused on speed, clarity, and self-custody.

Learn more: https://docs.flokicoin.org/wallets/twallet

## Reporting Issues

If you encounter a bug or have a feature request, please use the [GitHub Issue Tracker](https://github.com/flokiorg/twallet/issues). 

When reporting a bug, please include:
1.  A clear description of the issue.
2.  The output of `twallet --version`.
3.  Any relevant logs from your data directory.

## Data Locations

tWallet stores its data (configuration, wallet database, and logs) in the following default locations:

| OS | Path |
|---|---|
| **Linux** | `~/.flnd/` |
| **macOS** | `~/Library/Application Support/Flnd/` |
| **Windows** | `%LOCALAPPDATA%\Flnd\` |

### Logs

*   `twallet.log`: General application UI logs.
*   `crash.log`: If the application crashes or panics, a stack trace is saved here.
*   `flnd.log`: Detailed logs from the underlying node (found in `logs/flokicoin/<network>/flnd.log`).
