// device_info_repo.go (v0.18.7) 는 (agentName, unitID) → DeviceInfo (device_type / label)
// 매핑을 제공하는 패키지-레벨 싱글턴이다.
//
// 의도: 노드의 register-decoded emit 경로에서도 device_state 경로와 동일하게
// metadata.device_type 와 metadata.label 을 노출하기 위해, 에이전트가 디바이스
// 등록 시 미리 정보를 publish 하고 노드는 build 시점에 lookup 한다.
//
// DeviceIDRepository 와 별도 분리: device_id 는 영속 저장 (UUID 보존) 인 반면
// device_info 는 runtime-only (에이전트 재기동마다 갱신).

package agent

import (
	"sync"
)

// DeviceInfo 는 (agentName, unitID) 에 연결된 정적 device metadata 이다.
//
//   - DeviceType: 통합 device_type ("HVACR.IDU", "HVACR.ODU" 등)
//   - Label: 사용자 정의 라벨 (없으면 빈 문자열)
type DeviceInfo struct {
	DeviceType string
	Label      string
}

var (
	deviceInfoMu sync.RWMutex
	deviceInfo   = make(map[string]DeviceInfo) // key: "agentName:unitID"
)

// SetDeviceInfo 는 (agentName, unitID) 의 DeviceInfo 를 등록/갱신한다.
// label 이 변경되거나 device_type 이 바뀌면 새 값으로 덮어쓴다.
func SetDeviceInfo(agentName, unitID string, info DeviceInfo) {
	if agentName == "" || unitID == "" {
		return
	}
	deviceInfoMu.Lock()
	defer deviceInfoMu.Unlock()
	deviceInfo[deviceInfoKey(agentName, unitID)] = info
}

// GetDeviceInfo 는 (agentName, unitID) 의 DeviceInfo 를 반환한다.
// 없으면 zero value (DeviceType="", Label="") 와 ok=false 를 반환.
func GetDeviceInfo(agentName, unitID string) (DeviceInfo, bool) {
	if agentName == "" || unitID == "" {
		return DeviceInfo{}, false
	}
	deviceInfoMu.RLock()
	defer deviceInfoMu.RUnlock()
	info, ok := deviceInfo[deviceInfoKey(agentName, unitID)]
	return info, ok
}

// DeleteDeviceInfo 는 (agentName, unitID) 항목을 제거한다. 디바이스 unregister 시 호출.
func DeleteDeviceInfo(agentName, unitID string) {
	if agentName == "" || unitID == "" {
		return
	}
	deviceInfoMu.Lock()
	defer deviceInfoMu.Unlock()
	delete(deviceInfo, deviceInfoKey(agentName, unitID))
}

// deviceInfoKey 는 (agentName, unitID) 을 device_info 맵 키로 결합한다.
// agentName 을 정본 에이전트 ID 로 정규화하여, Set/Get/Delete 의 읽기·쓰기 키가
// 호출처가 이름을 쓰든 ID 를 쓰든 항상 동일하게 맞춰지도록 한다
// (device_id 정규화와 동일한 기준 — SPEC-DEVICE-IDENTITY-001).
func deviceInfoKey(agentName, unitID string) string {
	return normalizeAgentRef(agentName) + ":" + unitID
}
