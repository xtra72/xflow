package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// 오버라이드 레이어 파일/정책 상수.
//
// 설계: 사용자 원본 config(xflow.yaml)는 절대 수정하지 않고 읽기 전용 baseline 으로
// 유지한다. 런타임 편집값은 별도 오버라이드 파일에 저장하고, 로드 시 baseline 위에
// 병합한다. 저장할 때마다 이전 버전을 히스토리로 회전 보관하며, 오버라이드 파일이
// 손상(파싱 실패)되면 오버라이드를 무시하고 원본 config 로 복원한다.
const (
	overridesFileName   = "xflow.overrides.yaml"
	overridesHistoryDir = "xflow.overrides.history"
	overridesMaxHistory = 10
)

// overrideAllowedPrefixes 는 오버라이드로 영속화 가능한 config 키 접두사 allowlist 이다.
// 보안: 임의 config 키를 오버라이드 파일로 덮어쓰지 못하게 명시된 접두사만 허용한다.
//   - remote_management.*        : 원격 관리 클라이언트 설정 UI (SPEC-REMOTE-001)
//   - storage.schedule_log.type  : 스케줄 로그 저장소 백엔드 선택 (재시작 시 적용, 비-mutable)
var overrideAllowedPrefixes = []string{"remote_management.", "storage.schedule_log.type"}

// IsOverridable 은 key 가 오버라이드 저장 대상인지(allowlist) 반환한다.
func IsOverridable(key string) bool {
	for _, p := range overrideAllowedPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// OverrideStore 는 원본 config 를 보존한 채 런타임 편집값을 별도 파일에 저장하는
// 오버라이드 레이어이다. 내부적으로 오버라이드 값만 담는 viper 인스턴스를 유지해
// dotted-key(예: "remote_management.mode") 접근을 처리한다.
type OverrideStore struct {
	dir        string
	path       string
	historyDir string
	maxHistory int
	logger     *slog.Logger

	mu sync.Mutex
	v  *viper.Viper // 오버라이드 값만 보관 (dotted-key aware)
}

// NewOverrideStore 는 dir 아래에 오버라이드 파일/히스토리를 두는 스토어를 생성하고,
// 기존 오버라이드 파일이 있으면 로드한다(손상 시 무시).
func NewOverrideStore(dir string, logger *slog.Logger) *OverrideStore {
	if logger == nil {
		logger = slog.Default()
	}
	s := &OverrideStore{
		dir:        dir,
		path:       filepath.Join(dir, overridesFileName),
		historyDir: filepath.Join(dir, overridesHistoryDir),
		maxHistory: overridesMaxHistory,
		logger:     logger,
		v:          viper.New(),
	}
	s.load()
	return s
}

// load 는 오버라이드 파일을 파싱해 내부 viper 에 반영한다. 파일 미존재는 정상.
// 파싱 실패(손상)면 WARN 로그 후 무시한다 — 원본 config 로 복원되는 효과(오버라이드 없음).
func (s *OverrideStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			s.logger.Warn("config override: 파일 읽기 실패 — 오버라이드 무시(원본 config 사용)",
				"path", s.path, "error", err)
		}
		return
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		s.logger.Warn("config override: 파일 손상(파싱 실패) — 오버라이드 무시(원본 config 복원)",
			"path", s.path, "error", err)
		return
	}
	if err := s.v.MergeConfigMap(m); err != nil {
		s.logger.Warn("config override: 병합 실패 — 오버라이드 무시", "path", s.path, "error", err)
	}
}

// Values 는 baseline config 에 병합할 오버라이드 값(nested map)을 반환한다. 없으면 빈 map.
func (s *OverrideStore) Values() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v.AllSettings()
}

// Set 은 오버라이드 값을 갱신하고 파일에 영속화한다(히스토리 회전 포함).
// key 는 IsOverridable 대상이어야 한다(호출측에서 검증 전제).
func (s *OverrideStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v.Set(key, value)
	return s.saveLocked()
}

// saveLocked 는 현재 오버라이드 값을 파일에 원자적으로 기록한다. mu 보유 전제.
// 기존 파일이 있으면 히스토리로 회전 보관한 뒤, 임시 파일 + rename 으로 교체한다.
func (s *OverrideStore) saveLocked() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("config override: 디렉토리 생성 실패: %w", err)
	}

	// 히스토리 회전: 기존 파일을 timestamp 이름으로 백업.
	if existing, err := os.ReadFile(s.path); err == nil {
		if mkErr := os.MkdirAll(s.historyDir, 0o755); mkErr == nil {
			histPath := filepath.Join(s.historyDir, fmt.Sprintf("%d.yaml", time.Now().UnixMilli()))
			_ = os.WriteFile(histPath, existing, 0o600)
			s.pruneHistoryLocked()
		}
	}

	out, err := yaml.Marshal(s.v.AllSettings())
	if err != nil {
		return fmt.Errorf("config override: YAML 직렬화 실패: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("config override: 임시 파일 기록 실패: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("config override: 파일 교체 실패: %w", err)
	}
	return nil
}

// pruneHistoryLocked 는 히스토리 파일을 최근 maxHistory 개만 남기고 삭제한다. mu 보유 전제.
// 파일명이 epoch-ms 이므로 사전순 정렬 = 시간순 정렬이다.
func (s *OverrideStore) pruneHistoryLocked() {
	entries, err := os.ReadDir(s.historyDir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= s.maxHistory {
		return
	}
	sort.Strings(names)
	for _, old := range names[:len(names)-s.maxHistory] {
		_ = os.Remove(filepath.Join(s.historyDir, old))
	}
}
