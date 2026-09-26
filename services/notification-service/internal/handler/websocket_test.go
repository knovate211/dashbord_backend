package handler

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	notificationv1 "github.com/knovate211/proto/notification/v1"
)

// Clients connect, receive and drop out while broadcasts run. Before the hub
// copied its client set under the lock and stopped closing send channels, this
// died with a concurrent map access or a send on a closed channel.
func TestHubConcurrentBroadcastAndDisconnect(t *testing.T) {
	hub := NewHub(zap.NewNop())
	srv := httptest.NewServer(hub)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?user_id=u1"

	stop := make(chan struct{})
	var bg sync.WaitGroup
	for i := 0; i < 4; i++ {
		bg.Add(1)
		go func() {
			defer bg.Done()
			ev := &notificationv1.WebSocketEvent{}
			for {
				select {
				case <-stop:
					return
				default:
					hub.Broadcast("u1", ev)
					hub.BroadcastAll(ev)
				}
			}
		}()
	}

	var clients sync.WaitGroup
	for i := 0; i < 40; i++ {
		clients.Add(1)
		go func(i int) {
			defer clients.Done()
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Error(err)
				return
			}
			// Half read for a moment; half never read, so their buffers fill
			// and the hub drops them as slow while the reader also exits.
			if i%2 == 0 {
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)) //nolint:errcheck
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						break
					}
				}
			} else {
				time.Sleep(50 * time.Millisecond)
			}
			conn.Close()
		}(i)
	}
	clients.Wait()
	close(stop)
	bg.Wait()

	// Every client is gone once the readers notice.
	deadline := time.Now().Add(2 * time.Second)
	for len(hub.snapshot("")) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(hub.snapshot("")); n != 0 {
		t.Errorf("%d clients still registered", n)
	}
}
