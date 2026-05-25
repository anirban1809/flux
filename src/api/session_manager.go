package api

import (
	"sync"
	"time"

	"flux/src/agent"
	"flux/src/workspace"
)

type ManagedSession struct {
	ID        string
	WorkDir   string
	CreatedAt time.Time
	Runtime   *agent.Runtime
	mu        sync.Mutex
}

type SessionInfo struct {
	ID           string    `json:"id"`
	WorkDir      string    `json:"work_dir"`
	CreatedAt    time.Time `json:"created_at"`
	MessageCount int       `json:"message_count"`
}

func (s *ManagedSession) Info() SessionInfo {
	msgs := s.Runtime.Agent.Conversation.Messages
	count := len(msgs)
	if count > 0 && msgs[0].Role == "system" {
		count--
	}
	return SessionInfo{
		ID:           s.ID,
		WorkDir:      s.WorkDir,
		CreatedAt:    s.CreatedAt,
		MessageCount: count,
	}
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*ManagedSession
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*ManagedSession),
	}
}

func (sm *SessionManager) Create(workDir string) *ManagedSession {
	ws := workspace.Load(workDir)
	rt := agent.NewRuntime(&ws)

	id := ""
	if ws.Session != nil {
		id = ws.Session.ID
	}

	s := &ManagedSession{
		ID:        id,
		WorkDir:   workDir,
		CreatedAt: time.Now(),
		Runtime:   &rt,
	}

	sm.mu.Lock()
	sm.sessions[s.ID] = s
	sm.mu.Unlock()

	return s
}

func (sm *SessionManager) Get(id string) *ManagedSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

func (sm *SessionManager) List() []*ManagedSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make([]*ManagedSession, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		out = append(out, s)
	}
	return out
}

func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
}
