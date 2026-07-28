package main

import (
	"log/slog"
	"time"

	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/script"
)

// defaultScriptTimeout 는 script.timeout 이 비었거나 파싱 불가/무효(<=0)일 때의
// 폴백 값이다. 엔진 실행 타임아웃과 스크립트 노드 타임아웃 모두에 적용된다.
const defaultScriptTimeout = 5 * time.Second

// scriptSettings 는 config.ScriptConfig 로부터 해석된 "실효(effective)" 스크립트
// 엔진/노드 설정을 담는 순수 값 구조체이다. 단위 테스트에서 각 필드를 직접 단언할
// 수 있도록, 옵션 생성(engineOptions)과 해석(effectiveScriptSettings)을 분리한다.
type scriptSettings struct {
	// Timeout 은 엔진 실행 타임아웃(WithMaxExecutionTime)이자 스크립트 노드
	// 타임아웃(node.WithScriptTimeout)이다. 둘은 동일한 값을 공유한다.
	Timeout time.Duration
	// PoolSize 는 VM 풀 크기이다. 0 이면 옵션을 적용하지 않아 엔진 기본값(8)을 유지한다.
	PoolSize int
	// Sandbox 는 보안 기본값(DisabledModules/DisabledFunctions/AllowDynamicLoad)을
	// 보존한 채 config override 만 얹은 샌드박스 설정이다.
	Sandbox script.SandboxConfig
}

// effectiveScriptSettings 는 config.ScriptConfig 를 실효 설정으로 해석한다.
//
// 해석 규칙:
//   - Timeout: sc.Timeout 을 time.ParseDuration 으로 파싱한다. 빈 값/파싱 실패/
//     비양수(<=0)이면 5s 로 폴백하고 문제 값을 포함해 log.Warn 을 남긴다.
//   - PoolSize: sc.VMPoolSize 를 그대로 담는다(0 이면 engineOptions 가 옵션을
//     생략하여 엔진 기본값이 유지된다).
//   - Sandbox: script.DefaultSandboxConfig() 에서 시작하여 보안 기본값을 보존한 뒤,
//     config 로 명시된 값만 덮어쓴다:
//     Enabled      = sc.Sandbox.Enabled (항상 반영 — 운영자 opt-out 존중)
//     MaxMemoryMB  = sc.Sandbox.MaxMemoryMB (> 0 일 때만)
//     MaxExecutionTime = sc.Sandbox.MaxExecutionMS ms (> 0 일 때만)
func effectiveScriptSettings(sc config.ScriptConfig, log *slog.Logger) scriptSettings {
	timeout := defaultScriptTimeout
	if d, err := time.ParseDuration(sc.Timeout); err == nil && d > 0 {
		timeout = d
	} else if log != nil {
		log.Warn("script.timeout 무효 — 기본값으로 대체",
			"value", sc.Timeout,
			"fallback", defaultScriptTimeout,
		)
	}

	// 보안 기본값을 먼저 깔고, config override 만 얹는다(기본값 유실 방지).
	sb := script.DefaultSandboxConfig()
	sb.Enabled = sc.Sandbox.Enabled
	if sc.Sandbox.MaxMemoryMB > 0 {
		sb.MaxMemoryMB = sc.Sandbox.MaxMemoryMB
	}
	if sc.Sandbox.MaxExecutionMS > 0 {
		sb.MaxExecutionTime = time.Duration(sc.Sandbox.MaxExecutionMS) * time.Millisecond
	}

	return scriptSettings{
		Timeout:  timeout,
		PoolSize: sc.VMPoolSize,
		Sandbox:  sb,
	}
}

// engineOptions 는 실효 설정을 script.EngineOption 목록으로 변환한다.
//
// WithStdlib 는 표준 라이브러리 의존성을 필요로 하므로 호출 측(main)에서 별도로
// 선행 적용하고, 여기서 반환한 옵션을 그 뒤에 append 한다.
func (s scriptSettings) engineOptions() []script.EngineOption {
	opts := []script.EngineOption{
		script.WithMaxExecutionTime(s.Timeout),
		script.WithSandboxConfig(s.Sandbox),
	}
	// PoolSize 0 은 "미지정"으로 간주하여 옵션을 생략한다(엔진 기본값 유지).
	if s.PoolSize > 0 {
		opts = append(opts, script.WithPoolSize(s.PoolSize))
	}
	return opts
}
