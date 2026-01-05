package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rivo/tview"

	"github.com/flokiorg/twallet/components"
	"github.com/flokiorg/twallet/flnd"
	"github.com/flokiorg/twallet/load"
	"github.com/flokiorg/twallet/shared"
	"github.com/gdamore/tcell/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errInvalidPassphrase = errors.New("invalid wallet passphrase")

type rescanUI struct {
	instructions  string
	pages         *tview.Pages
	form          *tview.Form
	info          *tview.TextView
	startButton   *tview.Button
	logProgress   func(string)
	recordStatus  func(*load.RecoveryStatus)
	getLastStatus func() *load.RecoveryStatus
}

func (ui *rescanUI) setStartState(label string, disabled bool) {
	if ui == nil || ui.startButton == nil {
		return
	}
	ui.startButton.SetLabel(label)
	ui.startButton.SetDisabled(disabled)
}

func (ui *rescanUI) showProgress(w *Wallet) {
	if ui == nil || ui.pages == nil || w == nil || w.load == nil {
		return
	}
	w.load.QueueUpdateDraw(func() {
		ui.pages.SwitchToPage("progress")
	})
}

func (ui *rescanUI) promptForRetry(w *Wallet, message string) {
	if ui == nil || w == nil || w.load == nil {
		return
	}
	w.load.QueueUpdateDraw(func() {
		if ui.pages != nil {
			ui.pages.SwitchToPage("form")
		}
		if ui.info != nil {
			ui.info.SetText(message + "\n\n" + ui.instructions)
		}
		ui.setStartState("Start Rescan", false)
		if ui.form != nil {
			if passField, ok := ui.form.GetFormItem(0).(*tview.InputField); ok {
				passField.SetText("")
				w.load.Application.SetFocus(passField)
			}
		}
	})
}

func (w *Wallet) promptRescan() {
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	if rs, err := w.load.GetRecoveryStatus(); err == nil && rs != nil && rs.Info != nil {
		if rs.Info.GetRecoveryMode() && !rs.Info.GetRecoveryFinished() && rs.Info.GetProgress() < 1 {
			w.nav.ShowModal(components.NewDialog(
				"Rescan Already Running",
				"A wallet recovery/rescan is already in progress.\n\nPlease wait for it to finish before starting another rescan.",
				w.nav.CloseModal,
				[]string{"OK"},
				w.nav.CloseModal,
			))
			return
		}
	}

	netColor := shared.NetworkColor(*w.load.AppConfig.Network)
	progressView, logProgress, recordStatus, getLastStatus := w.newRescanProgressView(netColor)

	instructions := "Resetting wallet transactions requires a full blockchain rescan. The wallet will restart, lock, and remain unavailable until the process completes."

	info := tview.NewTextView()
	info.SetWrap(true)
	info.SetDynamicColors(true)
	info.SetText(instructions)
	info.SetBackgroundColor(tcell.ColorDefault)
	info.SetBorderPadding(1, 1, 2, 2)

	form := tview.NewForm()
	form.SetBackgroundColor(tcell.ColorDefault)
	form.SetBorderPadding(1, 1, 2, 2)

	defaultPass := strings.TrimSpace(w.load.AppConfig.DefaultPassword)
	form.AddPasswordField("Wallet passphrase:", defaultPass, 0, '*', nil)

	form.AddButton("Cancel", func() {
		w.closeRescanModal()
	})

	pages := tview.NewPages()
	ui := &rescanUI{
		instructions:  instructions,
		pages:         pages,
		form:          form,
		info:          info,
		logProgress:   logProgress,
		recordStatus:  recordStatus,
		getLastStatus: getLastStatus,
	}

	form.AddButton("Start Rescan", func() {
		passField := form.GetFormItem(0).(*tview.InputField)
		pass := strings.TrimSpace(passField.GetText())
		if pass == "" {
			info.SetText("[red]Wallet passphrase is required to unlock after restart.[-]\n\n" + instructions)
			w.load.Application.SetFocus(passField)
			return
		}
		ui.startButton = form.GetButton(1)
		ui.setStartState("Starting…", true)
		info.SetText("Preparing wallet rescan…")

		go func() {
			ui.logProgress("Preparing wallet rescan…")
			ui.showProgress(w)
			w.startRescan(pass, ui)
		}()
	})

	view := tview.NewFlex()
	view.SetDirection(tview.FlexRow)
	view.AddItem(info, 0, 1, false)
	view.AddItem(form, 0, 1, true)

	view.SetBorder(true).
		SetTitle("Wallet Rescan").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(netColor).
		SetBackgroundColor(netColor)

	pages.AddPage("form", view, true, true)
	pages.AddPage("progress", progressView, true, false)

	w.nav.ShowModal(components.NewModal(pages, 80, 16, nil))
	w.load.Application.SetFocus(form.GetFormItem(0))
}

