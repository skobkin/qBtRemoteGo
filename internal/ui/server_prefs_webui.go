package ui

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"unicode/utf16"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// WebUI tab: the server's own HTTP interface. Several of these settings can
// cut this app off from the server, so they are marked as disconnecting and
// the login fields are checked here before the server sees them.

// disconnecting marks a field whose change can cut this app off.
func disconnecting(field *prefField) *prefField {
	field.disconnects = true
	return field
}

// qtLength counts a string the way QString::length does (UTF-16 units), which
// is what qBittorrent's length checks use.
func qtLength(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// --- WebUI username ---------------------------------------------------------------

type usernameControl struct{ textControl }

// usernameField edits the WebUI username with the server's rules, so a bad
// value never reaches setPreferences (which would fail partway through).
func usernameField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: usernameControl{}}
}

func validateWebUIUsername(username string) error {
	if qtLength(username) < 3 {
		return errors.New("must be at least 3 characters long")
	}
	if strings.Contains(username, ":") {
		return errors.New("cannot contain a colon")
	}

	return nil
}

func (c usernameControl) newEditor(f *prefField) prefEditor {
	e := c.textControl.newEditor(f).(*entryEditor)
	parse := e.parse
	e.parse = func(text string, values prefValues) error {
		if err := validateWebUIUsername(text); err != nil {
			return err
		}
		return parse(text, values)
	}

	return e
}

// --- WebUI password (write-only) ----------------------------------------------------

// loginPasswordControl edits the WebUI password. The server never returns
// it, so the field shows whenever the username key is present, an empty box
// means "keep the current password", and a sent password cannot be checked
// on re-read.
type loginPasswordControl struct{ usernameKey string }

func loginPasswordField(key, usernameKey, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: loginPasswordControl{usernameKey: usernameKey}}
}

func (loginPasswordControl) writeOnly() {}

func (c loginPasswordControl) decode(_ *prefField, prefs qbt.Preferences) (prefValues, bool) {
	if _, ok := prefs.String(c.usernameKey); !ok {
		return nil, false
	}

	return prefValues{}, true
}

func (loginPasswordControl) format(f *prefField, values prefValues) string {
	if _, ok := values[f.keys[0]]; ok {
		return "(new password)"
	}

	return "(hidden)"
}

// equivalent: the server never reports the password back.
func (loginPasswordControl) equivalent(*prefField, prefValues, prefValues) bool { return true }

func (loginPasswordControl) newEditor(f *prefField) prefEditor {
	entry := widget.NewPasswordEntry()
	entry.SetPlaceHolder("Leave empty to keep the current password")
	repeat := widget.NewPasswordEntry()
	repeat.SetPlaceHolder("Repeat the new password")

	return &loginPasswordEditor{field: f, entry: entry, repeat: repeat}
}

type loginPasswordEditor struct {
	field  *prefField
	entry  *widget.Entry
	repeat *widget.Entry
}

func (e *loginPasswordEditor) object() fyne.CanvasObject { return container.NewVBox(e.entry, e.repeat) }

// load clears both boxes: there is never a value to show.
func (e *loginPasswordEditor) load(prefValues) {
	e.entry.SetText("")
	e.repeat.SetText("")
}

func (e *loginPasswordEditor) store(values prefValues) error {
	password := e.entry.Text
	if password == "" && e.repeat.Text == "" {
		return nil
	}
	if password != e.repeat.Text {
		return fmt.Errorf("%s: the two passwords do not match", e.field.name)
	}
	if qtLength(password) < 6 {
		return fmt.Errorf("%s: must be at least 6 characters long", e.field.name)
	}
	values[e.field.keys[0]] = password

	return nil
}

func (e *loginPasswordEditor) setEnabled(enabled bool) {
	setWidgetEnabled(e.entry, enabled)
	setWidgetEnabled(e.repeat, enabled)
}

func (e *loginPasswordEditor) disabled() bool { return e.entry.Disabled() }

// --- WebUI API key (read-only, rotated by its own request) ----------------------------

type apiKeyControl struct{}

// apiKeyField shows the server's WebAPI key masked, with Copy and Rotate.
// The key is never sent with setPreferences; rotation is its own request
// that the Server settings window wires in with setRotate.
func apiKeyField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: apiKeyControl{}}
}

// maskAPIKey hides the middle of a key the way the WebUI does.
func maskAPIKey(key string) string {
	if key == "" {
		return "(none)"
	}
	if len(key) <= 10 {
		return strings.Repeat("•", len(key))
	}

	return key[:4] + strings.Repeat("•", len(key)-10) + key[len(key)-6:]
}

