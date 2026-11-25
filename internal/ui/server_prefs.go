package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// This file is the declarative core of the Server settings window: each server
// preference shown in the UI is a prefField (wire key(s), label, control). The
// form, diffing, conflict and verification logic work on prefValues, the
// decoded wire values, so adding a settings tab only means adding descriptors.
// Key semantics are documented in docs/qbittorrent/.

const unsupportedTabText = "Not supported by this server version."

// maxRateLimitKiB is the largest limit qBittorrent can store: limits travel
// as int32 bytes/s and are kept in whole KiB.
const maxRateLimitKiB = 2147483647 / 1024

// prefValues holds decoded preference values keyed by wire key. Values are
// bool, int64, float64 or string, so they compare with ==.
type prefValues map[string]any

// prefField describes one control in the Server settings window.
type prefField struct {
	// keys are the wire keys the control reads and writes. Multi-key fields
	// (e.g. a schedule time's hour+minute) are always sent as a whole, which
	// the server requires for paired keys.
	keys []string
	// label is the form label; name identifies the field in confirmations,
	// reports and validation errors.
	label string
	name  string
	// requires names boolean keys whose controls must all be on for this
	// field to be editable (mirrors the WebUI's nested checkboxes).
	requires []string
	control  prefControl
}

func (f *prefField) format(values prefValues) string {
	for _, key := range f.keys {
		if _, ok := values[key]; !ok {
			return "(not set)"
		}
	}

	return f.control.format(f, values)
}

// prefControl is a kind of input: how its keys decode from the server, how
// values are shown, and which editor widget edits them.
type prefControl interface {
	decode(f *prefField, prefs qbt.Preferences) (prefValues, bool)
	format(f *prefField, values prefValues) string
	newEditor(f *prefField) prefEditor
}

// prefEditor is a live widget for one field.
type prefEditor interface {
	object() fyne.CanvasObject
	load(values prefValues)
	// store validates the widget and writes the field's wire values.
	store(values prefValues) error
	setEnabled(enabled bool)
	disabled() bool
}

func decodeInts(keys []string, prefs qbt.Preferences) (prefValues, bool) {
	values := prefValues{}
	for _, key := range keys {
		v, ok := prefs.Int(key)
		if !ok {
			return nil, false
		}
		values[key] = v
	}

	return values, true
}

// --- checkbox -------------------------------------------------------------

type checkControl struct{}

func checkField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: checkControl{}}
}

func (checkControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	v, ok := prefs.Bool(f.keys[0])
	if !ok {
		return nil, false
	}

	return prefValues{f.keys[0]: v}, true
}

func (checkControl) format(f *prefField, values prefValues) string {
	if on, _ := values[f.keys[0]].(bool); on {
		return "On"
	}

	return "Off"
}

func (checkControl) newEditor(f *prefField) prefEditor {
	return &checkEditor{field: f, check: widget.NewCheck("", nil)}
}

type checkEditor struct {
	field *prefField
	check *widget.Check
}

func (e *checkEditor) object() fyne.CanvasObject { return e.check }

func (e *checkEditor) load(values prefValues) {
	on, _ := values[e.field.keys[0]].(bool)
	e.check.SetChecked(on)
}

func (e *checkEditor) store(values prefValues) error {
	values[e.field.keys[0]] = e.check.Checked
	return nil
}

func (e *checkEditor) setEnabled(enabled bool) { setWidgetEnabled(e.check, enabled) }
func (e *checkEditor) disabled() bool          { return e.check.Disabled() }

// --- text entries (rate limits, times) ---------------------------------------

// entryEditor edits a field through a single text entry using a parse/show
// pair supplied by the control.
type entryEditor struct {
	field *prefField
	entry *widget.Entry
	show  func(values prefValues) string
	parse func(text string, values prefValues) error
}

func (e *entryEditor) object() fyne.CanvasObject { return e.entry }

func (e *entryEditor) load(values prefValues) { e.entry.SetText(e.show(values)) }