func (w *Wallet) startRescan(pass string, ui *rescanUI) {
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return
	}
	w.busy = true
	w.rescanInProgress = true
	w.mu.Unlock()

	log := ui.logProgress
	if log == nil {
		log = func(string) {}
	}

	log("⏳ Restarting wallet for rescan…")

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-w.quit:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		defer cancel()

		started := time.Now()

		if err := w.load.Wallet.TriggerRescan(); err != nil {
			w.finalizeRescan(log, started, nil, fmt.Errorf("failed to start rescan: %w", err))
			return
		}

		log("⏳ Waiting for wallet to restart…")

		if err := w.autoUnlockAfterRescan(ctx, pass, log); err != nil {
			if errors.Is(err, errInvalidPassphrase) {
				w.mu.Lock()
				w.busy = false
				w.rescanInProgress = false
				w.mu.Unlock()
				ui.promptForRetry(w, "[red]Incorrect wallet passphrase. Please try again.[-]")
				return
			}

			w.finalizeRescan(log, started, nil, fmt.Errorf("failed to unlock wallet after restart: %w", err))
			return
		}

		log("🔓 Wallet unlocked. Waiting for wallet RPC…")

		if err := w.waitForWalletRPC(ctx, log); err != nil {
			w.finalizeRescan(log, started, nil, fmt.Errorf("wallet RPC not ready: %w", err))
			return
		}

		log("✅ Wallet RPC ready. Monitoring recovery…")

		status, err := w.load.MonitorRecovery(ctx, time.Second, func(rs *load.RecoveryStatus) bool {
			if rs == nil {
				return ctx.Err() == nil
			}
			if ui.recordStatus != nil {
				ui.recordStatus(rs)
			}
			log(w.recoveryProgressMessage(rs))
			return ctx.Err() == nil
		})

		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			w.finalizeRescan(log, started, status, fmt.Errorf("rescan failed: %w", err))
			return
		}

		var finalStatus *load.RecoveryStatus
		if ui.getLastStatus != nil {
			finalStatus = ui.getLastStatus()
		}
		if finalStatus == nil {
			finalStatus = status
		}

		w.finalizeRescan(log, started, finalStatus, err)
	}()
}

func (w *Wallet) finalizeRescan(logProgress func(string), started time.Time, status *load.RecoveryStatus, runErr error) {
	w.mu.Lock()
	w.busy = false
	w.rescanInProgress = false
	w.mu.Unlock()

	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		w.load.QueueUpdateDraw(func() {
			w.nav.CloseModal()
			w.focusActiveView()
		})
		return
	}

	count := 0
	if status != nil {
		count = status.UTXOCount
	}

	if runErr == nil && (status == nil || count == 0) && w.load != nil && w.load.Wallet != nil {
		if latest, err := w.load.GetRecoveryStatus(); err == nil && latest != nil {
			if latest.UTXOCount > count {
				status = latest
				count = latest.UTXOCount
			}
		}
	}

	rescanDuration := humanDuration(time.Since(started))

	if runErr == nil {
		logProgress(fmt.Sprintf("✅ Rescan complete! [%d] UTXO recovered", count))
	} else {
		logProgress(fmt.Sprintf("[red:-:-]Rescan error:[-:-:-] %v", runErr))
	}

	w.load.QueueUpdateDraw(func() {
		w.nav.CloseModal()

		if runErr != nil {
			errMessage := fmt.Sprintf("Rescan failed: %v", runErr)
			w.nav.ShowModal(components.ErrorModal(errMessage, func() {
				w.nav.CloseModal()
				w.focusActiveView()
			}))
			return
		}

		summary := fmt.Sprintf("Rescan completed in %s.\nRecovered %d UTXOs.", rescanDuration, count)
		dialog := components.NewDialog("Rescan Complete", summary, nil, []string{"Continue"}, func() {
			w.nav.CloseModal()
			w.load.Notif.BroadcastBalanceRefresh()
			w.focusActiveView()
		})
		w.nav.ShowModal(dialog)
	})
}

