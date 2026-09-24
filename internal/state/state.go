// Package state 持久化 rainmail 的运行状态, 用于抑制同一场降水的重复提醒.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version 是状态文件结构版本, 与配置文件一样需要时可升级.
const Version = 1

// maxHistory 限制保留的提醒记录条数.
const maxHistory = 50

// Record 是一次已发出的提醒, 保留用于审计与排查.
type Record struct {
	At       time.Time `json:"at"`
	EventKey string    `json:"event_key"`
	Level    string    `json:"level"`
	Kind     string    `json:"kind"`
	Summary  string    `json:"summary"`
	Channels []string  `json:"channels"`
	Errors   []string  `json:"errors,omitempty"`
}

// State 是持久化的运行状态.
type State struct {
	Version        int       `json:"version"`
	LastCheckedAt  time.Time `json:"last_checked_at"`
	LastNotifiedAt time.Time `json:"last_notified_at"`
	LastEventKey   string    `json:"last_event_key"`
	LastLevel      string    `json:"last_level"`
	LastSummary    string    `json:"last_summary"`
	History        []Record  `json:"history,omitempty"`
}

// Store 提供带锁的状态读写, 可被常驻循环与单次检查共用.
type Store struct {
	path string
	mu   sync.Mutex
	st   State
}

// Open 读取状态文件, 文件不存在时返回空状态.
// 文件损坏时会把它重命名为 .corrupt 备份并重新开始, 避免程序无法启动.
func Open(path string) (*Store, error) {
	store := &Store{path: path, st: State{Version: Version}}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var loaded State
		if err := json.Unmarshal(data, &loaded); err != nil {
			backup := path + ".corrupt"
			if renameErr := os.Rename(path, backup); renameErr != nil {
				return nil, fmt.Errorf("状态文件 %s 解析失败且无法备份: %w", path, err)
			}
			store.st = State{Version: Version}
			return store, nil
		}
		if loaded.Version == 0 {
			loaded.Version = Version
		}
		store.st = loaded
	case errors.Is(err, os.ErrNotExist):
		// 首次运行, 保持空状态.
	default:
		return nil, fmt.Errorf("读取状态文件 %s 失败: %w", path, err)
	}

	return store, nil
}

// Path 返回状态文件路径.
func (s *Store) Path() string { return s.path }

// Snapshot 返回状态的副本.
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.st)
}

// Update 在锁内修改状态并落盘.
func (s *Store) Update(fn func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fn(&s.st)
	s.st.Version = Version
	if len(s.st.History) > maxHistory {
		s.st.History = s.st.History[len(s.st.History)-maxHistory:]
	}
	return s.save()
}

func (s *Store) save() error {
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建状态目录 %s 失败: %w", dir, err)
		}
	}

	data, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return fmt.Errorf("编码状态失败: %w", err)
	}
	data = append(data, '\n')

	// 先写临时文件再重命名, 避免进程中断留下半个文件.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入状态文件 %s 失败: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("替换状态文件 %s 失败: %w", s.path, err)
	}
	return nil
}

func clone(st State) State {
	out := st
	if len(st.History) > 0 {
		out.History = append([]Record(nil), st.History...)
	}
	return out
}
