package events

import (
	"sync"
	"testing"
	"time"
)

func TestHub_PublishToSubscriber(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(42)
	defer sub.Close()

	h.Publish(42, Event{Type: "notification", NotificationID: 1})

	select {
	case ev := <-sub.C:
		if ev.NotificationID != 1 {
			t.Errorf("got id %d, want 1", ev.NotificationID)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive event")
	}
}

func TestHub_OtherUserDoesNotReceive(t *testing.T) {
	h := NewHub()
	a := h.Subscribe(1)
	b := h.Subscribe(2)
	defer a.Close()
	defer b.Close()

	h.Publish(1, Event{Type: "notification", NotificationID: 100})

	select {
	case ev := <-a.C:
		if ev.NotificationID != 100 {
			t.Errorf("a got %d, want 100", ev.NotificationID)
		}
	case <-time.After(time.Second):
		t.Fatal("a did not receive")
	}

	select {
	case ev := <-b.C:
		t.Errorf("b should not receive, got %v", ev)
	case <-time.After(50 * time.Millisecond):
		// good: nothing for b
	}
}

func TestHub_NonBlockingOnFullBuffer(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(1)
	defer sub.Close()

	// 32 个填满,33 个不能阻塞
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			h.Publish(1, Event{Type: "notification", NotificationID: int64(i)})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked")
	}
}

func TestHub_CloseUnsubscribes(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(7)
	if h.CountSubscribers(7) != 1 {
		t.Fatalf("count = %d, want 1", h.CountSubscribers(7))
	}
	sub.Close()
	if h.CountSubscribers(7) != 0 {
		t.Errorf("count after Close = %d, want 0", h.CountSubscribers(7))
	}
	// 再次 Close 不应 panic
	sub.Close()
}

func TestHub_ConcurrentPubSub(t *testing.T) {
	h := NewHub()
	const N = 50

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			s := h.Subscribe(uid)
			defer s.Close()
			h.Publish(uid, Event{Type: "notification"})
			select {
			case <-s.C:
			case <-time.After(time.Second):
			}
		}(int64(i))
	}
	wg.Wait()
}

func TestHub_Shutdown(t *testing.T) {
	h := NewHub()
	s1 := h.Subscribe(1)
	s2 := h.Subscribe(2)
	h.Shutdown()
	// channels should be closed
	select {
	case _, ok := <-s1.C:
		if ok {
			t.Error("s1.C should be closed")
		}
	case <-time.After(time.Second):
		t.Error("s1.C not closed")
	}
	select {
	case _, ok := <-s2.C:
		if ok {
			t.Error("s2.C should be closed")
		}
	case <-time.After(time.Second):
		t.Error("s2.C not closed")
	}
	// Publish after shutdown 不应 panic
	h.Publish(1, Event{})
	// 二次调用 close 不 panic
	s1.Close()
}