func (w *Wallet) autoUnlockAfterRescan(ctx context.Context, pass string, logProgress func(string)) error {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return errors.New("wallet passphrase required")
	}

	const (
		maxAttempts = 30
		retryDelay  = 5 * time.Second
	)

	sub := w.load.Wallet.Subscribe()
	defer w.load.Wallet.Unsubscribe(sub)

	timer := time.NewTimer(0)
	defer timer.Stop()

	resetTimer := func(d time.Duration) {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)
	}

	attempts := 0
	awaitingConfirmation := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case update, ok := <-sub:
			if !ok || update == nil {
				return errors.New("wallet subscription closed while unlocking")
			}

			switch update.State {
			case flnd.StatusUnlocked, flnd.StatusReady, flnd.StatusSyncing:
				logProgress("🔓 Wallet unlock confirmed.")
				w.load.QueueUpdateDraw(func() {
					w.load.Notif.ShowToastWithTimeout("🔓 Wallet unlocked.", time.Second*2)
				})
				return nil

			case flnd.StatusLocked:
				if awaitingConfirmation {
					logProgress("Wallet reported locked again. Retrying unlock…")
				}
				awaitingConfirmation = false
				resetTimer(0)

			case flnd.StatusDown:
				if update.Err != nil {
					logProgress(fmt.Sprintf("[red:-:-]Wallet down:[-:-:-] %v", update.Err))
				} else {
					logProgress("Wallet service reported down state during unlock.")
				}
				awaitingConfirmation = false
				resetTimer(retryDelay)

			case flnd.StatusNone:
				logProgress("Wallet initializing…")
				awaitingConfirmation = false
				resetTimer(retryDelay)
			}

		case <-timer.C:
			if awaitingConfirmation {
				logProgress("Unlock confirmation timed out. Retrying…")
				awaitingConfirmation = false
				resetTimer(0)
				continue
			}

			if attempts >= maxAttempts {
				logProgress("Exceeded maximum unlock attempts.")
				return errors.New("wallet did not unlock after multiple attempts")
			}

			attempts++
			logProgress(fmt.Sprintf("Attempting to unlock wallet (%d/%d)…", attempts, maxAttempts))

			err := w.load.Wallet.Unlock(pass)
			if err == nil {
				awaitingConfirmation = true
				logProgress("Unlock RPC accepted. Awaiting confirmation…")
				resetTimer(retryDelay)
				continue
			}

			st, ok := status.FromError(err)
			if ok {
				msg := strings.ToLower(st.Message())
				if strings.Contains(msg, "already unlocked") {
					logProgress("Wallet already unlocked.")
					w.load.QueueUpdateDraw(func() {
						w.load.Notif.ShowToastWithTimeout("🔓 Wallet unlocked.", time.Second*2)
					})
					return nil
				}
				if strings.Contains(msg, "invalid passphrase") {
					logProgress("[red:-:-]Unlock failed:[-:-:-] invalid passphrase provided.")
					return errInvalidPassphrase
				}
				switch st.Code() {
				case codes.Unavailable, codes.Canceled, codes.DeadlineExceeded, codes.FailedPrecondition, codes.Unknown:
					logProgress("Wallet service not ready. Waiting before retry…")
					resetTimer(retryDelay)
					continue
				default:
					logProgress(fmt.Sprintf("[red:-:-]Unlock failed:[-:-:-] %v", err))
					return err
				}
			}

			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "already unlocked") {
				logProgress("Wallet already unlocked.")
				w.load.QueueUpdateDraw(func() {
					w.load.Notif.ShowToastWithTimeout("🔓 Wallet unlocked.", time.Second*2)
				})
				return nil
			}
			if strings.Contains(lower, "invalid passphrase") {
				logProgress("[red:-:-]Unlock failed:[-:-:-] invalid passphrase provided.")
				return errInvalidPassphrase
			}

			logProgress(fmt.Sprintf("[red:-:-]Unlock failed:[-:-:-] %v", err))
			resetTimer(retryDelay)
			continue
		}
	}
}

func (w *Wallet) waitForWalletRPC(ctx context.Context, logProgress func(string)) error {
	sub := w.load.Wallet.Subscribe()
	defer w.load.Wallet.Unsubscribe(sub)

	logProgress("Waiting for wallet RPC readiness signals…")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case update, ok := <-sub:
			if !ok || update == nil {
				return errors.New("wallet subscription closed while waiting for RPC")
			}

			switch update.State {
			case flnd.StatusReady, flnd.StatusBlock, flnd.StatusTransaction, flnd.StatusSyncing:
				logProgress("Wallet RPC ready.")
				return nil

			case flnd.StatusUnlocked:
				logProgress("Wallet unlocked. Waiting for RPC to become active…")

			case flnd.StatusDown:
				if update.Err != nil {
					logProgress(fmt.Sprintf("[red:-:-]Wallet down:[-:-:-] %v", update.Err))
				} else {
					logProgress("Wallet service reported down state. Waiting…")
				}

			case flnd.StatusNone:
				logProgress("Wallet initializing RPC services…")

			case flnd.StatusLocked:
				logProgress("Wallet locked; still waiting for RPC availability…")

			default:
				// ignore other states
			}
		}
	}
}

