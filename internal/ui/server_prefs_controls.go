package ui

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/skobkin/qbtremotego/internal/qbt"
)

// More prefControl kinds: bounded integers, "-1 = unlimited" counts,
// checkbox-gated limits, free text and read-only text.

// maxPrefInt is the largest integer qBittorrent stores for most numeric
// preferences (they are C++ ints).
const maxPrefInt = math.MaxInt32

func parsePrefInt(text string, minValue int64) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value < minValue || value > maxPrefInt {
		return 0, fmt.Errorf("enter a whole number from %d to %d", minValue, maxPrefInt)
	}

	return value, nil
}

func withUnit(value, unit string) string {
	if unit == "" {
		return value
	}

	return value + " " + unit
}

// --- bounded integer ----------------------------------------------------------

type intControl struct {
	unit     string
	minValue int64
}

// intField edits a whole number of at least minValue; unit is shown in
// summaries ("5 KiB/s") and as the entry placeholder.
func intField(key, label, name, unit string, minValue int64) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: intControl{unit: unit, minValue: minValue}}
}

func (intControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeInts(f.keys, prefs)
}

func (c intControl) format(f *prefField, values prefValues) string {
	value, _ := values[f.keys[0]].(int64)
	return withUnit(strconv.FormatInt(value, 10), c.unit)
}

func (c intControl) newEditor(f *prefField) prefEditor {
	key := f.keys[0]
	show := func(values prefValues) string {
		value, _ := values[key].(int64)
		return strconv.FormatInt(value, 10)
	}
	parse := func(text string, values prefValues) error {
		value, err := parsePrefInt(text, c.minValue)
		if err != nil {
			return err
		}
		values[key] = value

		return nil
	}

	return newEntryEditor(f, c.unit, show, parse)
}

// --- count with "Unlimited" (-1) ------------------------------------------------

type unlimitedControl struct{}

// unlimitedIntField edits a count where the server uses -1 for unlimited, as
// an "Unlimited" checkbox next to the number.
func unlimitedIntField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: unlimitedControl{}}
}

func (unlimitedControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeInts(f.keys, prefs)
}

func (unlimitedControl) format(f *prefField, values prefValues) string {
	value, _ := values[f.keys[0]].(int64)
	if value < 0 {
		return "Unlimited"
	}

	return strconv.FormatInt(value, 10)
}

func (unlimitedControl) newEditor(f *prefField) prefEditor {
	e := &unlimitedEditor{field: f, entry: widget.NewEntry(), enabled: true}
	e.unlimited = widget.NewCheck("Unlimited", func(bool) { e.syncEntry() })
	e.box = container.NewBorder(nil, nil, nil, e.unlimited, e.entry)

	return e
}

type unlimitedEditor struct {
	field     *prefField
	unlimited *widget.Check
	entry     *widget.Entry
	box       fyne.CanvasObject
	enabled   bool
}

func (e *unlimitedEditor) object() fyne.CanvasObject { return e.box }

func (e *unlimitedEditor) load(values prefValues) {
	value, _ := values[e.field.keys[0]].(int64)
	if value < 0 {
		e.unlimited.SetChecked(true)
		e.entry.SetText("")
	} else {
		e.unlimited.SetChecked(false)
		e.entry.SetText(strconv.FormatInt(value, 10))
	}
	e.syncEntry()
}

func (e *unlimitedEditor) store(values prefValues) error {
	if e.unlimited.Checked {
		values[e.field.keys[0]] = int64(-1)
		return nil
	}
	value, err := parsePrefInt(e.entry.Text, 0)
	if err != nil {
		return fmt.Errorf("%s: %w, or tick Unlimited", e.field.name, err)
	}
	values[e.field.keys[0]] = value

	return nil
}

func (e *unlimitedEditor) setEnabled(enabled bool) {
	e.enabled = enabled
	setWidgetEnabled(e.unlimited, enabled)
	e.syncEntry()
}

func (e *unlimitedEditor) disabled() bool { return !e.enabled }

