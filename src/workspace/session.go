package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	llm "flux/src/llm/provider"
	"flux/src/secrets"
)

const sessionsDir = ".flux/sessions"

type Session struct {
	ID        string        `json:"id"`
	StartedAt time.Time     `json:"started_at"`
	Workspace string        `json:"workspace"`
	Messages  []llm.Message `json:"messages,omitempty"`
	Path      string        `json:"-"`
}

func NewSession(workspaceRoot string) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(workspaceRoot, sessionsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	if err := addSessionsToGitignore(workspaceRoot); err != nil {
		return nil, fmt.Errorf("add sessions to gitignore: %w", err)
	}

	s := &Session{
		ID:        id,
		StartedAt: time.Now(),
		Workspace: workspaceRoot,
		Path:      filepath.Join(dir, id+".json"),
	}

	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

func addSessionsToGitignore(workspaceRoot string) error {
	gitignorePath := filepath.Join(workspaceRoot, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == sessionsDir {
			return nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, "\n"+sessionsDir)
	return err
}

func (s *Session) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	redactedData := secrets.RedactForDisplay(string(data))
	return os.WriteFile(s.Path, []byte(redactedData), 0o644)
}

func ListSessions(workspaceRoot string) ([]*Session, error) {
	dir := filepath.Join(workspaceRoot, sessionsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sessions := make([]*Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		s, err := LoadSession(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.After(sessions[j].StartedAt)
	})
	return sessions, nil
}

func LoadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.Path = path
	return &s, nil
}

func generateSessionID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return time.Now().
		UTC().
		Format("20060102-150405") +
		"-" + hex.EncodeToString(
		buf,
	), nil
}
