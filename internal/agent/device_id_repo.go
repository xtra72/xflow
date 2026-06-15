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
	"log/slog"
	"sync"
)

// DeviceIDRepository 는 (agentName, unitID) → device_id (UUID) 매핑을 제공한다.
// 구체 구현체는 internal/storage 패키지에 위치한다.
type DeviceIDRepository interface {
	// GetOrCreate 는 (agentName, unitID) 의 UUID 를 반환한다. 없으면 새로 생성.
	GetOrCreate(ctx context.Context, agentName, unitID string) (string, error)
	// Get 은 (agentName, unitID) 의 UUID 를 반환한다. 없으면 빈 문자열.
	Get(ctx context.Context, agentName, unitID string) (string, error)
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

// resetDeviceIDRepoMissingWarnForTest 는 테스트 전용으로 sync.Once 를 재설정한다.
// 프로덕션 코드는 호출하지 않는다.
func resetDeviceIDRepoMissingWarnForTest() {
	deviceIDRepoMu.Lock()
	defer deviceIDRepoMu.Unlock()
	deviceIDRepoMissingWarnOnce = sync.Once{}
}
