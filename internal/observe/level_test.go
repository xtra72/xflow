package observe_test

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/xtra/xflow/internal/observe"
)

// TestLevelManager_GetSetLevel 은 기본적인 레벨 설정과 조회를 검증한다.
func TestLevelManager_GetSetLevel(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	// 컴포넌트 레벨을 설정하고 조회한다
	lm.SetLevel("agent.mqtt", slog.LevelDebug)

	got := lm.GetLevel("agent.mqtt")
	if got != slog.LevelDebug {
		t.Errorf("GetLevel(\"agent.mqtt\") = %v, 기대값 %v", got, slog.LevelDebug)
	}

	// 다른 레벨로 변경한다
	lm.SetLevel("agent.mqtt", slog.LevelWarn)

	got = lm.GetLevel("agent.mqtt")
	if got != slog.LevelWarn {
		t.Errorf("GetLevel(\"agent.mqtt\") 변경 후 = %v, 기대값 %v", got, slog.LevelWarn)
	}
}

// TestLevelManager_DefaultLevel 은 등록되지 않은 컴포넌트가 기본 레벨을 반환하는지 검증한다.
func TestLevelManager_DefaultLevel(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelWarn)

	got := lm.GetLevel("unregistered.component")
	if got != slog.LevelWarn {
		t.Errorf("등록되지 않은 컴포넌트 GetLevel = %v, 기대값 %v", got, slog.LevelWarn)
	}

	defaultLevel := lm.DefaultLevel()
	if defaultLevel != slog.LevelWarn {
		t.Errorf("DefaultLevel() = %v, 기대값 %v", defaultLevel, slog.LevelWarn)
	}
}

// TestLevelManager_SetDefaultLevel 은 기본 레벨 변경이 반영되는지 검증한다.
func TestLevelManager_SetDefaultLevel(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	lm.SetDefaultLevel(slog.LevelError)

	got := lm.DefaultLevel()
	if got != slog.LevelError {
		t.Errorf("SetDefaultLevel 후 DefaultLevel() = %v, 기대값 %v", got, slog.LevelError)
	}

	// 등록되지 않은 컴포넌트도 변경된 기본값을 사용해야 한다
	got = lm.GetLevel("new.component")
	if got != slog.LevelError {
		t.Errorf("SetDefaultLevel 후 미등록 컴포넌트 GetLevel = %v, 기대값 %v", got, slog.LevelError)
	}
}

// TestLevelManager_SetLevelByPattern 은 와일드카드 패턴 매칭을 검증한다.
func TestLevelManager_SetLevelByPattern(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	// 여러 컴포넌트를 등록한다
	lm.SetLevel("agent.mqtt", slog.LevelInfo)
	lm.SetLevel("agent.http", slog.LevelInfo)
	lm.SetLevel("agent.mqtt.client1", slog.LevelInfo)
	lm.SetLevel("router.main", slog.LevelInfo)
	lm.SetLevel("db.postgres", slog.LevelInfo)

	tests := []struct {
		name          string
		pattern       string
		level         slog.Level
		expectedCount int
		checkTargets  map[string]slog.Level // 패턴 적용 후 확인할 컴포넌트와 기대 레벨
	}{
		{
			name:          "agent.* 패턴은 agent.으로 시작하는 모든 컴포넌트에 매칭",
			pattern:       "agent.*",
			level:         slog.LevelDebug,
			expectedCount: 3, // agent.mqtt, agent.http, agent.mqtt.client1
			checkTargets: map[string]slog.Level{
				"agent.mqtt":         slog.LevelDebug,
				"agent.http":         slog.LevelDebug,
				"agent.mqtt.client1": slog.LevelDebug,
				"router.main":        slog.LevelInfo, // 변경되지 않아야 함
			},
		},
		{
			name:          "* 패턴은 모든 컴포넌트에 매칭",
			pattern:       "*",
			level:         slog.LevelWarn,
			expectedCount: 5,
			checkTargets: map[string]slog.Level{
				"agent.mqtt":   slog.LevelWarn,
				"router.main":  slog.LevelWarn,
				"db.postgres":  slog.LevelWarn,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := lm.SetLevelByPattern(tt.pattern, tt.level)
			if count != tt.expectedCount {
				t.Errorf("SetLevelByPattern(%q) 반환 = %d, 기대값 %d", tt.pattern, count, tt.expectedCount)
			}

			for comp, expectedLevel := range tt.checkTargets {
				got := lm.GetLevel(comp)
				if got != expectedLevel {
					t.Errorf("패턴 %q 적용 후 GetLevel(%q) = %v, 기대값 %v",
						tt.pattern, comp, got, expectedLevel)
				}
			}
		})
	}
}

