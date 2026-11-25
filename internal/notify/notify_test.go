package notify

import "testing"

func TestNopDiscardsNotifications(t *testing.T) {
	var n Notifier = Nop{}
	if err := n.Notify("summary", "body"); err != nil {
		t.Fatalf("Nop.Notify returned %v", err)
	}
}