func (e *entryEditor) store(values prefValues) error {
	if err := e.parse(e.entry.Text, values); err != nil {
		return fmt.Errorf("%s: %w", e.field.name, err)
	}

	return nil
}

func (e *entryEditor) setEnabled(enabled bool) { setWidgetEnabled(e.entry, enabled) }
func (e *entryEditor) disabled() bool          { return e.entry.Disabled() }

func newEntryEditor(f *prefField, placeholder string, show func(prefValues) string, parse func(string, prefValues) error) *entryEditor {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(placeholder)
	entry.Validator = func(text string) error { return parse(text, prefValues{}) }

	return &entryEditor{field: f, entry: entry, show: show, parse: parse}
}

// rateLimitControl edits a bytes/s limit in KiB/s; 0 means unlimited.
type rateLimitControl struct{}

func rateLimitField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: rateLimitControl{}}
}

func (rateLimitControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeInts(f.keys, prefs)
}

func (rateLimitControl) format(f *prefField, values prefValues) string {
	bytes, _ := values[f.keys[0]].(int64)
	if bytes <= 0 {
		return "Unlimited"
	}

	return fmt.Sprintf("%d KiB/s", bytes/1024)
}

func (rateLimitControl) newEditor(f *prefField) prefEditor {
	key := f.keys[0]
	show := func(values prefValues) string {
		bytes, _ := values[key].(int64)
		return strconv.FormatInt(bytes/1024, 10)
	}
	parse := func(text string, values prefValues) error {
		kib, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil || kib < 0 {
			return errors.New("enter a whole number of KiB/s (0 = unlimited)")
		}
		if kib > maxRateLimitKiB {
			return fmt.Errorf("the maximum is %d KiB/s", maxRateLimitKiB)
		}
		values[key] = kib * 1024

		return nil
	}

	return newEntryEditor(f, "KiB/s, 0 = unlimited", show, parse)
}

// timeControl edits an hour+minute key pair as HH:MM.
type timeControl struct{}

func timeField(hourKey, minuteKey, label, name string) *prefField {
	return &prefField{keys: []string{hourKey, minuteKey}, label: label, name: name, control: timeControl{}}
}

func (timeControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeInts(f.keys, prefs)
}

func (timeControl) format(f *prefField, values prefValues) string {
	hour, _ := values[f.keys[0]].(int64)
	minute, _ := values[f.keys[1]].(int64)

	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func (c timeControl) newEditor(f *prefField) prefEditor {
	show := func(values prefValues) string { return c.format(f, values) }
	parse := func(text string, values prefValues) error {
		hourText, minuteText, ok := strings.Cut(strings.TrimSpace(text), ":")
		hour, hourErr := strconv.Atoi(hourText)
		minute, minuteErr := strconv.Atoi(minuteText)
		if !ok || hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return errors.New("enter a time as HH:MM (00:00–23:59)")
		}
		values[f.keys[0]] = int64(hour)
		values[f.keys[1]] = int64(minute)

		return nil
	}

	return newEntryEditor(f, "HH:MM", show, parse)
}

// --- enum select ------------------------------------------------------------

// prefOption is one choice of an enum preference; T is int64 or string,
// matching the wire type.
type prefOption[T comparable] struct {
	value T
	label string
}

type enumControl[T comparable] struct {
	options     []prefOption[T]
	decodeValue func(prefs qbt.Preferences, key string) (T, bool)
}

// enumField is a select over an integer-coded preference.
func enumField(key, label, name string, options []prefOption[int64]) *prefField {
	control := enumControl[int64]{options: options, decodeValue: qbt.Preferences.Int}
	return &prefField{keys: []string{key}, label: label, name: name, control: control}
}

// textEnumField is a select over a string-coded preference.
func textEnumField(key, label, name string, options []prefOption[string]) *prefField {
	control := enumControl[string]{options: options, decodeValue: qbt.Preferences.String}
	return &prefField{keys: []string{key}, label: label, name: name, control: control}
}

func (c enumControl[T]) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	value, ok := c.decodeValue(prefs, f.keys[0])
	if !ok {
		return nil, false
	}

	return prefValues{f.keys[0]: value}, true
}

