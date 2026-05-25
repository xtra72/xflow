// yaml_resolver.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T7)
//
// 본 파일은 yaml 설정 파일에서 디바이스 참조 문자열을 정규화하기 위한 resolver
// 를 제공한다. SPEC § M6 의 규칙:
//
//   - "agent/name" 형식 (예: "lgcnp/indoor-1") 1급 (사람이 쓰기 편함).
//   - UUID v4 형식 (예: "a58ba668-5741-...") 1급 (자동 생성 yaml).
//   - composite key (예: "lgcnp:81") 호환 alias (Deprecation 경고).
//   - 어떤 형식에도 매칭되지 않으면 ErrInvalidDeviceReference.
//
// 본 resolver 는 순수 파서이며 실제 lookup (Device 객체 조회) 은 호출자
// (device.DeviceRegistry.ResolveDevice) 에 위임한다. yaml 로딩 시점에는
// Registry 가 아직 채워지지 않을 수 있으므로 분리한 것이다.
//
// 호출 패턴 (yaml 설정 consumer):
//
//	ref, err := config.ParseDeviceRef(rawRef, "/etc/xflow/flow.yaml", lineNum)
//	if err != nil {
//	    return fmt.Errorf("flow.pinned: %w", err)
//	}
//	if ref.IsLegacyComposite() {
//	    observe.IncDeviceCompositeUse("yaml")
//	    logger.Warn("yaml: legacy composite reference",
//	        "ref", ref.Raw, "file", ref.SourceFile, "line", ref.SourceLine)
//	}
//	// 이후 ref.Raw 로 Registry.ResolveDevice(...) 호출.

package config

import (
	"errors"
	"fmt"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
)

// ErrInvalidDeviceReference 는 yaml 의 디바이스 참조가 어떤 형식에도 매칭되지
// 않을 때 반환된다 (SPEC § M6 Unwanted).
var ErrInvalidDeviceReference = errors.New("config: invalid device reference (expected UUID, agent/name, or legacy agent:local_id)")

// DeviceRef 는 yaml 의 디바이스 참조 파싱 결과를 나타낸다.
//
// 사용처 (예: flow.pinned, device 메타데이터 참조 등) 는 본 구조체의 Raw 와
// Kind 를 이용하여 후속 lookup / 경고 / 메트릭 처리를 분기한다.
type DeviceRef struct {
	// Raw 는 yaml 에 작성된 원본 참조 문자열이다 (예: "lgcnp/indoor-1").
	Raw string

	// Kind 는 파싱으로 분류된 형식이다.
	Kind device.DeviceRefKind

	// 분류 결과별 파생 필드 (해당 형식일 때만 유효).
	UUID  string // Kind == DeviceRefUUID 일 때만.
	Agent string // Kind == DeviceRefAgentName / DeviceRefComposite 일 때만.
	Name  string // Kind == DeviceRefAgentName 일 때만.
	Local string // Kind == DeviceRefComposite 일 때만 (composite 의 후반부).

	// 진단 정보 (선택) — 호출자가 경고 / 에러 메시지에 활용.
	SourceFile string
	SourceLine int
}

// IsLegacyComposite 는 참조가 v0.x composite 형식 (deprecated alias) 인지
// 반환한다. yaml consumer 는 이 결과로 Deprecation 메트릭 / 경고를 분기한다.
func (r DeviceRef) IsLegacyComposite() bool {
	return r.Kind == device.DeviceRefComposite
}

// ParseDeviceRef 는 yaml 의 디바이스 참조 문자열을 파싱하여 DeviceRef 를 반환한다.
//
// 형식 우선순위 (device.ClassifyDeviceRef 참조):
//  1. UUID v4 → DeviceRefUUID
//  2. "agent/name" → DeviceRefAgentName
//  3. "agent:local_id" → DeviceRefComposite (Deprecated)
//  4. 그 외 → ErrInvalidDeviceReference (호출자가 부팅 실패로 처리 가능)
//
// 빈 문자열 입력은 ErrInvalidDeviceReference.
//
// sourceFile / sourceLine 은 진단용 메타데이터 (yaml 파서 위치). 0/"" 허용.
func ParseDeviceRef(ref, sourceFile string, sourceLine int) (DeviceRef, error) {
	kind := device.ClassifyDeviceRef(ref)
	result := DeviceRef{
		Raw:        ref,
		Kind:       kind,
		SourceFile: sourceFile,
		SourceLine: sourceLine,
	}

	switch kind {
	case device.DeviceRefUUID:
		result.UUID = ref
		return result, nil

	case device.DeviceRefAgentName:
		agent, name, ok := device.SplitAgentName(ref)
		if !ok {
			return DeviceRef{}, formatRefError(ref, sourceFile, sourceLine)
		}
		result.Agent = agent
		result.Name = name
		return result, nil

	case device.DeviceRefComposite:
		agent, local, ok := device.SplitComposite(ref)
		if !ok {
			return DeviceRef{}, formatRefError(ref, sourceFile, sourceLine)
		}
		result.Agent = agent
		result.Local = local
		// SPEC-DEVICE-IDENTITY-001 § B-T9 — composite alias 사용 빈도 추적.
		// yaml 파싱 단계에서 composite 가 발견되었음을 기록 (Deprecation 메트릭).
		observe.IncDeviceCompositeUse(observe.CompositeUseSourceYAML)
		return result, nil

	default:
		return DeviceRef{}, formatRefError(ref, sourceFile, sourceLine)
	}
}

// ParseDeviceRefs 는 디바이스 참조 슬라이스를 일괄 파싱한다 (yaml 의 `pinned:
// [...]` 같은 배열 처리).
//
// 단 하나의 항목이라도 ErrInvalidDeviceReference 면 즉시 에러로 returns 한다
// (모든 부분 변경 없음 — fail-fast). 호출자가 부분 허용을 원하면 ParseDeviceRef
// 를 항목별로 호출해야 한다.
func ParseDeviceRefs(refs []string, sourceFile string) ([]DeviceRef, error) {
	out := make([]DeviceRef, 0, len(refs))
	for i, raw := range refs {
		parsed, err := ParseDeviceRef(raw, sourceFile, i+1)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

// formatRefError 는 ErrInvalidDeviceReference 에 진단 메타데이터를 wrapping 한다.
func formatRefError(ref, sourceFile string, sourceLine int) error {
	if sourceFile != "" && sourceLine > 0 {
		return fmt.Errorf("%w: %q at %s:%d", ErrInvalidDeviceReference, ref, sourceFile, sourceLine)
	}
	if sourceFile != "" {
		return fmt.Errorf("%w: %q at %s", ErrInvalidDeviceReference, ref, sourceFile)
	}
	return fmt.Errorf("%w: %q", ErrInvalidDeviceReference, ref)
}
