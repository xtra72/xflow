package observe

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// ParseLogLevel 은 문자열을 slog.Level로 변환한다.
// 유효한 값: "debug", "info", "warn", "error" (대소문자 무시).
// 빈 문자열이면 ok=false를 반환한다.
func ParseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "":
		return slog.LevelInfo, fmt.Errorf("empty log level")
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %q", s)
	}
}

// LevelManager 는 컴포넌트별 로그 레벨을 관리하는 인터페이스이다.
type LevelManager interface {
	// GetLevel 은 지정된 컴포넌트의 로그 레벨을 반환한다.
	// 등록되지 않은 컴포넌트는 기본 레벨을 반환한다.
	GetLevel(component string) slog.Level

	// SetLevel 은 지정된 컴포넌트의 로그 레벨을 설정한다.
	SetLevel(component string, level slog.Level)

	// SetLevelByPattern 은 패턴에 매칭되는 모든 컴포넌트의 레벨을 설정하고,
	// 변경된 컴포넌트 수를 반환한다.
	SetLevelByPattern(pattern string, level slog.Level) int

	// DefaultLevel 은 기본 로그 레벨을 반환한다.
	DefaultLevel() slog.Level

	// SetDefaultLevel 은 기본 로그 레벨을 변경한다.
	SetDefaultLevel(level slog.Level)

	// Levels 는 등록된 모든 컴포넌트의 레벨 스냅샷을 반환한다.
	Levels() map[string]slog.Level
}

// levelManager 는 LevelManager 의 구현체이다.
// sync.Map 과 slog.LevelVar 를 사용하여 동시성 안전을 보장한다.
type levelManager struct {
	// registry 는 컴포넌트 이름 -> *slog.LevelVar 매핑을 저장한다.
	registry sync.Map

	// defaultLevel 은 등록되지 않은 컴포넌트의 기본 레벨이다.
	defaultLevel *slog.LevelVar
}

// NewLevelManager 는 지정된 기본 레벨로 새 LevelManager 를 생성한다.
func NewLevelManager(defaultLevel slog.Level) LevelManager {
	lv := &slog.LevelVar{}
	lv.Set(defaultLevel)
	return &levelManager{
		defaultLevel: lv,
	}
}

// registerLevel 은 컴포넌트의 LevelVar 를 생성하거나 기존 것을 반환한다.
func (lm *levelManager) registerLevel(component string) *slog.LevelVar {
	if v, ok := lm.registry.Load(component); ok {
		return v.(*slog.LevelVar)
	}
	lv := &slog.LevelVar{}
	lv.Set(lm.defaultLevel.Level())
	actual, _ := lm.registry.LoadOrStore(component, lv)
	return actual.(*slog.LevelVar)
}

// GetLevel 은 지정된 컴포넌트의 로그 레벨을 반환한다.
// 등록되지 않은 컴포넌트는 기본 레벨을 반환한다.
func (lm *levelManager) GetLevel(component string) slog.Level {
	if v, ok := lm.registry.Load(component); ok {
		return v.(*slog.LevelVar).Level()
	}
	return lm.defaultLevel.Level()
}

// SetLevel 은 지정된 컴포넌트의 로그 레벨을 설정한다.
// 컴포넌트가 등록되지 않은 경우 자동으로 등록한다.
func (lm *levelManager) SetLevel(component string, level slog.Level) {
	lv := lm.registerLevel(component)
	lv.Set(level)
}

// SetLevelByPattern 은 패턴에 매칭되는 모든 컴포넌트의 레벨을 설정한다.
// 패턴 규칙:
//   - "agent.*" 는 "agent." 으로 시작하는 모든 컴포넌트에 매칭
//   - "*" 는 모든 등록된 컴포넌트에 매칭
//   - 트레일링 "*" 를 제거하고 HasPrefix 로 매칭
func (lm *levelManager) SetLevelByPattern(pattern string, level slog.Level) int {
	count := 0

	// "*" 패턴: 모든 컴포넌트에 매칭
	if pattern == "*" {
		lm.registry.Range(func(key, value any) bool {
			value.(*slog.LevelVar).Set(level)
			count++
			return true
		})
		return count
	}

	// 트레일링 "*" 가 있으면 접두사 매칭
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		lm.registry.Range(func(key, value any) bool {
			comp := key.(string)
			if strings.HasPrefix(comp, prefix) {
				value.(*slog.LevelVar).Set(level)
				count++
			}
			return true
		})
		return count
	}

	// 정확한 매칭
	if v, ok := lm.registry.Load(pattern); ok {
		v.(*slog.LevelVar).Set(level)
		count = 1
	}
	return count
}

// DefaultLevel 은 기본 로그 레벨을 반환한다.
func (lm *levelManager) DefaultLevel() slog.Level {
	return lm.defaultLevel.Level()
}

// SetDefaultLevel 은 기본 로그 레벨을 변경한다.
func (lm *levelManager) SetDefaultLevel(level slog.Level) {
	lm.defaultLevel.Set(level)
}

// Levels 는 등록된 모든 컴포넌트의 레벨 스냅샷을 반환한다.
// 반환된 맵을 수정해도 원본에 영향을 주지 않는다.
func (lm *levelManager) Levels() map[string]slog.Level {
	result := make(map[string]slog.Level)
	lm.registry.Range(func(key, value any) bool {
		result[key.(string)] = value.(*slog.LevelVar).Level()
		return true
	})
	return result
}