func (apiKeyControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeString(f, prefs)
}

func (apiKeyControl) format(f *prefField, values prefValues) string {
	key, _ := values[f.keys[0]].(string)
	return maskAPIKey(key)
}

func (apiKeyControl) newEditor(f *prefField) prefEditor {
	e := &apiKeyEditor{field: f, display: widget.NewEntry()}
	e.display.SetPlaceHolder("No key yet")
	e.display.Disable()
	e.copyButton = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		fyne.CurrentApp().Clipboard().SetContent(e.loaded)
	})
	e.rotateButton = widget.NewButton("Rotate…", func() {
		if e.rotate != nil {
			e.rotate()
		}
	})
	e.rotateButton.Disable()
	e.view = container.NewBorder(nil, nil, nil, container.NewHBox(e.copyButton, e.rotateButton), e.display)

	return e
}

type apiKeyEditor struct {
	field        *prefField
	display      *widget.Entry
	copyButton   *widget.Button
	rotateButton *widget.Button
	view         fyne.CanvasObject
	loaded       string
	rotate       func()
}

func (e *apiKeyEditor) object() fyne.CanvasObject { return e.view }

func (e *apiKeyEditor) load(values prefValues) {
	key, _ := values[e.field.keys[0]].(string)
	e.setKey(key)
}

// setKey shows a key the server reported (on load or after a rotation).
func (e *apiKeyEditor) setKey(key string) {
	e.loaded = key
	if key == "" {
		e.display.SetText("")
		e.rotateButton.SetText("Generate…")
	} else {
		e.display.SetText(maskAPIKey(key))
		e.rotateButton.SetText("Rotate…")
	}
	setWidgetEnabled(e.copyButton, key != "")
}

// setRotate wires the Rotate button to the window's rotation flow.
func (e *apiKeyEditor) setRotate(fn func()) {
	e.rotate = fn
	setWidgetEnabled(e.rotateButton, fn != nil)
}

// store reports the loaded key, so the field never counts as changed.
func (e *apiKeyEditor) store(values prefValues) error {
	values[e.field.keys[0]] = e.loaded
	return nil
}

func (e *apiKeyEditor) setEnabled(bool) {}
func (e *apiKeyEditor) disabled() bool  { return false }

// --- subnet list ------------------------------------------------------------------

// subnetListControl edits a list of IP subnets. The server splits it on
// newlines and commas, drops invalid entries and reads it back one subnet
// per line, so "not applied" compares normalized sets.
type subnetListControl struct{ textControl }

func subnetListField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: subnetListControl{textControl{multiLine: true}}}
}

func (subnetListControl) equivalent(f *prefField, a, b prefValues) bool {
	first, _ := a[f.keys[0]].(string)
	second, _ := b[f.keys[0]].(string)

	return slices.Equal(normalizeSubnetList(first), normalizeSubnetList(second))
}

func normalizeSubnetList(text string) []string {
	var entries []string
	for _, entry := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == ',' }) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			entry = prefix.Masked().String()
		} else if addr, err := netip.ParseAddr(entry); err == nil {
			entry = netip.PrefixFrom(addr, addr.BitLen()).String()
		}
		entries = append(entries, entry)
	}
	slices.Sort(entries)

	return slices.Compact(entries)
}

// --- tab ----------------------------------------------------------------------------

func dynDNSServiceOptions() []prefOption[int64] {
	return []prefOption[int64]{{0, "DynDNS"}, {1, "No-IP"}}
}