// TestLevelManager_SetLevelByPattern_Specific 은 더 구체적인 패턴과 미매칭 패턴을 검증한다.
func TestLevelManager_SetLevelByPattern_Specific(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	lm.SetLevel("agent.mqtt", slog.LevelInfo)
	lm.SetLevel("agent.http", slog.LevelInfo)
	lm.SetLevel("agent.mqtt.client1", slog.LevelInfo)

	// agent.mqtt.* 는 agent.mqtt.client1 만 매칭 (agent.mqtt 자체는 매칭하지 않음)
	count := lm.SetLevelByPattern("agent.mqtt.*", slog.LevelDebug)
	if count != 1 {
		t.Errorf("SetLevelByPattern(\"agent.mqtt.*\") = %d, 기대값 1", count)
	}

	got := lm.GetLevel("agent.mqtt.client1")
	if got != slog.LevelDebug {
		t.Errorf("agent.mqtt.client1 레벨 = %v, 기대값 %v", got, slog.LevelDebug)
	}

	// agent.mqtt 자체는 변경되지 않아야 한다
	got = lm.GetLevel("agent.mqtt")
	if got != slog.LevelInfo {
		t.Errorf("agent.mqtt 레벨 = %v, 기대값 %v (변경되면 안 됨)", got, slog.LevelInfo)
	}

	// 존재하지 않는 패턴
	count = lm.SetLevelByPattern("nonexistent.*", slog.LevelDebug)
	if count != 0 {
		t.Errorf("SetLevelByPattern(\"nonexistent.*\") = %d, 기대값 0", count)
	}
}

// TestLevelManager_Levels 는 등록된 모든 컴포넌트의 레벨 스냅샷을 검증한다.
func TestLevelManager_Levels(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	lm.SetLevel("agent.mqtt", slog.LevelDebug)
	lm.SetLevel("agent.http", slog.LevelWarn)
	lm.SetLevel("router.main", slog.LevelError)

	levels := lm.Levels()

	expected := map[string]slog.Level{
		"agent.mqtt":  slog.LevelDebug,
		"agent.http":  slog.LevelWarn,
		"router.main": slog.LevelError,
	}

	if len(levels) != len(expected) {
		t.Errorf("Levels() 길이 = %d, 기대값 %d", len(levels), len(expected))
	}

	for comp, expectedLevel := range expected {
		got, ok := levels[comp]
		if !ok {
			t.Errorf("Levels() 에 %q 가 없다", comp)
			continue
		}
		if got != expectedLevel {
			t.Errorf("Levels()[%q] = %v, 기대값 %v", comp, got, expectedLevel)
		}
	}

	// 스냅샷이므로 반환된 맵을 수정해도 원본에 영향이 없어야 한다
	levels["agent.mqtt"] = slog.LevelError
	afterModify := lm.GetLevel("agent.mqtt")
	if afterModify != slog.LevelDebug {
		t.Errorf("스냅샷 수정 후 원본 값이 변경되었다: %v, 기대값 %v", afterModify, slog.LevelDebug)
	}
}

// TestLevelManager_Concurrent 는 동시성 안전성을 검증한다.
// 10개 고루틴 x 1000 연산을 수행하며 -race 플래그로 실행해야 한다.
func TestLevelManager_Concurrent(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 1000

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				comp := "component." + string(rune('a'+id))

				switch j % 5 {
				case 0:
					lm.SetLevel(comp, slog.LevelDebug)
				case 1:
					lm.GetLevel(comp)
				case 2:
					lm.SetLevelByPattern("component.*", slog.LevelInfo)
				case 3:
					lm.Levels()
				case 4:
					lm.SetDefaultLevel(slog.LevelWarn)
				}
			}
		}(i)
	}

	wg.Wait()

	// 패닉이나 데이터 레이스 없이 완료되면 성공
}

// TestParseLogLevel 은 문자열을 slog.Level로 변환하는 함수를 검증한다.
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"  Debug  ", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"INFO", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"WARN", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"ERROR", slog.LevelError, false},
		{"", slog.LevelInfo, true},
		{"invalid", slog.LevelInfo, true},
		{"trace", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := observe.ParseLogLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLogLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestLevelManager_RegisterLevel 은 동일 컴포넌트를 반복 등록해도
// 같은 LevelVar 를 사용하는지 검증한다.
func TestLevelManager_RegisterLevel(t *testing.T) {
	lm := observe.NewLevelManager(slog.LevelInfo)

	// 처음 설정
	lm.SetLevel("agent.mqtt", slog.LevelDebug)
	first := lm.GetLevel("agent.mqtt")

	// 다시 설정 (같은 컴포넌트)
	lm.SetLevel("agent.mqtt", slog.LevelWarn)
	second := lm.GetLevel("agent.mqtt")

	if first == second {
		t.Error("같은 컴포넌트에 다른 레벨 설정 후에도 값이 동일하다")
	}

	if second != slog.LevelWarn {
		t.Errorf("재설정 후 GetLevel = %v, 기대값 %v", second, slog.LevelWarn)
	}
}
