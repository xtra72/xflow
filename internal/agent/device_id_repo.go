// device_id_repo.go (v0.18.6) 는 (agentName, unitID) → device_id (UUID) 매핑을
// 제공하는 저장소의 패키지-레벨 싱글턴이다.
//
// 5 HVAC 에이전트 (century / samsung NASA / lg.{LGAP, LGCNP, LGCP}) 가 디바이스
// 등록 시 글로벌 고유 device_id 를 할당하기 위해 사용한다. 실 impl 은
// internal/storage 의 DeviceIDFileRepository / DeviceIDMemoryRepository.
//
// 와이어링: cmd/xflowd/main.go 가 startup 시 SetDeviceIDRepository 로 주입.
// 미설정 (nil) 상태에서도 에이전트가 죽지 않도록 GetDeviceIDRepository 호출자는
// nil 체크 후 fallback (UUID 없이 unit_id 만 emit) 처리.

package agent

import (
	"context"
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
func ResolveDeviceID(ctx context.Context, agentName, unitID string) string {
	repo := GetDeviceIDRepository()
	if repo == nil {
		return ""
	}
	id, err := repo.GetOrCreate(ctx, agentName, unitID)
	if err != nil {
		return ""
	}
	return id
}
