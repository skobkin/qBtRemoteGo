package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestTriStateCheckSetState(t *testing.T) {
	test.NewTempApp(t)

	check := newTriStateCheck(nil)
	win := test.NewWindow(check)
	defer win.Close()

	tests := []struct {
		name    string
		state   checkState
		checked bool
		partial bool
	}{
		{"unchecked", checkStateUnchecked, false, false},
		{"checked", checkStateChecked, true, false},
		{"mixed", checkStateMixed, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fyne.DoAndWait(func() {
				check.SetState(tt.state)
			})

			if check.Checked != tt.checked {
				t.Fatalf("Checked = %v, want %v", check.Checked, tt.checked)
			}
			if check.Partial != tt.partial {
				t.Fatalf("Partial = %v, want %v", check.Partial, tt.partial)
			}

			// Rows re-bind on every poll tick; a repeated state must be a
			// no-op rather than a repaint.
			fyne.DoAndWait(func() {
				check.SetState(tt.state)
			})

			if check.Checked != tt.checked || check.Partial != tt.partial {
				t.Fatalf("repeated SetState changed the state: Checked %v, Partial %v", check.Checked, check.Partial)
			}
		})
	}
}

func TestTriStateCheckIsReadOnly(t *testing.T) {
	test.NewTempApp(t)

	check := newTriStateCheck(nil)
	check.Resize(check.MinSize())
	check.Move(fyne.NewPos(10, 10))
	win := test.NewWindow(check)
	defer win.Close()
	win.Resize(fyne.NewSize(60, 60))

	fyne.DoAndWait(func() {
		check.SetState(checkStateChecked)
	})

	// A pointer tap must reach the shadowed Tapped through the driver and
	// leave the displayed state untouched.
	test.TapCanvas(win.Canvas(), fyne.NewPos(18, 18))
	if !check.Checked {
		t.Fatal("tap should not change the read-only checkbox state")
	}

	fyne.DoAndWait(func() {
		check.TypedRune(' ')
	})
	if !check.Checked {
		t.Fatal("spacebar should not change the read-only checkbox state")
	}

	fyne.DoAndWait(func() {
		check.SetState(checkStateUnchecked)
	})
	if check.Checked || check.Partial {
		t.Fatal("state should stay settable programmatically")
	}
}

func TestTriStateCheckTapCyclesStateOnceInteractive(t *testing.T) {
	test.NewTempApp(t)

	check := newTriStateCheck(nil)
	win := test.NewWindow(check)
	defer win.Close()

	var reported []bool
	fyne.DoAndWait(func() {
		check.onTapped = func(next bool) {
			reported = append(reported, next)
		}
		check.SetState(checkStateMixed)
	})

	// mixed -> checked -> unchecked -> checked
	states := []struct {
		checked bool
		partial bool
	}{
		{true, false},
		{false, false},
		{true, false},
	}
	for index, want := range states {
		fyne.DoAndWait(func() {
			check.Tapped(nil)
		})

		if check.Checked != want.checked || check.Partial != want.partial {
			t.Fatalf("tap %d: Checked %v, Partial %v, want Checked %v, Partial %v", index+1, check.Checked, check.Partial, want.checked, want.partial)
		}
	}
	if len(reported) != len(states) {
		t.Fatalf("expected %d onTapped reports, got %d", len(states), len(reported))
	}
	for index, next := range reported {
		if next != states[index].checked {
			t.Fatalf("onTapped report %d = %v, want %v", index+1, next, states[index].checked)
		}
	}
}

func TestTriStateCheckHoverForwardsToRow(t *testing.T) {
	test.NewTempApp(t)

	app := newTestApplication(t)
	row := newDetailsContentRow(app)
	// The fyne test driver runs fyne.Do callbacks inline on the timer
	// goroutine, so a live 50 ms timer would write widget state concurrently
	// with the assertions. Disable the timer and drive the delayed hide here.
	row.hoverOutDelay = time.Hour
	win := test.NewWindow(row)
	defer win.Close()
	win.Resize(fyne.NewSize(totalContentWidth(), contentRowHeight))

	if row.hoverBG.Visible() {
		t.Fatal("hover background should start hidden")
	}

	fyne.DoAndWait(func() {
		row.checkbox.MouseIn(nil)
	})
	if !row.hoverBG.Visible() {
		t.Fatal("hover background should be visible while the checkbox is hovered")
	}

	fyne.DoAndWait(func() {
		row.checkbox.MouseOut()
	})
	if !row.hoverBG.Visible() {
		t.Fatal("hover background should remain visible while the hide delay is pending")
	}

	fyne.DoAndWait(row.finishHoverOut)

	if row.hoverBG.Visible() {
		t.Fatal("hover background should be hidden after the hide delay elapsed")
	}
}

func TestDetailsContentRowSetsCheckboxState(t *testing.T) {
	test.NewTempApp(t)

	app := newTestApplication(t)
	row := newDetailsContentRow(app)
	row.Resize(fyne.NewSize(totalContentWidth(), contentRowHeight))
	win := test.NewWindow(row)
	defer win.Close()

	tests := []struct {
		name     string
		priority int
		checked  bool
		partial  bool
	}{
		{"ignored file is unchecked", contentPriorityIgnored, false, false},
		{"normal priority is checked", contentPriorityNormal, true, false},
		{"mixed folder is partial", contentPriorityMixed, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fyne.DoAndWait(func() {
				row.SetRow(&contentVisibleRow{node: &contentNode{priority: tt.priority}})
			})

			if row.checkbox.Checked != tt.checked {
				t.Fatalf("checkbox Checked = %v, want %v", row.checkbox.Checked, tt.checked)
			}
			if row.checkbox.Partial != tt.partial {
				t.Fatalf("checkbox Partial = %v, want %v", row.checkbox.Partial, tt.partial)
			}
		})
	}
}
