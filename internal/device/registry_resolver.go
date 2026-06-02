// registry_resolver.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T1)
//
// 본 파일은 device.DeviceRegistry 의 Get(id) dispatch 를 디바이스 참조 형식
// (UUID / agent/name / composite) 에 따라 분기하는 resolver 헬퍼와, 명시적
// UUID / (agent, name) lookup 메서드 (GetByUID / GetByAgentName) 를 제공한다.
//
// 동작 우선순위 (Soft Deprecation):
//   1. UUID v4 형식 (정규식 매칭) → UUID 기본 인덱스 lookup (Device.UID() 비교)
//   2. "agent/name" 형식 (슬래시 포함) → (agent, name) 보조 인덱스 lookup
//      (Device.AgentName() == agent AND Device.Name() == name)
//   3. 그 외 → 기존 composite key dispatch (provider.Device(id))
//
// 외부 클라이언트 무영향 보장:
//   - 기존 Get("agent:local_id") 호출은 그대로 composite path 로 dispatch 된다.
//   - 신규 호출자 (REST UUID URL, agent/name URL) 는 새 분기를 이용한다.
//   - SoftDeprecation 단계 — composite alias 는 그대로 작동 (헤더만 별도).
//
// UID 인덱스 구현 노트:
//   - inMemoryRegistry 는 pull 모델 (provider.Devices() 매번 호출) 을 유지한다.
//   - UUID lookup 은 모든 provider 의 Devices() 를 enumerate 하며 Device.UID()
//     를 비교한다. 추가 mutable 캐시를 유지하지 않으므로 invalidation 결함이
//     없다 (provider 가 진실의 원천).
//   - O(N) lookup 비용은 N (전체 디바이스 수) 가 보통 < 1000 인 xflow 운영
//     특성상 허용 가능하다. 필요 시 후속 PR 에서 캐시 도입.

package device

import (
	"regexp"
	"strings"
)

// uuidv4Pattern 은 UUID v4 형식 ("xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx") 을
// 매칭한다. 대소문자 무관, 8-4-4-4-12 hex 그룹.
var uuidv4Pattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
)

// DeviceRefKind 는 디바이스 참조 문자열의 형식을 분류한다.
type DeviceRefKind int

const (
	// DeviceRefUnknown 은 어떤 형식에도 매칭되지 않는 참조이다.
	// SPEC-DEVICE-IDENTITY-001 Phase D 부터 composite ("agent:local_id") 형식도
	// 본 값으로 분류된다 (registry / yaml / REST 가 일률적으로 404 또는 부팅 실패).
	DeviceRefUnknown DeviceRefKind = iota

	// DeviceRefUUID 는 UUID v4 형식 ("a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d") 이다.
	// SPEC-DEVICE-IDENTITY-001 Phase B 의 1급 식별자.
	DeviceRefUUID

	// DeviceRefAgentName 은 "agent/name" 형식 ("lg_hvacr01/indoor-1") 이다.
	// Phase B 에서 신규 도입된 사람이 읽기 좋은 식별자.
	DeviceRefAgentName

	// DeviceRefComposite 은 v0.x 의 composite key ("agent:local_id" — "lg_icp01:81") 이다.
	//
	// Deprecated: SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T2) 부터
	// 본 enum 값은 더 이상 ClassifyDeviceRef 에서 반환되지 않는다 (composite 형식의
	// 입력은 DeviceRefUnknown 으로 분류). 본 상수는 외부 호출자의 컴파일 호환을 위해
	// 유지되나, registry / yaml / REST URL 모두에서 composite 는 거부된다.
	DeviceRefComposite
)

// String 은 DeviceRefKind 의 사람이 읽을 수 있는 표현을 반환한다 (로그·메트릭 용).
func (k DeviceRefKind) String() string {
	switch k {
	case DeviceRefUUID:
		return "uuid"
	case DeviceRefAgentName:
		return "agent_name"
	case DeviceRefComposite:
		return "composite"
	default:
		return "unknown"
	}
}

// ClassifyDeviceRef 는 디바이스 참조 문자열의 형식을 분류한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T2, Breaking):
// composite ("agent:local_id") 패턴은 더 이상 식별되지 않으며 DeviceRefUnknown
// 으로 분류된다. 호출자 (registry.ResolveDevice / REST handler / yaml 파서) 는
// 이를 명시적 404 또는 부팅 실패로 처리한다.
//
// 우선순위:
//  1. UUID v4 정규식 매칭 → DeviceRefUUID
//  2. 슬래시 포함 ("agent/name") → DeviceRefAgentName
//  3. 그 외 (composite "agent:local_id" 포함) → DeviceRefUnknown
//
// 빈 문자열은 DeviceRefUnknown 으로 분류한다.
func ClassifyDeviceRef(ref string) DeviceRefKind {
	if ref == "" {
		return DeviceRefUnknown
	}
	if uuidv4Pattern.MatchString(ref) {
		return DeviceRefUUID
	}
	if strings.Contains(ref, "/") {
		return DeviceRefAgentName
	}
	// composite ("agent:local_id") 형식과 그 외 알 수 없는 형식은 모두 Unknown.
	// Phase D 부터 composite alias dispatch 는 registry / REST / yaml 모두에서 제거.
	return DeviceRefUnknown
}