func (c enumControl[T]) format(f *prefField, values prefValues) string {
	value, _ := values[f.keys[0]].(T)
	for _, option := range c.options {
		if option.value == value {
			return option.label
		}
	}

	return fmt.Sprint(value)
}

func (c enumControl[T]) newEditor(f *prefField) prefEditor {
	labels := make([]string, 0, len(c.options))
	for _, option := range c.options {
		labels = append(labels, option.label)
	}

	return &enumEditor[T]{field: f, options: c.options, selector: widget.NewSelect(labels, nil)}
}

// enumEditor keeps a server value it has no label for unchanged until the
// user picks an option, so unknown values from newer servers survive.
type enumEditor[T comparable] struct {
	field    *prefField
	options  []prefOption[T]
	selector *widget.Select
	loaded   T
}

func (e *enumEditor[T]) object() fyne.CanvasObject { return e.selector }

func (e *enumEditor[T]) load(values prefValues) {
	e.loaded, _ = values[e.field.keys[0]].(T)
	e.selector.ClearSelected()
	e.selector.PlaceHolder = fmt.Sprint(e.loaded)
	for _, option := range e.options {
		if option.value == e.loaded {
			e.selector.SetSelected(option.label)
			break
		}
	}
	e.selector.Refresh()
}

func (e *enumEditor[T]) store(values prefValues) error {
	value := e.loaded
	for _, option := range e.options {
		if option.label == e.selector.Selected {
			value = option.value
			break
		}
	}
	values[e.field.keys[0]] = value

	return nil
}

func (e *enumEditor[T]) setEnabled(enabled bool) { setWidgetEnabled(e.selector, enabled) }
func (e *enumEditor[T]) disabled() bool          { return e.selector.Disabled() }

type disableable interface {
	Enable()
	Disable()
}

func setWidgetEnabled(w disableable, enabled bool) {
	if enabled {
		w.Enable()
	} else {
		w.Disable()
	}
}

// --- diffing ----------------------------------------------------------------

// presentFields keeps the fields whose keys the server returned with the
// expected types (other fields are unsupported by this server version) and
// decodes their values.
func presentFields(fields []*prefField, prefs qbt.Preferences) ([]*prefField, prefValues) {
	var present []*prefField
	values := prefValues{}
	for _, field := range fields {
		decoded, ok := field.control.decode(field, prefs)
		if !ok {
			continue
		}
		present = append(present, field)
		for key, value := range decoded {
			values[key] = value
		}
	}

	return present, values
}

// decodePrefValues decodes the given fields from a fresh server read; keys
// that are missing or mistyped are left out.
func decodePrefValues(fields []*prefField, prefs qbt.Preferences) prefValues {
	values := prefValues{}
	for _, field := range fields {
		if decoded, ok := field.control.decode(field, prefs); ok {
			for key, value := range decoded {
				values[key] = value
			}
		}
	}

	return values
}

func fieldDiffers(field *prefField, a, b prefValues) bool {
	for _, key := range field.keys {
		av, aok := a[key]
		bv, bok := b[key]
		if aok != bok || av != bv {
			return true
		}
	}

	return false
}

func filterFields(fields []*prefField, keep func(*prefField) bool) []*prefField {
	var out []*prefField
	for _, field := range fields {
		if keep(field) {
			out = append(out, field)
		}
	}

	return out
}

// changedFields lists the fields the user edited.
func changedFields(fields []*prefField, baseline, edited prefValues) []*prefField {
	return filterFields(fields, func(f *prefField) bool { return fieldDiffers(f, baseline, edited) })
}

// changesPayload is the setPreferences body: every key of every changed
// field, so paired keys always travel together.
func changesPayload(changed []*prefField, edited prefValues) map[string]any {
	payload := map[string]any{}
	for _, field := range changed {
		for _, key := range field.keys {
			payload[key] = edited[key]
		}
	}

	return payload
}

