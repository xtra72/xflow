package main

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/script"
)

// captureHandler 는 slog 레코드를 수집하는 테스트용 핸들러이다.
// warn 로깅 여부를 단언하기 위해 사용한다.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) countAtLevel(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == level {
			n++
		}
	}
	return n
}

func newCaptureLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{}
	return slog.New(h), h
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestEffectiveScriptSettings_ValidTimeout 는 유효한 timeout 이 그대로 파싱되는지 검증한다.
func TestEffectiveScriptSettings_ValidTimeout(t *testing.T) {
	log, cap := newCaptureLogger()
	sc := config.ScriptConfig{Timeout: "3s"}

	got := effectiveScriptSettings(sc, log)

	if got.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %v, want 3s", got.Timeout)
	}
	if warns := cap.countAtLevel(slog.LevelWarn); warns != 0 {
		t.Fatalf("유효 timeout 인데 warn 로그 %d건 발생", warns)
	}

	// 엔진 옵션에 max-execution-time 이 실제로 반영되는지 엔진 Info 로 확인한다.
	eng := script.NewScriptEngine(got.engineOptions()...)
	if eng.Info().MaxExecutionTime != 3*time.Second {
		t.Fatalf("engine MaxExecutionTime = %v, want 3s", eng.Info().MaxExecutionTime)
	}
}

// TestEffectiveScriptSettings_InvalidTimeout 는 빈 값/무효 값이 5s 로 폴백하고
// warn 을 남기는지 검증한다.
func TestEffectiveScriptSettings_InvalidTimeout(t *testing.T) {
	for _, bad := range []string{"", "abc"} {
		log, cap := newCaptureLogger()
		sc := config.ScriptConfig{Timeout: bad}

		got := effectiveScriptSettings(sc, log)

		if got.Timeout != 5*time.Second {
			t.Fatalf("Timeout(%q) = %v, want 5s", bad, got.Timeout)
		}
		if warns := cap.countAtLevel(slog.LevelWarn); warns == 0 {
			t.Fatalf("무효 timeout(%q) 인데 warn 로그가 없음", bad)
		}
	}
}

// TestEffectiveScriptSettings_PoolSize 는 vm_pool_size 옵션 적용 여부를 검증한다.
func TestEffectiveScriptSettings_PoolSize(t *testing.T) {
	log, _ := newCaptureLogger()

	// 0 → 옵션 미적용(엔진 기본값 8 유지).
	zero := effectiveScriptSettings(config.ScriptConfig{Timeout: "5s", VMPoolSize: 0}, log)
	if zero.PoolSize != 0 {
		t.Fatalf("PoolSize = %d, want 0", zero.PoolSize)
	}
	engZero := script.NewScriptEngine(zero.engineOptions()...)
	if engZero.Info().PoolSize != 8 {
		t.Fatalf("pool 0 일 때 engine PoolSize = %d, want 8(기본값)", engZero.Info().PoolSize)
	}

	// >0 → 옵션 적용.
	four := effectiveScriptSettings(config.ScriptConfig{Timeout: "5s", VMPoolSize: 4}, log)
	if four.PoolSize != 4 {
		t.Fatalf("PoolSize = %d, want 4", four.PoolSize)
	}
	engFour := script.NewScriptEngine(four.engineOptions()...)
	if engFour.Info().PoolSize != 4 {
		t.Fatalf("pool 4 일 때 engine PoolSize = %d, want 4", engFour.Info().PoolSize)
	}
}

