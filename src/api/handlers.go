package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"flux/src/config"
	"flux/src/events"
	llm "flux/src/llm/provider"
)

type Handler struct {
	sm *SessionManager
}

func NewHandler(sm *SessionManager) *Handler {
	return &Handler{sm: sm}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/sessions", h.listSessions)
	mux.HandleFunc("POST /api/sessions", h.createSession)
	mux.HandleFunc("GET /api/sessions/{id}", h.getSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", h.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/run", h.runSession)
	mux.HandleFunc("POST /api/sessions/{id}/clear", h.clearSession)
	mux.HandleFunc("GET /api/providers", h.listProviders)
	mux.HandleFunc("POST /api/providers/{name}/activate", h.activateProvider)
	mux.HandleFunc("GET /api/config", h.getConfig)
	mux.HandleFunc("GET /api/skills", h.listSkills)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok", "version": config.Cfg.AppVersion})
}

// --- Sessions ---

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.sm.List()
	infos := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		infos = append(infos, s.Info())
	}
	jsonOK(w, infos)
}

type createSessionRequest struct {
	WorkDir string `json:"work_dir"`
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.WorkDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			http.Error(w, "cannot determine working directory", http.StatusInternalServerError)
			return
		}
		req.WorkDir = cwd
	}

	s := h.sm.Create(req.WorkDir)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(s.Info())
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s := h.sm.Get(id)
	if s == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, s.Info())
}

func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if h.sm.Get(id) == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	h.sm.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Run (SSE streaming) ---

type runRequest struct {
	Prompt string `json:"prompt"`
}

func (h *Handler) runSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s := h.sm.Get(id)
	if s == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if !s.mu.TryLock() {
		writeSSE(w, `{"type":"error","error":"session is already running a prompt"}`)
		flusher.Flush()
		return
	}
	defer s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		_, err := s.Runtime.Run(req.Prompt)
		done <- err
	}()

	eventCh := s.Runtime.GetExecutorEventChannel()

loop:
	for {
		select {
		case event := <-eventCh:
			writeSSE(w, responseEventJSON(event))
			flusher.Flush()
		case err := <-done:
			// drain remaining buffered events
		drain:
			for {
				select {
				case event := <-eventCh:
					writeSSE(w, responseEventJSON(event))
					flusher.Flush()
				default:
					break drain
				}
			}
			if err != nil {
				b, _ := json.Marshal(map[string]string{"type": "error", "error": err.Error()})
				writeSSE(w, string(b))
			} else {
				writeSSE(w, `{"type":"done"}`)
			}
			flusher.Flush()
			break loop
		case <-r.Context().Done():
			break loop
		}
	}
}

func writeSSE(w http.ResponseWriter, data string) {
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func responseEventJSON(e events.ResponseEvent) string {
	var typ string
	switch e.EventType {
	case events.Message:
		typ = "message"
	case events.Tool:
		typ = "tool"
	case events.StreamChunk:
		typ = "chunk"
	case events.Error:
		typ = "error"
	default:
		typ = "message"
	}
	b, _ := json.Marshal(map[string]any{
		"type":     typ,
		"text":     e.Message,
		"subagent": e.SubAgent,
		"agent":    e.SubAgentName,
		"skill":    e.SkillName,
	})
	return string(b)
}

// --- Clear ---

func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s := h.sm.Get(id)
	if s == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !s.mu.TryLock() {
		http.Error(w, "session is busy", http.StatusConflict)
		return
	}
	defer s.mu.Unlock()
	s.Runtime.Clear()
	w.WriteHeader(http.StatusNoContent)
}

// --- Providers ---

type providerInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	known := []string{"anthropic", "openai", "openrouter", "bedrock"}
	active := config.Cfg.ActiveProviderName
	infos := make([]providerInfo, 0, len(known))
	for _, name := range known {
		canonical, _ := llm.GetProviderName(name)
		infos = append(infos, providerInfo{
			Name:   name,
			Active: string(canonical) == active,
		})
	}
	jsonOK(w, infos)
}

func (h *Handler) activateProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	canonical, err := llm.GetProviderName(name)
	if err != nil {
		http.Error(w, "unknown provider: "+name, http.StatusBadRequest)
		return
	}
	config.Cfg.ActiveProviderName = string(canonical)
	config.Cfg.Save()
	w.WriteHeader(http.StatusNoContent)
}

// --- Config ---

type configView struct {
	ActiveProvider string            `json:"active_provider"`
	CurrentModel   string            `json:"current_model"`
	ProviderModels map[string]string `json:"provider_models"`
	StreamResponse bool              `json:"stream_responses"`
}

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, configView{
		ActiveProvider: config.Cfg.ActiveProviderName,
		CurrentModel:   config.Cfg.CurrentModel,
		ProviderModels: config.Cfg.ProviderModels,
		StreamResponse: config.Cfg.StreamResponses,
	})
}

// --- Skills ---

type skillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

func (h *Handler) listSkills(w http.ResponseWriter, r *http.Request) {
	sessions := h.sm.List()
	if len(sessions) == 0 {
		jsonOK(w, []skillInfo{})
		return
	}
	reg := sessions[0].Runtime.SkillRegistry
	if reg == nil {
		jsonOK(w, []skillInfo{})
		return
	}
	all := reg.ListEnabled()
	infos := make([]skillInfo, 0, len(all))
	for _, sk := range all {
		infos = append(infos, skillInfo{
			Name:        sk.Name,
			Description: sk.Description,
			Enabled:     sk.Enabled,
		})
	}
	jsonOK(w, infos)
}
