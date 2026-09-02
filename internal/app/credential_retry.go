package app

import (
	"context"
	"strings"
	"time"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
)

// Tunables for bounded keychain access and the background retry loop. They are
// package variables so tests can shorten them without fake clocks; internal/app
// tests run serially, and every override is restored via t.Cleanup.
var (
	// startupCredentialTimeout bounds the single synchronous keychain read at
	// construction, so the window can start even while the wallet is still
	// unlocking.
	startupCredentialTimeout = 2 * time.Second
	// retryCredentialInterval is the pause between background load attempts.
	retryCredentialInterval = 5 * time.Second
	// retryCredentialTimeout bounds each individual attempt.
	retryCredentialTimeout = 5 * time.Second
	// maxCredentialRetries caps the number of attempts before the loop records
	// a terminal, actionable status (~10 minutes at the defaults).
	maxCredentialRetries = 60
)

// callWithTimeout runs fn in its own goroutine and bounds how long the caller
// waits for its result. go-keyring's D-Bus calls ignore contexts, so a wedged
// Secret Service call cannot be cancelled: the abandoned goroutine is left to
// finish on its own and its result is absorbed by the buffered channel.
func callWithTimeout[T any](timeout time.Duration, fn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}

	// Buffered so the abandoned goroutine never leaks on the send.
	delivered := make(chan result, 1)
	go func() {
		value, err := fn()
		delivered <- result{value: value, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-delivered:
		return res.value, res.err
	case <-timer.C:
		var zero T

		return zero, errKeychainTimeout
	}
}

// CredentialUnavailableError reports that credentials are configured to live in
// the system keychain but have not been loaded yet. The UI shows the carried
// status instead of a plain connection failure. Waiting distinguishes an
// ongoing background retry (a transient waiting state) from a terminal failure
// whose status message is already actionable on its own.
type CredentialUnavailableError struct {
	Status  credentials.Status
	Waiting bool
}

func (e *CredentialUnavailableError) Error() string {
	message := "credentials are not loaded yet"
	if e != nil && strings.TrimSpace(e.Status.Message) != "" {
		message = e.Status.Message
	}
	if e == nil || !e.Waiting {
		return message
	}

	return "waiting for system keychain: " + message
}

// credentialsRetryPending reports whether a background loop is currently trying
// to load the keychain credentials.
func (c *Controller) credentialsRetryPending() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.credentialRetryActive
}

// CredentialRetryActive is the exported view of credentialsRetryPending: it
// reports whether the background loop is currently re-reading the keychain for
// stored credentials. The UI uses it to present stored-but-unloaded
// credentials as "still retrying" rather than failed.
func (c *Controller) CredentialRetryActive() bool {
	return c.credentialsRetryPending()
}

// keychainLoadPending reports whether a background retry could still load
// keychain credentials into the still-empty session, i.e. the keychain has not
// concluded that nothing is stored. The stored-credentials marker makes "not
// found" mean "stored but not loaded yet"; configs saved before the marker
// existed carry no marker, so for them every non-available keychain state — a
// locked wallet, a timed-out or otherwise failing read — must keep retrying.
// Only a reachable keychain holding nothing (available + not found) rules a
// retry out.
func (c *Controller) keychainLoadPending() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.config.Connection.CredentialStorage == config.CredentialStorageKeychain &&
		c.sessionCredentials == (credentials.Credentials{}) &&
		c.credentialStatus.State != credentials.StateAvailable
}

// startCredentialRetryLoop launches the background loop that re-reads the
// keychain until the stored credentials show up, the user saves credentials,
// the storage mode changes, or the retry cap is exhausted.
func (c *Controller) startCredentialRetryLoop() {
	c.stateMu.Lock()
	if c.credentialRetryActive {
		c.stateMu.Unlock()

		return
	}
	c.credentialRetryActive = true
	c.stateMu.Unlock()

	go c.credentialRetryLoop()
}

// stopCredentialRetries stops the retry loop if one is running and waits for
// it to finish. Safe to call repeatedly and when no loop was ever started.
func (c *Controller) stopCredentialRetries() {
	c.credentialRetryOnce.Do(func() {
		close(c.credentialRetryStop)
	})
	if c.credentialsRetryPending() {
		<-c.credentialRetryDone
	}
}