func (e *unlimitedEditor) syncEntry() {
	setWidgetEnabled(e.entry, e.enabled && !e.unlimited.Checked)
}

// --- checkbox-gated limit (seeding limits) ------------------------------------------

// optionalLimitControl edits an "<x>_enabled" switch and its value as one
// field. The server only applies the value together with the switch, and
// reports a disabled limit's value as -1.
type optionalLimitControl struct {
	ratio   bool   // float value (share ratio) instead of an integer
	unit    string // shown after the value
	offText string // shown in the greyed entry while the limit is off
}

func optionalRatioField(enabledKey, valueKey, label, name string) *prefField {
	return &prefField{keys: []string{enabledKey, valueKey}, label: label, name: name,
		control: optionalLimitControl{ratio: true, offText: "1"}}
}

func optionalMinutesField(enabledKey, valueKey, label, name string) *prefField {
	return &prefField{keys: []string{enabledKey, valueKey}, label: label, name: name,
		control: optionalLimitControl{unit: "min", offText: "1440"}}
}

func (c optionalLimitControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	enabled, ok := prefs.Bool(f.keys[0])
	if !ok {
		return nil, false
	}
	var value any
	if c.ratio {
		value, ok = prefs.Float(f.keys[1])
	} else {
		value, ok = prefs.Int(f.keys[1])
	}
	if !ok {
		return nil, false
	}

	return prefValues{f.keys[0]: enabled, f.keys[1]: value}, true
}

func (c optionalLimitControl) format(f *prefField, values prefValues) string {
	if on, _ := values[f.keys[0]].(bool); !on {
		return "Off"
	}

	return withUnit(c.formatValue(values[f.keys[1]]), c.unit)
}

func (c optionalLimitControl) formatValue(value any) string {
	if c.ratio {
		ratio, _ := value.(float64)
		return strconv.FormatFloat(ratio, 'f', -1, 64)
	}
	minutes, _ := value.(int64)

	return strconv.FormatInt(minutes, 10)
}

// offValue is what the WebUI sends for a disabled limit.
func (c optionalLimitControl) offValue() any {
	if c.ratio {
		return float64(-1)
	}

	return int64(-1)
}

func (c optionalLimitControl) parse(text string) (any, error) {
	if !c.ratio {
		return parsePrefInt(text, 0)
	}
	ratio, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return nil, errors.New("enter a ratio of 0 or more, e.g. 1.5")
	}

	return ratio, nil
}

func (c optionalLimitControl) newEditor(f *prefField) prefEditor {
	e := &optionalLimitEditor{field: f, control: c, entry: widget.NewEntry(), enabled: true}
	e.check = widget.NewCheck("", func(bool) { e.syncEntry() })
	var unit fyne.CanvasObject
	if c.unit != "" {
		unit = widget.NewLabel(c.unit)
	}
	e.box = container.NewBorder(nil, nil, e.check, unit, e.entry)

	return e
}

type optionalLimitEditor struct {
	field   *prefField
	control optionalLimitControl
	check   *widget.Check
	entry   *widget.Entry
	box     fyne.CanvasObject
	enabled bool
	loaded  prefValues
}

func (e *optionalLimitEditor) object() fyne.CanvasObject { return e.box }

func (e *optionalLimitEditor) load(values prefValues) {
	enabledKey, valueKey := e.field.keys[0], e.field.keys[1]
	e.loaded = prefValues{enabledKey: values[enabledKey], valueKey: values[valueKey]}
	on, _ := values[enabledKey].(bool)
	e.check.SetChecked(on)
	if on {
		e.entry.SetText(e.control.formatValue(values[valueKey]))
	} else {
		e.entry.SetText(e.control.offText)
	}
	e.syncEntry()
}

