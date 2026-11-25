package qbt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// Preferences is the raw app/preferences object. Values stay undecoded so keys
// the app does not know survive untouched, key presence tells which settings
// this server version supports, and large integers are not rounded through
// float64. See docs/qbittorrent/ for the key reference.
type Preferences map[string]json.RawMessage

// Has reports whether the server returned key.
func (p Preferences) Has(key string) bool {
	_, ok := p[key]
	return ok
}

// Bool returns a boolean preference; ok is false when the key is absent or
// not a JSON boolean.
func (p Preferences) Bool(key string) (value bool, ok bool) {
	raw, present := p[key]
	if !present {
		return false, false
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}

	return value, true
}

// Int returns an integer preference; ok is false when the key is absent or
// not a whole JSON number.
func (p Preferences) Int(key string) (value int64, ok bool) {
	number, ok := p.number(key)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil {
		return 0, false
	}

	return value, true
}

// Float returns a numeric preference; ok is false when the key is absent or
// not a JSON number.
func (p Preferences) Float(key string) (value float64, ok bool) {
	number, ok := p.number(key)
	if !ok {
		return 0, false
	}
	value, err := number.Float64()
	if err != nil {
		return 0, false
	}

	return value, true
}

// String returns a string preference; ok is false when the key is absent or
// not a JSON string.
func (p Preferences) String(key string) (value string, ok bool) {
	raw, present := p[key]
	if !present {
		return "", false
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}

	return value, true
}

func (p Preferences) number(key string) (json.Number, bool) {
	raw, present := p[key]
	if !present {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	number, ok := value.(json.Number)

	return number, ok
}

// Preferences reads all application preferences of the server.
func (c *Client) Preferences(ctx context.Context) (Preferences, error) {
	var prefs Preferences
	if err := c.getJSON(ctx, "app/preferences", nil, &prefs); err != nil {
		return nil, err
	}

	return prefs, nil
}

// SetPreferences sends changed preferences to the server. Only the given keys
// are sent. The server applies them one by one and silently ignores unknown
// keys; a 400 (e.g. a rejected WebUI username) can leave earlier keys applied,
// so callers should re-read the preferences afterwards.
func (c *Client) SetPreferences(ctx context.Context, changes map[string]any) error {
	if len(changes) == 0 {
		return errors.New("no preference changes to send")
	}
	payload, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("encode preference changes: %w", err)
	}

	form := url.Values{}
	form.Set("json", string(payload))

	return c.postForm(ctx, "app/setPreferences", form)
}
