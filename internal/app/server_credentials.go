package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/skobkin/qbtremotego/internal/config"
	"github.com/skobkin/qbtremotego/internal/credentials"
)

// LoginMethod reports how the app logs in to the server.
func (c *Controller) LoginMethod() config.AuthMethod {
	return c.Config().Connection.AuthMethod
}

// RotateAPIKey makes the server generate a new WebAPI key; the old one stops
// working at once. When the app itself logs in with the API key, the new key
// becomes the session's and replaces the saved one. A non-empty key with an
// error means the server rotated it but saving it failed: the session still
// uses it, and the caller must show it so it can be entered in Settings.
func (c *Controller) RotateAPIKey(ctx context.Context) (string, error) {
	client, err := c.client()
	if err != nil {
		return "", err
	}

	logger := c.currentLogger()
	remoteURL := c.Config().Connection.URL
	key, err := client.RotateAPIKey(ctx)
	if err != nil {
		logger.Warn("rotate server API key failed", "remote_url", remoteURL, "error", err)
		return "", err
	}
	logger.Info("rotated server API key", "remote_url", remoteURL)

	if c.LoginMethod() != config.AuthMethodAPIKey {
		return key, nil
	}
	if err := c.replaceSavedCredentials(ctx, credentials.Credentials{APIKey: key}); err != nil {
		logger.Warn("save rotated API key failed", "remote_url", remoteURL, "error", err)
		return key, err
	}

	return key, nil
}

// UpdateLoginCredentials replaces the saved login after the server's WebUI
// username or password changed. An empty value keeps the current one.
func (c *Controller) UpdateLoginCredentials(ctx context.Context, username, password string) error {
	if c.LoginMethod() != config.AuthMethodPassword {
		return errors.New("the app does not log in with a username and password")
	}

	creds := c.SessionCredentials()
	if username != "" {
		creds.Username = username
	}
	if password != "" {
		creds.Password = password
	}
	if err := c.replaceSavedCredentials(ctx, credentials.Credentials{Username: creds.Username, Password: creds.Password}); err != nil {
		c.currentLogger().Warn("save updated login failed", "remote_url", c.Config().Connection.URL, "error", err)
		return err
	}

	return nil
}

// replaceSavedCredentials switches the session to creds and saves them where
// the current ones are kept, without changing the storage mode. The session
// switches first: the server already expects the new credentials.
func (c *Controller) replaceSavedCredentials(ctx context.Context, creds credentials.Credentials) error {
	c.setSessionCredentials(creds)

	switch c.Config().Connection.CredentialStorage {
	case config.CredentialStorageKeychain:
		if err := c.credentialStore.Set(ctx, creds); err != nil {
			c.setCredentialStatus(c.statusFromError(err))
			return fmt.Errorf("save the new credentials to the keychain: %w", err)
		}
	case config.CredentialStoragePlaintext:
		if _, err := c.persistConfig(false, func(dst *config.AppConfig) {
			dst.Connection.Username = creds.Username
			dst.Connection.Password = creds.Password
			dst.Connection.APIKey = creds.APIKey
		}); err != nil {
			return fmt.Errorf("save the new credentials to the config file: %w", err)
		}
	default:
		// Session-only credentials are never persisted.
	}

	return nil
}
