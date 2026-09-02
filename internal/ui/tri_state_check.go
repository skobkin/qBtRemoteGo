package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// checkState is the inclusion state shown by the Content tab's checkbox
// column: whether a file or folder takes part in the download.
type checkState int

const (
	checkStateUnchecked checkState = iota
	checkStateChecked
	checkStateMixed
)

// triStateCheck renders all three inclusion states with the native checkbox
// glyphs: widget.Check gained the indeterminate Partial state in Fyne 2.6.
//
// The column is read-only for now. Making it interactive later only requires
// wiring Tapped: the mixed state is stored as (Checked=false, Partial=true),
// so a plain SetChecked(!Checked) already yields the intended cycle
// mixed -> checked -> unchecked -> checked.
type triStateCheck struct {
	widget.Check
	// row receives hover events so the row tint survives hovering the
	// checkbox, like the row's other hover-capable cells.
	row hoverTarget
	// onTapped is nil while the checkbox is read-only; when file priority
	// toggling arrives, it receives the new inclusion state.
	onTapped func(next bool)
}

func newTriStateCheck(row hoverTarget) *triStateCheck {
	c := &triStateCheck{row: row}
	// Claim the embedded Check's base before the first renderer is created:
	// the Check would otherwise self-extend, and the promoted calls (Refresh,
	// renderer caching, the canvas repaint fired by its renderer) would
	// resolve through a widget the canvas never paints. ExtendBaseWidget
	// ignores later re-assignment, so this sticks.
	c.ExtendBaseWidget(c)
	return c
}

// SetState renders the given inclusion state. The fields are set directly:
// SetChecked would clear Partial and fire OnChanged. Rows are recycled and
// re-bound on every poll tick, so the unchanged case is the common one.
func (c *triStateCheck) SetState(state checkState) {
	checked := state == checkStateChecked
	partial := state == checkStateMixed
	if c.Checked == checked && c.Partial == partial {
		return
	}
	c.Checked = checked
	c.Partial = partial
	c.Refresh()
}

// Tapped is read-only for now: nothing assigns onTapped, so taps do nothing.
// When file priority toggling lands, the app layer only assigns onTapped to
// receive the new inclusion state; the state advance below already follows
// mixed -> checked -> unchecked -> checked.
func (c *triStateCheck) Tapped(*fyne.PointEvent) {
	if c.onTapped == nil {
		return
	}
	next := checkStateChecked
	if c.Checked {
		next = checkStateUnchecked
	}
	// Mixed is stored as unchecked with Partial, and unchecked files re-enter
	// the download: both advance to checked.
	c.SetState(next)
	c.onTapped(next == checkStateChecked)
}

// TypedRune suppresses the embedded Check's spacebar toggle in case the
// widget receives keyboard focus; TypedKey is a no-op already.
func (c *triStateCheck) TypedRune(rune) {}

// MouseIn forwards hover to the row: the checkbox is hover-capable and would
// otherwise swallow the event, leaving the row untinted under the pointer.
func (c *triStateCheck) MouseIn(*desktop.MouseEvent) {
	if c.row != nil {
		c.row.hoverIn()
	}
}

func (c *triStateCheck) MouseMoved(*desktop.MouseEvent) {}

func (c *triStateCheck) MouseOut() {
	if c.row != nil {
		c.row.hoverOut()
	}
}
