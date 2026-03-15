package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type UserSettings struct {
	KindleEmail string `json:"kindle_email"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	settings map[string]*UserSettings
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:     path,
		settings: make(map[string]*UserSettings),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &s.settings); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) GetSettings(key string) (UserSettings, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	us, ok := s.settings[key]
	if !ok {
		return UserSettings{}, false
	}
	return *us, true
}

func (s *Store) SetSettings(key string, us UserSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = &us
	return s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// EnsureDir creates the parent directory of the settings file if needed.
func (s *Store) EnsureDir() error {
	return os.MkdirAll(filepath.Dir(s.path), 0755)
}
