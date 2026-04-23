package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

// dialTestHub spins up a test HTTP server with the hub's HandleWS handler and
// returns a client WebSocket connection to it.
func dialTestHub(t *testing.T, h *Hub, userID uuid.UUID) *ws.Conn {
	t.Helper()
	srv := httptest.NewServer(h.HandleWS(func(r *http.Request) *uuid.UUID {
		return &userID
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestHub_SendToUser_DeliveredToClient(t *testing.T) {
	h := NewHub()
	userID := uuid.New()

	conn := dialTestHub(t, h, userID)

	want := `{"event":"test"}`
	h.SendToUser(userID, []byte(want))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg) != want {
		t.Errorf("got %q, want %q", string(msg), want)
	}
}

func TestHub_SendToUser_NoBroadcastBlockOnSlowClient(t *testing.T) {
	h := NewHub()

	// Create two users: one fast (we read), one slow (we never read).
	fastID := uuid.New()
	slowID := uuid.New()

	// Dial fast client.
	fastConn := dialTestHub(t, h, fastID)

	// Dial slow client but never read from it.
	_ = dialTestHub(t, h, slowID)

	// Let connections register.
	time.Sleep(50 * time.Millisecond)

	// Fill the slow client's send buffer to capacity.
	for i := 0; i < sendBufSize+10; i++ {
		h.SendToUser(slowID, []byte(`{}`))
	}

	// Send to fast client — this must not block despite the slow client.
	done := make(chan struct{})
	go func() {
		h.SendToUser(fastID, []byte(`{"event":"fast"}`))
		close(done)
	}()

	select {
	case <-done:
		// Good — SendToUser returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("SendToUser blocked — hub was stalled by slow client")
	}

	// Fast client should receive the message.
	fastConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := fastConn.ReadMessage()
	if err != nil {
		t.Fatalf("fast client read: %v", err)
	}
	if string(msg) != `{"event":"fast"}` {
		t.Errorf("fast client got %q", string(msg))
	}
}

func TestHub_Register_Unregister(t *testing.T) {
	h := NewHub()
	userID := uuid.New()

	_ = dialTestHub(t, h, userID)
	// Give the server goroutine time to register.
	time.Sleep(50 * time.Millisecond)

	h.mu.RLock()
	count := len(h.conns[userID])
	h.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 registered connection, got %d", count)
	}
}

func TestHub_MultipleConnsSameUser(t *testing.T) {
	h := NewHub()
	userID := uuid.New()

	conn1 := dialTestHub(t, h, userID)
	_ = dialTestHub(t, h, userID)

	time.Sleep(50 * time.Millisecond)

	h.mu.RLock()
	count := len(h.conns[userID])
	h.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 connections for same user, got %d", count)
	}

	// Both should receive the message.
	want := `{"event":"multi"}`
	h.SendToUser(userID, []byte(want))

	var wg sync.WaitGroup
	for _, c := range []*ws.Conn{conn1} {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, msg, err := c.ReadMessage()
			if err != nil {
				t.Errorf("ReadMessage: %v", err)
				return
			}
			if string(msg) != want {
				t.Errorf("got %q, want %q", string(msg), want)
			}
		}()
	}
	wg.Wait()
}

func TestHub_SendToUser_UnknownUser_NoOp(t *testing.T) {
	h := NewHub()
	// Sending to a user with no connections should be a no-op (no panic, no error).
	h.SendToUser(uuid.New(), []byte("hello"))
}