// SplitAgentName 은 "agent/name" 형식의 참조를 (agent, name) 으로 분리한다.
//
// 슬래시가 없거나 어느 한 쪽이 비어 있으면 (agent="", name="", ok=false) 를 반환한다.
// 슬래시가 여러 개 있는 경우 (예: "lg_hvacr01/zone/indoor-1") 첫 번째 슬래시를 기준으로
// 분리한다 (agent 는 슬래시를 허용하지 않으나 name 은 허용 가능).
func SplitAgentName(ref string) (agentName, name string, ok bool) {
	idx := strings.Index(ref, "/")
	if idx <= 0 || idx == len(ref)-1 {
		return "", "", false
	}
	return ref[:idx], ref[idx+1:], true
}

// SplitComposite 는 v0.x composite key ("agent:local_id") 를 (agent, localID) 로
// 분리한다. 콜론이 없거나 어느 한 쪽이 비어 있으면 (agent="", localID="", ok=false).
//
// 콜론이 여러 개 있는 경우 (예: "lg_icp01:zone:81") 첫 번째 콜론을 기준으로 분리한다
// (Century / Modbus 등 일부 어댑터에서 localID 가 콜론을 포함할 수 있음).
func SplitComposite(ref string) (agentName, localID string, ok bool) {
	idx := strings.Index(ref, ":")
	if idx <= 0 || idx == len(ref)-1 {
		return "", "", false
	}
	return ref[:idx], ref[idx+1:], true
}

// GetByUID 는 Device.UID() == uid 인 디바이스를 모든 provider 에서 검색하여
// 반환한다. UUID 가 비어 있거나 매칭이 없으면 ErrDeviceNotFound 를 반환한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B 의 1급 lookup 경로.
// pull 모델을 유지하므로 O(N) 비용 (N = 전체 디바이스 수). 부가 인덱스는
// 없다 — provider 가 진실의 원천이며 device 추가/제거의 invalidation 결함을
// 회피한다.
func (r *inMemoryRegistry) GetByUID(uid string) (Device, error) {
	if uid == "" {
		return nil, ErrDeviceNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	for agentName, provider := range r.providers {
		isOffline := r.offlineAgents[agentName]
		for _, dev := range provider.Devices() {
			if dev.UID() == uid {
				return r.wrapIfOffline(dev, isOffline), nil
			}
		}
	}
	return nil, ErrDeviceNotFound
}

// GetByAgentName 은 Device.AgentName() == agent AND Device.Name() == name 인
// 디바이스를 검색하여 반환한다.
//
// agent 또는 name 이 빈 문자열이거나 매칭이 없으면 ErrDeviceNotFound.
//
// 동일 (agent, name) 쌍의 중복 디바이스는 존재하지 않는다고 가정한다 (운영
// 정책상 에이전트 내 name 은 유일). 중복 발견 시 첫 번째 매치를 반환한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B 의 사람이 읽기 좋은 lookup 경로.
func (r *inMemoryRegistry) GetByAgentName(agent, name string) (Device, error) {
	if agent == "" || name == "" {
		return nil, ErrDeviceNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[agent]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	isOffline := r.offlineAgents[agent]
	for _, dev := range provider.Devices() {
		if dev.Name() == name {
			return r.wrapIfOffline(dev, isOffline), nil
		}
	}
	return nil, ErrDeviceNotFound
}

// ResolveDevice 는 참조 문자열의 형식을 자동 판단하여 디바이스를 검색한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T2, Breaking):
// composite ("agent:local_id") 형식은 더 이상 지원하지 않는다. ClassifyDeviceRef
// 가 composite 패턴을 DeviceRefUnknown 으로 분류하므로 본 함수는 별도 분기 없이
// 자동으로 ErrDeviceNotFound 를 반환한다.
//
// 형식 우선순위 (ClassifyDeviceRef 참조):
//   - UUID v4 → GetByUID
//   - "agent/name" → GetByAgentName
//   - 그 외 (composite, 빈 문자열, 알 수 없는 형식) → ErrDeviceNotFound
//
// 두 번째 반환값 kind 는 매칭에 사용된 형식이다.
func (r *inMemoryRegistry) ResolveDevice(ref string) (Device, DeviceRefKind, error) {
	kind := ClassifyDeviceRef(ref)
	switch kind {
	case DeviceRefUUID:
		dev, err := r.GetByUID(ref)
		return dev, kind, err
	case DeviceRefAgentName:
		agent, name, ok := SplitAgentName(ref)
		if !ok {
			return nil, kind, ErrDeviceNotFound
		}
		dev, err := r.GetByAgentName(agent, name)
		return dev, kind, err
	default:
		// composite / 빈 문자열 / 알 수 없는 형식 — Phase D 부터 모두 거부.
		return nil, kind, ErrDeviceNotFound
	}
}
