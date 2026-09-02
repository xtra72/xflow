package xsfm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// B7 — 로스터 영속화 라운드트립 (REQ-XSFM-001-02-05 / 02-08)
// ---------------------------------------------------------------------------
//
// 런타임 등록 디바이스(Source="bridge"/"auto")를 registry_path 저장소에 영속화하고 Init 시
// 복원한다. 설정 디바이스(Source="config")는 절대 저장하지 않는다(REQ-02-05). 위치 계층
// 속성(station/place/index)까지 라운드트립한다(REQ-02-08). 로스터는 device_id 로 키잉한다
// (프로젝트 규약). 저장소 포맷은 device_metadata.go 패턴(단일 JSON 파일 + tmp+rename atomic
// write)을 미러하되, CRUD 변경 후 전체 스냅샷을 통째로 덮어쓰므로 인메모리 캐시는 두지 않는다.
//
// 이는 xsfm 로스터 전용 저장소이며, 설정-디바이스 메커니즘(parseConfigDevices)과 별개다.
// registry_path 는 station_registry_path 와 동일하게 디렉터리로 취급하며 그 안의
// device_registry.json 에 저장한다(패키지 내 일관성).
//
// device_id 저장소(agent.ResolveDeviceID/SetDeviceID) 라운드트립: samsung/lg 는 프로토콜
// 주소(UnitID)를 글로벌 device_id(UUID)로 매핑하기 위해 device_id 저장소를 쓴다. xsfm 의
// device_id 는 사용자가 부여한 글로벌 식별자이자 로스터 키 자체이며 프로토콜 주소에서 도출되지
// 않으므로, 그 매핑 라운드트립은 적용되지 않는다(device_id 를 그대로 저장/복원한다).

// persistedDevice 는 registry_path 저장소의 디바이스 항목이다(REQ-XSFM-001-02-05/08).
//
// 후방호환(REQ-02-08): station/place/index 필드가 없던 v0.2.0 포맷을 로드하면 JSON unmarshal
// 이 부재 키를 zero 값(""/0)으로 자연 복원하므로 오류 없이 하위호환된다.
type persistedDevice struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	GroupID  string `json:"group_id"`
	Station  string `json:"station,omitempty"`
	Place    string `json:"place,omitempty"`
	Index    int    `json:"index,omitempty"`
	Source   string `json:"source"`
	// NameOverridden 은 composite 디바이스의 커스텀 이름 sticky 여부를 라운드트립한다 — 재시작
	// 후에도 사용자 지정 이름이 주소 변경으로 재계산되지 않도록 보존한다(REQ sticky name).
	NameOverridden bool `json:"name_overridden,omitempty"`
}

// deviceRegistryStore 는 런타임 등록 디바이스 로스터의 파일 기반 영속 저장소이다.
// device_metadata.go 패턴(단일 JSON 파일 + atomic write)을 미러한다.
type deviceRegistryStore struct {
	filePath string
	mu       sync.Mutex
}

// newDeviceRegistryStore 는 파일 기반 로스터 저장소를 생성한다. 파일은 {dir}/device_registry.json
// 에 저장되며 디렉터리가 없으면 생성된다.
func newDeviceRegistryStore(dir string) (*deviceRegistryStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create device registry directory: %w", err)
	}
	return &deviceRegistryStore{filePath: filepath.Join(dir, "device_registry.json")}, nil
}

// load 는 저장된 디바이스를 device_id → persistedDevice 맵으로 읽어들인다.
// 파일이 없으면 빈 맵 + nil 을 반환한다(최초 기동).
func (s *deviceRegistryStore) load() (map[string]persistedDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]persistedDevice{}, nil
		}
		return nil, fmt.Errorf("read device registry: %w", err)
	}
	if len(data) == 0 {
		return map[string]persistedDevice{}, nil
	}
	var out map[string]persistedDevice
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal device registry: %w", err)
	}
	return out, nil
}