func (c *Controller) credentialRetryLoop() {
	defer func() {
		c.stateMu.Lock()
		c.credentialRetryActive = false
		c.stateMu.Unlock()
		close(c.credentialRetryDone)
	}()

	for attempt := 1; ; attempt++ {
		select {
		case <-c.credentialRetryStop:
			return
		default:
		}

		if c.credentialRetryStep() {
			return
		}
		if attempt >= maxCredentialRetries {
			c.giveUpCredentialRetries(attempt)

			return
		}

		timer := time.NewTimer(retryCredentialInterval)
		select {
		case <-c.credentialRetryStop:
			timer.Stop()

			return
		case <-timer.C:
		}
	}
}

// credentialRetryStep performs one bounded keychain load attempt and reports
// whether the loop should stop. It is callable outside the loop so tests can
// drive attempts deterministically instead of sleeping.
func (c *Controller) credentialRetryStep() (stop bool) {
	mode, marker := c.keychainLoadContext()
	if mode != config.CredentialStorageKeychain {
		return true
	}
	if c.SessionCredentials() != (credentials.Credentials{}) {
		return true
	}

	creds, err := callWithTimeout(retryCredentialTimeout, func() (credentials.Credentials, error) {
		return c.credentialStore.Get(context.Background())
	})
	if err == nil {
		if !c.applyKeychainCredentials(creds) {
			// Credentials were saved (or the mode changed) while the attempt
			// ran; whatever is in the session now wins.
			return true
		}
		c.currentLogger().Info("system keychain credentials loaded after retry", "backend", credentials.BackendName())

		return true
	}

	status := keychainLoadStatus(marker, err)
	c.setCredentialStatus(status)
	if !retryableCredentialStatus(status) {
		c.currentLogger().Warn("system keychain credentials not recoverable by retrying", "backend", status.Backend, "state", status.State, "error", err)

		return true
	}
	c.currentLogger().Debug("system keychain credentials still unavailable", "backend", status.Backend, "state", status.State, "error", err)

	return false
}

// giveUpCredentialRetries records a terminal status after the retry cap, with
// an actionable message instead of the waiting hint.
func (c *Controller) giveUpCredentialRetries(attempts int) {
	status := credentials.Status{
		Backend: credentials.BackendName(),
		State:   credentials.StateUnavailable,
		Message: "Gave up waiting for the system keychain; open Settings > Connection and re-enter the credentials.",
	}
	c.setCredentialStatus(status)
	c.currentLogger().Warn("gave up waiting for system keychain credentials", "backend", status.Backend, "attempts", attempts)
}

func (c *Controller) keychainLoadContext() (config.CredentialStorageMode, bool) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	return c.config.Connection.CredentialStorage, c.config.Connection.KeychainHasCredentials
}

// applyKeychainCredentials records credentials loaded from the keychain. The
// acceptance re-check happens under the same lock that guards concurrent
// saves, so a retry finishing after the user re-entered credentials can never
// clobber them.
func (c *Controller) applyKeychainCredentials(creds credentials.Credentials) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.config.Connection.CredentialStorage != config.CredentialStorageKeychain {
		return false
	}
	if c.sessionCredentials != (credentials.Credentials{}) {
		return false
	}

	method := inferAuthMethod(c.config.Connection.AuthMethod, creds)
	if method != c.config.Connection.AuthMethod {
		c.config.Connection.AuthMethod = method
		c.logger.Info("kept password auth for credentials stored before auth methods existed")
	}
	c.sessionCredentials = canonicalCredentials(method, creds)
	c.credentialStatus = credentials.Status{
		Backend: credentials.BackendName(),
		State:   credentials.StateAvailable,
		Message: "System keychain is available.",
	}

	return true
}

// boundedKeychainStatus probes the keychain availability with a timeout,
// mapping a timeout onto the same unavailable status shape as other failures.
func boundedKeychainStatus(timeout time.Duration, store credentials.Store) credentials.Status {
	status, err := callWithTimeout(timeout, func() (credentials.Status, error) {
		return store.Status(context.Background()), nil
	})
	if err != nil {
		return credentials.Status{
			Backend: credentials.BackendName(),
			State:   credentials.StateUnavailable,
			Message: err.Error(),
		}
	}

	return status
}

// retryableCredentialStatus reports whether another attempt could plausibly
// succeed: a locked wallet can be unlocked, an absent secret service can be
// started, and a slow one can answer. Corrupt payloads and unsupported
// platforms will not fix themselves.
func retryableCredentialStatus(status credentials.Status) bool {
	switch status.State {
	case credentials.StateLocked, credentials.StateUnavailable:
		return true
	default:
		return false
	}
}
