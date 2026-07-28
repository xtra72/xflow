// device_id_repo.go (v0.18.6) 는 (agentName, unitID) → device_id (UUID) 매핑을
// 제공하는 저장소의 패키지-레벨 싱글턴이다.
//
// 5 HVAC 에이전트 (century / samsung NASA / lg.{LGAP, Hvacr01, LGCP}) 가 디바이스
// 등록 시 글로벌 고유 device_id 를 할당하기 위해 사용한다. 실 impl 은
// internal/storage 의 DeviceIDFileRepository / DeviceIDMemoryRepository.
//
// 와이어링: cmd/xflowd/main.go 가 startup 시 SetDeviceIDRepository 로 주입.
// 미설정 (nil) 상태에서도 에이전트가 죽지 않도록 GetDeviceIDRepository 호출자는
// nil 체크 후 fallback (UUID 없이 unit_id 만 emit) 처리.

package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// ErrDeviceIDConflict 는 지정 device_id 가 이미 다른 (agentName, unitID) 에
// 배정돼 있어 등록이 거부됐음을 나타낸다. Set 이 유일성 위반 시 이 에러(래핑)를
// 반환하며, 호출자는 errors.Is 로 충돌을 구분해 도메인 에러로 매핑한다.
var ErrDeviceIDConflict = errors.New("device-id repository: device_id already assigned to another device")

// DeviceIDRepository 는 (agentName, unitID) → device_id (UUID) 매핑을 제공한다.
// 구체 구현체는 internal/storage 패키지에 위치한다.
type DeviceIDRepository interface {
	// GetOrCreate 는 (agentName, unitID) 의 UUID 를 반환한다. 없으면 새로 생성.
	GetOrCreate(ctx context.Context, agentName, unitID string) (string, error)
	// Get 은 (agentName, unitID) 의 UUID 를 반환한다. 없으면 빈 문자열.
	Get(ctx context.Context, agentName, unitID string) (string, error)
	// Set 은 (agentName, unitID) 에 지정 deviceID 를 등록·영속한다.
	// 지정 deviceID 가 이미 다른 (agentName, unitID) 에 배정돼 있으면
	// ErrDeviceIDConflict(래핑)를 반환한다. 같은 키에 같은 값 재지정은 idempotent.
	Set(ctx context.Context, agentName, unitID, deviceID string) error
}

var (
	deviceIDRepoMu sync.RWMutex
	deviceIDRepo   DeviceIDRepository

	// deviceIDRepoMissingWarnOnce 는 DeviceIDRepository 미설정 경고를 프로세스
	// 라이프타임 동안 단 한 번만 로깅하도록 보장한다. ResolveDeviceID 가 매
	// 호출마다 로그를 쏟아내지 않도록 spam 방지 (SPEC-DEVICE-IDENTITY-001 M1).
	deviceIDRepoMissingWarnOnce sync.Once
)

// SetDeviceIDRepository 는 패키지-레벨 device_id 저장소를 설정한다.
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다.
func SetDeviceIDRepository(r DeviceIDRepository) {
	deviceIDRepoMu.Lock()
	defer deviceIDRepoMu.Unlock()
	deviceIDRepo = r
}

// GetDeviceIDRepository 는 현재 설정된 저장소를 반환한다 (없으면 nil).
// 호출자는 nil 체크 후 fallback 처리해야 한다.
func GetDeviceIDRepository() DeviceIDRepository {
	deviceIDRepoMu.RLock()
	defer deviceIDRepoMu.RUnlock()
	return deviceIDRepo
}

// ResolveDeviceID 는 (agentName, unitID) 의 UUID 를 안전하게 조회한다.
// 저장소가 미설정이거나 에러면 빈 문자열을 반환하고 에러는 무시 (best-effort).
// 호출자는 빈 device_id 의 경우 unit_id 만으로 emit 한다 (graceful degradation).
//
// SPEC-DEVICE-IDENTITY-001 M1: 저장소 미설정 시 한 번만 경고 로그를 남긴다
// (Phase A graceful degradation 시그널; Phase D 에서 부팅 실패로 전환 예정).
// 모든 호출에 spam 하지 않도록 sync.Once 로 보호한다.
func ResolveDeviceID(ctx context.Context, agentName, unitID string) string {
	repo := GetDeviceIDRepository()
	if repo == nil {
		deviceIDRepoMissingWarnOnce.Do(func() {
			slog.Warn("DeviceIDRepository not configured; Device.UID() will return empty (graceful degradation). " +
				"Phase D of SPEC-DEVICE-IDENTITY-001 will fail boot in this state.")
		})
		return ""
	}
	// agentName 이 이름이든 ID 든 정본 에이전트 ID 로 정규화하여, 저장소 키가
	// 항상 ID 기준으로 일관되게 발급되도록 한다 (이름키/ID키 중복 발급 방지).
	agentName = normalizeAgentRef(agentName)
	id, err := repo.GetOrCreate(ctx, agentName, unitID)
	if err != nil {
		return ""
	}
	return id
}

// SetDeviceID 는 (agentName, unitID) 에 사용자가 지정한 deviceID 를 등록·영속한다.
// ResolveDeviceID 와 동일하게 저장소 미설정(nil) 시 no-op(nil)으로 graceful 하며,
// agentName 을 정본 에이전트 ID 로 정규화하여 읽기/쓰기 키를 일치시킨다.
//
// 지정 deviceID 가 이미 다른 (agentName, unitID) 에 배정돼 있으면 저장소가
// ErrDeviceIDConflict(래핑)를 반환한다. 같은 키에 같은 값 재지정은 idempotent.
func SetDeviceID(ctx context.Context, agentName, unitID, deviceID string) error {
	repo := GetDeviceIDRepository()
	if repo == nil {
		// 저장소 미설정: 지정 device_id 를 영속할 수 없으나, ResolveDeviceID 도
		// nil 저장소에서 "" 를 반환하므로(graceful degradation) 여기서도 no-op.
		return nil
	}
	agentName = normalizeAgentRef(agentName)
	return repo.Set(ctx, agentName, unitID, deviceID)
}

// GetDeviceID 는 (agentName, unitID) 에 등록된 device_id 를 조회한다(읽기 전용).
// GetOrCreate 와 달리 없으면 새로 생성하지 않고 빈 문자열을 반환한다.
// 저장소 미설정(nil) 시 "" 를 반환하며 agentName 을 정본 ID 로 정규화한다.
//
// 재등록 경로가 "저장소에 이미 값이 있으면 덮어쓰지 않는다"(set-if-absent)를
// 판단할 때 사용한다 — 기존 저장 디바이스의 device_id 를 보존하기 위함.
func GetDeviceID(ctx context.Context, agentName, unitID string) string {
	repo := GetDeviceIDRepository()
	if repo == nil {
		return ""
	}
	agentName = normalizeAgentRef(agentName)
	id, err := repo.Get(ctx, agentName, unitID)
	if err != nil {
		return ""
	}
	return id
}

// resetDeviceIDRepoMissingWarnForTest 는 테스트 전용으로 sync.Once 를 재설정한다.
// 프로덕션 코드는 호출하지 않는다.
func resetDeviceIDRepoMissingWarnForTest() {
	deviceIDRepoMu.Lock()
	defer deviceIDRepoMu.Unlock()
	deviceIDRepoMissingWarnOnce = sync.Once{}
}