func (e *optionalLimitEditor) store(values prefValues) error {
	enabledKey, valueKey := e.field.keys[0], e.field.keys[1]
	if !e.check.Checked {
		values[enabledKey] = false
		// Keep the server's value for a limit that was already off, so an
		// untouched field never shows up as a change.
		if wasOn, _ := e.loaded[enabledKey].(bool); !wasOn && e.loaded[valueKey] != nil {
			values[valueKey] = e.loaded[valueKey]
		} else {
			values[valueKey] = e.control.offValue()
		}
		return nil
	}
	value, err := e.control.parse(e.entry.Text)
	if err != nil {
		return fmt.Errorf("%s: %w", e.field.name, err)
	}
	values[enabledKey] = true
	values[valueKey] = value

	return nil
}

func (e *optionalLimitEditor) setEnabled(enabled bool) {
	e.enabled = enabled
	setWidgetEnabled(e.check, enabled)
	e.syncEntry()
}

func (e *optionalLimitEditor) disabled() bool { return !e.enabled }

func (e *optionalLimitEditor) syncEntry() {
	setWidgetEnabled(e.entry, e.enabled && e.check.Checked)
}

// --- text -----------------------------------------------------------------------------

type textControl struct{ multiLine bool }

// textField edits a single-line string such as a URL.
func textField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: textControl{}}
}

// textAreaField edits a multi-line string such as a tracker list.
func textAreaField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: textControl{multiLine: true}}
}

func decodeString(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	value, ok := prefs.String(f.keys[0])
	if !ok {
		return nil, false
	}

	return prefValues{f.keys[0]: value}, true
}

// formatText keeps summaries short: multi-line values are counted, not shown.
func formatText(text string, multiLine bool) string {
	if text == "" {
		return "(empty)"
	}
	if !multiLine {
		return strconv.Quote(text)
	}
	if lines := len(strings.Split(strings.TrimRight(text, "\n"), "\n")); lines != 1 {
		return fmt.Sprintf("%d lines", lines)
	}

	return "1 line"
}

func (textControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeString(f, prefs)
}

func (c textControl) format(f *prefField, values prefValues) string {
	text, _ := values[f.keys[0]].(string)
	return formatText(text, c.multiLine)
}

func (c textControl) newEditor(f *prefField) prefEditor {
	key := f.keys[0]
	show := func(values prefValues) string {
		text, _ := values[key].(string)
		return text
	}
	// Text is sent as typed: trimming would turn untouched values into changes.
	parse := func(text string, values prefValues) error {
		values[key] = text
		return nil
	}
	e := newEntryEditor(f, "", show, parse)
	if c.multiLine {
		e.entry.MultiLine = true
		e.entry.Wrapping = fyne.TextWrapOff
		e.entry.SetMinRowsVisible(5)
	}

	return e
}

// --- read-only text -----------------------------------------------------------------

type readOnlyTextControl struct{}

// readOnlyTextField shows a server-computed value; it is never sent.
func readOnlyTextField(key, label, name string) *prefField {
	return &prefField{keys: []string{key}, label: label, name: name, control: readOnlyTextControl{}}
}

func (readOnlyTextControl) decode(f *prefField, prefs qbt.Preferences) (prefValues, bool) {
	return decodeString(f, prefs)
}

func (readOnlyTextControl) format(f *prefField, values prefValues) string {
	text, _ := values[f.keys[0]].(string)
	return formatText(text, true)
}

func (readOnlyTextControl) newEditor(f *prefField) prefEditor {
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetMinRowsVisible(5)
	entry.Disable()

	return &readOnlyTextEditor{field: f, entry: entry}
}

type readOnlyTextEditor struct {
	field  *prefField
	entry  *widget.Entry
	loaded string
}

func (e *readOnlyTextEditor) object() fyne.CanvasObject { return e.entry }

func (e *readOnlyTextEditor) load(values prefValues) {
	e.loaded, _ = values[e.field.keys[0]].(string)
	e.entry.SetText(e.loaded)
}

// store reports the loaded value, so the field never counts as changed.
func (e *readOnlyTextEditor) store(values prefValues) error {
	values[e.field.keys[0]] = e.loaded
	return nil
}

func (e *readOnlyTextEditor) setEnabled(bool) {}
func (e *readOnlyTextEditor) disabled() bool  { return true }
