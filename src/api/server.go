package api

import (
	"fmt"
	"net/http"

	"flux/src/events"
)

func NewServer(port int) *http.Server {
	sm := NewSessionManager()
	h := NewHandler(sm)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	go drainGlobalChannels()

	return &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: corsMiddleware(mux),
	}
}

// drainGlobalChannels consumes the global event channels that the runtime
// writes to via goroutines. Without drainers, these goroutines would leak.
func drainGlobalChannels() {
	go func() {
		for {
			events.EventManager.ReadFromChannel(events.NOTIFICATION_CHANNEL)
		}
	}()
	go func() {
		for {
			events.EventManager.ReadFromChannel(events.PLAN_STATUS_CHANNEL)
		}
	}()
	go func() {
		for {
			events.EventManager.ReadFromChannel(events.COMPACTION_CHANNEL)
		}
	}()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