// conflictingFields lists edited fields whose server value changed since the
// window loaded it (someone else edited them meanwhile).
func conflictingFields(changed []*prefField, baseline, fresh prefValues) []*prefField {
	return filterFields(changed, func(f *prefField) bool { return fieldDiffers(f, baseline, fresh) })
}

// unappliedFields lists sent fields the server does not report back as sent.
func unappliedFields(sent []*prefField, sentValues, reread prefValues) []*prefField {
	return filterFields(sent, func(f *prefField) bool { return fieldDiffers(f, sentValues, reread) })
}

// --- form -------------------------------------------------------------------

type prefSection struct {
	title  string
	hint   string
	fields []*prefField
}

type prefTab struct {
	title    string
	sections []prefSection
}

// prefForm is the editable view of the server preferences: one tab per
// prefTab, showing only the fields this server supports.
type prefForm struct {
	fields   []*prefField
	editors  map[*prefField]prefEditor
	baseline prefValues
	tabs     *container.AppTabs
}

func newPrefForm(tabs []prefTab, prefs qbt.Preferences) *prefForm {
	form := &prefForm{editors: map[*prefField]prefEditor{}, tabs: container.NewAppTabs()}

	var all []*prefField
	for _, tab := range tabs {
		for _, section := range tab.sections {
			all = append(all, section.fields...)
		}
	}
	form.fields, form.baseline = presentFields(all, prefs)
	supported := map[*prefField]bool{}
	for _, field := range form.fields {
		supported[field] = true
	}

	for _, tab := range tabs {
		var sections []fyne.CanvasObject
		for _, section := range tab.sections {
			items := form.sectionItems(section, supported)
			if len(items) == 0 {
				continue
			}
			content := []fyne.CanvasObject{widget.NewForm(items...)}
			if section.hint != "" {
				hint := widget.NewLabelWithStyle(section.hint, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
				content = append(content, hint)
			}
			if len(sections) > 0 {
				sections = append(sections, widget.NewSeparator())
			}
			sections = append(sections, newSettingsSection(section.title, content...))
		}

		var body fyne.CanvasObject
		if len(sections) == 0 {
			body = container.NewPadded(widget.NewLabel(unsupportedTabText))
		} else {
			body = container.NewVScroll(container.NewVBox(sections...))
		}
		form.tabs.Append(container.NewTabItem(tab.title, body))
	}

	form.load(form.baseline)

	return form
}

func (f *prefForm) sectionItems(section prefSection, supported map[*prefField]bool) []*widget.FormItem {
	var items []*widget.FormItem
	for _, field := range section.fields {
		if !supported[field] {
			continue
		}
		editor := field.control.newEditor(field)
		f.editors[field] = editor
		if check, ok := editor.(*checkEditor); ok {
			check.check.OnChanged = func(bool) { f.syncEnablement() }
		}
		items = append(items, widget.NewFormItem(field.label, editor.object()))
	}

	return items
}

// load shows values in every editor.
func (f *prefForm) load(values prefValues) {
	for _, field := range f.fields {
		f.editors[field].load(values)
	}
	f.syncEnablement()
}

// edited validates every editor and returns the values they hold. The first
// invalid field's error names it.
func (f *prefForm) edited() (prefValues, error) {
	values := prefValues{}
	for _, field := range f.fields {
		if err := f.editors[field].store(values); err != nil {
			return nil, err
		}
	}

	return values, nil
}

func (f *prefForm) fieldByKey(key string) *prefField {
	for _, field := range f.fields {
		for _, k := range field.keys {
			if k == key {
				return field
			}
		}
	}

	return nil
}

// syncEnablement enables fields whose required switches are all on; a switch
// this server does not report counts as on.
func (f *prefForm) syncEnablement() {
	for _, field := range f.fields {
		if len(field.requires) == 0 {
			continue
		}
		enabled := true
		for _, key := range field.requires {
			switchField := f.fieldByKey(key)
			if switchField == nil {
				continue
			}
			if check, ok := f.editors[switchField].(*checkEditor); ok && !check.check.Checked {
				enabled = false
			}
		}
		f.editors[field].setEnabled(enabled)
	}
}
