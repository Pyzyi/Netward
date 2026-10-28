package hub

import "testing"

func TestPublishSubscribe(t *testing.T) {
	h := New()
	a, b := h.Subscribe(2), h.Subscribe(1)
	h.Publish("device", 1)
	h.Publish("device", 2) // b is full now and must not block
	if (<-a.C).Data != 1 || (<-a.C).Data != 2 {
		t.Fatal("a missed messages")
	}
	if (<-b.C).Data != 1 || !b.Dropped() || b.Dropped() {
		t.Fatal("b must report (once) that it dropped a message")
	}
	h.Unsubscribe(a)
	if h.Count() != 1 {
		t.Fatal("unsubscribe failed")
	}
	var nilHub *Hub
	nilHub.Publish("x", nil) // must not panic
}