// TestEffectiveScriptSettings_SandboxOverrides 는 샌드박스 override 규칙과
// 보안 기본값 보존을 검증한다.
func TestEffectiveScriptSettings_SandboxOverrides(t *testing.T) {
	log, _ := newCaptureLogger()

	// MaxMemoryMB 0 → DefaultSandboxConfig 의 128 유지.
	m0 := effectiveScriptSettings(config.ScriptConfig{
		Timeout: "5s",
		Sandbox: config.SandboxConfig{Enabled: true, MaxMemoryMB: 0},
	}, log)
	if m0.Sandbox.MaxMemoryMB != 128 {
		t.Fatalf("MaxMemoryMB(0) = %d, want 128", m0.Sandbox.MaxMemoryMB)
	}

	// MaxMemoryMB 32 → 32.
	m32 := effectiveScriptSettings(config.ScriptConfig{
		Timeout: "5s",
		Sandbox: config.SandboxConfig{Enabled: true, MaxMemoryMB: 32},
	}, log)
	if m32.Sandbox.MaxMemoryMB != 32 {
		t.Fatalf("MaxMemoryMB(32) = %d, want 32", m32.Sandbox.MaxMemoryMB)
	}

	// MaxExecutionMS 0 → 기본 5s 유지.
	e0 := effectiveScriptSettings(config.ScriptConfig{
		Timeout: "5s",
		Sandbox: config.SandboxConfig{Enabled: true, MaxExecutionMS: 0},
	}, log)
	if e0.Sandbox.MaxExecutionTime != 5*time.Second {
		t.Fatalf("MaxExecutionTime(0) = %v, want 5s", e0.Sandbox.MaxExecutionTime)
	}

	// MaxExecutionMS 2000 → 2s.
	e2 := effectiveScriptSettings(config.ScriptConfig{
		Timeout: "5s",
		Sandbox: config.SandboxConfig{Enabled: true, MaxExecutionMS: 2000},
	}, log)
	if e2.Sandbox.MaxExecutionTime != 2*time.Second {
		t.Fatalf("MaxExecutionTime(2000) = %v, want 2s", e2.Sandbox.MaxExecutionTime)
	}

	// Enabled=false → 샌드박스 비활성이지만 보안 기본값 목록은 여전히 보존.
	disabled := effectiveScriptSettings(config.ScriptConfig{
		Timeout: "5s",
		Sandbox: config.SandboxConfig{Enabled: false},
	}, log)
	if disabled.Sandbox.Enabled {
		t.Fatalf("Enabled = true, want false")
	}
	if !contains(disabled.Sandbox.DisabledModules, "os") ||
		!contains(disabled.Sandbox.DisabledModules, "io") ||
		!contains(disabled.Sandbox.DisabledModules, "debug") {
		t.Fatalf("DisabledModules 보안 기본값 유실: %v", disabled.Sandbox.DisabledModules)
	}
	if !contains(disabled.Sandbox.DisabledFunctions, "loadfile") ||
		!contains(disabled.Sandbox.DisabledFunctions, "dofile") {
		t.Fatalf("DisabledFunctions 보안 기본값 유실: %v", disabled.Sandbox.DisabledFunctions)
	}
	if disabled.Sandbox.AllowDynamicLoad {
		t.Fatalf("AllowDynamicLoad = true, want false(보안 기본값)")
	}
}

// TestEffectiveScriptSettings_SecurityDefaultsPreservedOnOverride 는 config override
// 를 적용해도 보안 기본값(DisabledModules/AllowDynamicLoad)이 사라지지 않음을 검증한다.
func TestEffectiveScriptSettings_SecurityDefaultsPreservedOnOverride(t *testing.T) {
	log, _ := newCaptureLogger()
	sc := config.ScriptConfig{
		Timeout:    "3s",
		VMPoolSize: 4,
		Sandbox:    config.SandboxConfig{Enabled: true, MaxMemoryMB: 32, MaxExecutionMS: 2000},
	}

	got := effectiveScriptSettings(sc, log)

	if !contains(got.Sandbox.DisabledModules, "os") ||
		!contains(got.Sandbox.DisabledModules, "io") ||
		!contains(got.Sandbox.DisabledModules, "debug") {
		t.Fatalf("override 후 DisabledModules 유실: %v", got.Sandbox.DisabledModules)
	}
	if got.Sandbox.AllowDynamicLoad {
		t.Fatalf("override 후 AllowDynamicLoad = true, want false")
	}
}