func (w *Wallet) newRescanProgressView(netColor tcell.Color) (*tview.TextView, func(string), func(*load.RecoveryStatus), func() *load.RecoveryStatus) {
	progressView := tview.NewTextView()
	progressView.SetDynamicColors(true)
	progressView.SetWrap(true)
	progressView.SetScrollable(true)
	progressView.SetChangedFunc(func() { progressView.ScrollToEnd() })
	progressView.SetTextAlign(tview.AlignLeft)
	progressView.SetBorder(true)
	progressView.SetTitle("Wallet Rescan")
	progressView.SetTitleAlign(tview.AlignCenter)
	progressView.SetTitleColor(netColor)
	progressView.SetBorderColor(netColor)
	progressView.SetBorderPadding(1, 1, 2, 2)
	progressView.SetBackgroundColor(tcell.ColorDefault)

	var mu sync.Mutex
	logLines := make([]string, 0, 8)
	progressLineIdx := -1
	var lastStatus *load.RecoveryStatus
	var bestStatus *load.RecoveryStatus

	appendLine := func(message string) {
		if message == "" {
			return
		}

		timestamp := time.Now().Format("15:04:05")
		mu.Lock()
		line := fmt.Sprintf("[%s] %s", timestamp, message)

		if strings.Contains(message, "Recovery in progress") {
			if progressLineIdx >= 0 && progressLineIdx < len(logLines) {
				logLines[progressLineIdx] = line
			} else {
				progressLineIdx = len(logLines)
				logLines = append(logLines, line)
			}
		} else {
			logLines = append(logLines, line)
			progressLineIdx = -1
		}
		content := strings.Join(logLines, "\n")
		mu.Unlock()

		if w.load == nil || w.load.Application == nil {
			return
		}

		app := w.load.Application
		text := content
		go func() {
			app.QueueUpdateDraw(func() {
				progressView.SetText(text)
			})
		}()
	}

	updateStatus := func(status *load.RecoveryStatus) {
		if status == nil {
			return
		}
		copyStatus := *status
		mu.Lock()
		lastStatus = &copyStatus
		if copyStatus.UTXOCount > 0 && (bestStatus == nil || copyStatus.UTXOCount >= bestStatus.UTXOCount) {
			bestCopy := copyStatus
			bestStatus = &bestCopy
		}
		mu.Unlock()
	}

	return progressView, appendLine, updateStatus, func() *load.RecoveryStatus {
		mu.Lock()
		defer mu.Unlock()
		if bestStatus != nil {
			copyBest := *bestStatus
			return &copyBest
		}
		if lastStatus != nil {
			copyLast := *lastStatus
			return &copyLast
		}
		return nil
	}
}

func (w *Wallet) recoveryProgressMessage(rs *load.RecoveryStatus) string {
	count := 0
	if rs != nil {
		count = rs.UTXOCount
	}

	var percentText string
	if rs != nil && rs.Info != nil {
		progress := rs.Info.GetProgress() * 100
		if progress > 100 {
			progress = 100
		}
		if rs.Info.GetRecoveryFinished() {
			progress = 100
		}
		if progress > 0 {
			percentText = fmt.Sprintf(" • %.2f%% complete", progress)
		}
	}

	return fmt.Sprintf("⏳ Recovery in progress… [%d] UTXO recovered %s", count, percentText)
}

func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return "1s"
	}
	return d.Round(time.Second).String()
}

func (w *Wallet) isRescanActive() bool {
	w.mu.Lock()
	active := w.rescanInProgress
	w.mu.Unlock()
	return active
}

func (w *Wallet) closeRescanModal() {
	if w == nil || w.load == nil || w.nav == nil {
		return
	}
	shouldRestart := w.isRescanActive() || w.isWalletLocked()
	w.mu.Lock()
	w.busy = false
	w.rescanInProgress = false
	w.mu.Unlock()
	if shouldRestart {
		w.restartWithoutRescan()
		w.navigateToUnlockPage()
	} else if w.isWalletLocked() {
		if !w.navigateToUnlockPage() {
			w.focusActiveView()
		}
	}
	w.nav.CloseModal()
}

func (w *Wallet) isWalletLocked() bool {
	if w == nil || w.load == nil || w.load.Wallet == nil {
		return false
	}
	locked, err := w.load.Wallet.IsLocked()
	if err == nil {
		return locked
	}

	if ev := w.load.Wallet.GetLastEvent(); ev != nil {
		return ev.State == flnd.StatusLocked
	}

	// If we cannot determine the state, assume locked to keep user safe.
	return true
}

func (w *Wallet) navigateToUnlockPage() bool {
	if w == nil || w.load == nil || w.load.Router == nil {
		return false
	}
	w.load.Go(shared.LOCK)
	return true
}

func (w *Wallet) restartWithoutRescan() {
	if w == nil || w.load == nil || w.load.Wallet == nil {
		return
	}
	go w.load.Wallet.Restart(context.Background())
}