// save 는 디바이스 스냅샷을 JSON 파일에 atomic 하게 기록한다(tmp write + rename).
func (s *deviceRegistryStore) save(devices map[string]persistedDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device registry: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp device registry file: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp device registry file: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 에이전트 영속화 표면 (Init 복원 · CRUD 변경 후 저장 · B9 API 어댑터 경로)
// ---------------------------------------------------------------------------

// loadPersistedRoster 는 registry_path 저장소에서 런타임 디바이스를 로스터에 복원한다
// (REQ-XSFM-001-02-05/08). Init 에서 호출한다. 설정 기반 디바이스(NewXSFMAgent 에서
// 선등록)와 device_id 가 충돌하면 설정이 우선하여 덮어쓰지 않는다(config precedence). 복원된
// 디바이스는 저장된 Source 를 유지한다(재시작 후에도 "bridge" → 삭제 가능성 보존).
func (a *XSFMAgent) loadPersistedRoster() error {
	if a.registry == nil {
		return nil
	}
	persisted, err := a.registry.load()
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for id, pd := range persisted {
		if _, exists := a.devices[id]; exists {
			continue // config precedence — 설정 디바이스가 우선
		}
		dev := &Device{
			DeviceID: id,
			Name:     pd.Name,
			GroupID:  pd.GroupID,
			Station:  pd.Station,
			Place:    pd.Place,
			Index:    pd.Index,
			Online:   false,
			Source:   pd.Source,
			// 저장된 device_id 는 verbatim 으로 키에 사용한다(과거 id 를 재작성하지 않음). UUID 형태면
			// 합성 주소 모델 디바이스로 간주해 composite=true 로 표시한다 — 이후 set_device 로 위치가
			// 바뀌면 Name 을 재계산한다. 비-UUID(과거 합성/blob id)는 composite=false 로 둔다.
			composite: isUUID(id),
			// 커스텀 이름 sticky 여부 복원 — 재시작 후에도 사용자 지정 이름이 보존된다.
			nameOverridden: pd.NameOverridden,
		}
		a.devices[id] = dev
		// 보조 인덱스는 station/place/index 로부터 항상 재구축한다(id 형태와 무관) — 유입 상태
		// 토픽이 재시작 후에도 올바른 device_id 로 매칭되도록 한다.
		a.indexDeviceLocked(dev)
	}
	return nil
}

// isUUID 는 문자열이 UUID(v4 등) 형식인지 판정한다. 영속 복원 시 device_id 가 생성된 UUID(합성
// 주소 모델)인지 과거 blob/합성 식별자인지 구분하는 데 쓴다.
func isUUID(s string) bool {
	return uuid.Validate(s) == nil
}

// persistRoster 는 현재 런타임 로스터(Source="bridge"/"auto")의 스냅샷을 저장소에 통째로
// 기록한다. add/remove/set 디바이스 CRUD 핸들러가 변경 후 호출한다. 저장소 미설정이면 no-op,
// 저장 실패는 로깅만 하고 CRUD 명령을 실패시키지 않는다(best-effort, REQ-02-05).
//
// 락 규율: 로스터 스냅샷은 RLock 하에 뜨고 락 해제 후 파일 I/O 를 수행한다 — 락을 I/O 에
// 걸쳐 잡지 않으며 저장소 자체 락(deviceRegistryStore.mu)과 로스터 락(mu)을 중첩하지 않는다.
func (a *XSFMAgent) persistRoster() {
	if a.registry == nil {
		return
	}

	a.mu.RLock()
	snapshot := make(map[string]persistedDevice)
	for id, dev := range a.devices {
		if dev.Source != "bridge" && dev.Source != "auto" {
			continue // 설정 디바이스는 절대 영속화하지 않는다(REQ-02-05)
		}
		snapshot[id] = persistedDevice{
			DeviceID:       dev.DeviceID,
			Name:           dev.Name,
			GroupID:        dev.GroupID,
			Station:        dev.Station,
			Place:          dev.Place,
			Index:          dev.Index,
			Source:         dev.Source,
			NameOverridden: dev.nameOverridden,
		}
	}
	a.mu.RUnlock()

	if err := a.registry.save(snapshot); err != nil {
		a.logger.Warn("xsfm: persist roster failed", "error", err)
	}
}

// GetPersistableDevices 는 영속 저장 대상 디바이스(Source="bridge"/"auto")를 agent.DeviceEntry
// 목록으로 반환한다(samsung Hvacr01Agent.GetPersistableDevices 시그니처 미러). B9 의 API 어댑터
// 로스터 영속화 경로(persistDeviceRosterAfterExec)가 이 표면을 소비한다.
//
// xsfm 의 device_id 는 정체성(주소 = 안정 식별자)이므로 Address/Name 슬롯에 device_id 를,
// 사용자 표시 이름은 DisplayName 슬롯에 담는다(samsung 이 Name=UnitID, DisplayName=사용자명 을
// 쓰는 패턴에 대응). Source 를 보존해 재시작 후에도 "bridge" 삭제 가능성이 유지된다.
func (a *XSFMAgent) GetPersistableDevices() []agent.DeviceEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []agent.DeviceEntry
	for id, dev := range a.devices {
		if dev.Source != "bridge" && dev.Source != "auto" {
			continue
		}
		result = append(result, agent.DeviceEntry{
			Address:     id,       // device_id = xsfm 정체성(주소 = 식별자)
			Name:        id,       // 안정 식별자 슬롯
			DisplayName: dev.Name, // 사용자 표시 이름
			Source:      dev.Source,
		})
	}
	return result
}