func webUIPrefTab(dirs autocompleteFetcher) prefTab {
	return prefTab{title: "WebUI", sections: []prefSection{
		{title: "Web User Interface (Remote control)",
			hint: "IP address * means all addresses. Changing the address, port or HTTPS can disconnect this app: then update the connection URL in Settings.",
			fields: []*prefField{
				disconnecting(textField("web_ui_address", "IP address", "WebUI IP address")),
				disconnecting(intRangeField("web_ui_port", "Port", "WebUI port", "", 1, 65535)),
				checkField("web_ui_upnp", "Use UPnP / NAT-PMP to forward the port from my router", "WebUI port forwarding"),
			}},
		{title: "HTTPS", hint: "Certificate and key are files on the server.", fields: []*prefField{
			disconnecting(checkField("use_https", "Use HTTPS instead of HTTP", "WebUI HTTPS")),
			disconnecting(requiring(requiredPathField("web_ui_https_cert_path", "Certificate", "HTTPS certificate", dirs), "use_https")),
			disconnecting(requiring(requiredPathField("web_ui_https_key_path", "Key", "HTTPS key", dirs), "use_https")),
		}},
		{title: "Authentication",
			hint: "The password is never shown: leave it empty to keep the current one. " +
				"Rotating the API key stops the old key at once. " +
				"Subnets: one per line or comma-separated, e.g. 172.17.32.0/24, fdff:ffff:c8::/40. " +
				"0 failures: never ban. Sessions count limit 0: unlimited.",
			fields: []*prefField{
				usernameField("web_ui_username", "Username", "WebUI username"),
				loginPasswordField("web_ui_password", "web_ui_username", "Password", "WebUI password"),
				apiKeyField("web_ui_api_key", "API key", "WebUI API key"),
				checkField("bypass_local_auth", "Bypass authentication for clients on localhost", "Bypass authentication on localhost"),
				checkField("bypass_auth_subnet_whitelist_enabled", "Bypass authentication for clients in whitelisted IP subnets",
					"Bypass authentication for whitelisted subnets"),
				requiring(subnetListField("bypass_auth_subnet_whitelist", "Whitelisted subnets", "Authentication bypass subnets"),
					"bypass_auth_subnet_whitelist_enabled"),
				intField("web_ui_max_auth_fail_count", "Ban client after consecutive failures", "Ban after failures", "", 0),
				intField("web_ui_ban_duration", "Ban for (s)", "Ban duration", "s", 1),
				intField("web_ui_session_timeout", "Session timeout (s)", "Session timeout", "s", 0),
				intField("web_ui_sessions_count_limit", "Sessions count limit", "Sessions count limit", "", 0),
			}},
		{title: "Use alternative WebUI", fields: []*prefField{
			checkField("alternative_webui_enabled", "Use alternative WebUI", "Alternative WebUI"),
			requiring(requiredPathField("alternative_webui_path", "Files location", "Alternative WebUI location", dirs), "alternative_webui_enabled"),
		}},
		{title: "Security",
			hint: "Server domains: ';'-separated, * matches any. If Host header validation rejects the host name this app connects to, the app loses access.",
			fields: []*prefField{
				checkField("web_ui_clickjacking_protection_enabled", "Enable clickjacking protection", "Clickjacking protection"),
				checkField("web_ui_csrf_protection_enabled", "Enable Cross-Site Request Forgery (CSRF) protection", "CSRF protection"),
				checkField("web_ui_secure_cookie_enabled", "Enable cookie Secure flag (requires HTTPS or localhost connection)", "Secure cookie flag"),
				disconnecting(checkField("web_ui_host_header_validation_enabled", "Enable Host header validation", "Host header validation")),
				disconnecting(requiring(textField("web_ui_domain_list", "Server domains", "Server domains"), "web_ui_host_header_validation_enabled")),
			}},
		{title: "Custom HTTP headers", hint: "Header: value pairs, one per line.", fields: []*prefField{
			checkField("web_ui_use_custom_http_headers_enabled", "Add custom HTTP headers", "Custom HTTP headers"),
			requiring(textAreaField("web_ui_custom_http_headers", "Headers", "Custom HTTP header lines"), "web_ui_use_custom_http_headers_enabled"),
		}},
		{title: "Reverse proxy", hint: "Trusted proxies: IPs or subnets, ';'-separated.", fields: []*prefField{
			checkField("web_ui_reverse_proxy_enabled", "Enable reverse proxy support", "Reverse proxy support"),
			requiring(textField("web_ui_reverse_proxies_list", "Trusted proxies list", "Trusted reverse proxies"), "web_ui_reverse_proxy_enabled"),
		}},
		{title: "Update my dynamic domain name", fields: []*prefField{
			checkField("dyndns_enabled", "Update my dynamic domain name", "Dynamic DNS"),
			requiring(enumField("dyndns_service", "Service", "Dynamic DNS service", dynDNSServiceOptions()), "dyndns_enabled"),
			requiring(textField("dyndns_domain", "Domain name", "Dynamic DNS domain"), "dyndns_enabled"),
			requiring(textField("dyndns_username", "Username", "Dynamic DNS username"), "dyndns_enabled"),
			requiring(passwordField("dyndns_password", "Dynamic DNS password"), "dyndns_enabled"),
		}},
	}}
}
