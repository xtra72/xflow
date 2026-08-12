package chirpstack

import (
	"context"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
)

// deviceState 는 devEui 로 키잉되는 자동 생성 디바이스의 런타임 스냅샷이다.
//
// applicationID 는 다운링크 토픽 구성에 필요한 ChirpStack applicationId 캐시이다
// (SPEC-CHIRPSTACK-002 REQ-M2-05). comm 맵이 아닌 로스터(devices)에 캐시하는 이유:
// upsertDevice 는 모든 업링크마다 실행되는 반면 comm 맵 갱신은 emit_comm_state 노브
// 뒤에 게이팅되므로, comm 맵에 캐시하면 제어 경로가 그 노브에 숨은 의존을 갖게 된다
// (REQ-M4-04 의 "새 노브 없음" 취지에 반함).
type deviceState struct {
	devEui            string
	deviceName        string
	deviceProfileName string
	applicationID     string
	tags              map[string]string
	lastSeen          time.Time
	online            bool
}

// clone 은 tags 맵을 포함해 깊은 복사한다 (roster 스냅샷용).
func (d *deviceState) clone() deviceState {
	cp := *d
	if d.tags != nil {
		cp.tags = make(map[string]string, len(d.tags))
		for k, v := range d.tags {
			cp.tags[k] = v
		}
	}
	return cp
}

// upsertDevice 는 업링크의 디바이스를 devEui 키로 자동 생성/갱신하고, 시스템 UID
// 발급 및 런타임 DeviceInfo 등록을 수행한다 (REQ-M4-01/02/04).
//
//   - ResolveDeviceID(agentName, devEui): UUID v4 발급/조회 (저장소 미설정 시 "" — graceful).
//   - SetDeviceInfo(agentName, devEui, {DeviceType=deviceProfileName, Label=deviceName}).
//   - 로스터 upsert: deviceName/tags 지속화(Device.Metadata 로 노출).
//
// HVAC 락 함정 회피: agentName 은 devicesMu 획득 전에 1회 캡처한다.
func (a *ChirpStackAgent) upsertDevice(up *uplink) {
	devEui := up.DeviceInfo.DevEui
	if devEui == "" {
		return
	}
	agentName := a.Name() // 락 보유 전 캡처.

	// UID 발급(존재 시 재사용) + 런타임 device_type/label 등록.
	_ = agent.ResolveDeviceID(context.Background(), agentName, devEui)
	agent.SetDeviceInfo(agentName, devEui, agent.DeviceInfo{
		DeviceType: up.DeviceInfo.DeviceProfileName,
		Label:      up.DeviceInfo.DeviceName,
	})

	a.devicesMu.Lock()
	d, ok := a.devices[devEui]
	if !ok {
		d = &deviceState{devEui: devEui}
		a.devices[devEui] = d
	}
	d.deviceName = up.DeviceInfo.DeviceName
	d.deviceProfileName = up.DeviceInfo.DeviceProfileName
	// applicationId 캐시: 다운링크 토픽 구성의 유일한 소스이다 (REQ-M2-05).
	// 업링크가 값을 비워 보내면 기존 캐시를 지우지 않는다.
	if up.DeviceInfo.ApplicationID != "" {
		d.applicationID = up.DeviceInfo.ApplicationID
	}
	d.tags = up.DeviceInfo.Tags
	d.lastSeen = time.Now()
	d.online = true
	a.devicesMu.Unlock()
}

// DownlinkTarget 은 devEui 의 다운링크 대상(캐시된 applicationId + deviceProfileName)을
// 조회한다 (REQ-M2-05, R5 순서 제약).
//
// 최초 업링크가 applicationId 를 캐시하기 전에는 ok=false 이며, 이때 호출자는 다운링크
// 토픽을 구성할 수 없으므로 발행하지 않아야 한다.
//
// HVAC 락 함정 회피(REQ-FROZEN-B): devicesMu 보유 구간에서 a.Name() 등 a.mu 를 다시
// 잡는 메서드를 호출하지 않는다 — devicesMu → a.mu 락 순서 엣지를 새로 만들지 않는다.
func (a *ChirpStackAgent) DownlinkTarget(devEui string) (applicationID, deviceProfileName string, ok bool) {
	if devEui == "" {
		return "", "", false
	}
	a.devicesMu.RLock()
	defer a.devicesMu.RUnlock()

	d, exists := a.devices[devEui]
	if !exists || d.applicationID == "" {
		return "", "", false
	}
	return d.applicationID, d.deviceProfileName, true
}

// listDevices 는 로스터의 디바이스 스냅샷(깊은 복사)을 반환한다.
func (a *ChirpStackAgent) listDevices() []deviceState {
	a.devicesMu.RLock()
	defer a.devicesMu.RUnlock()
	out := make([]deviceState, 0, len(a.devices))
	for _, d := range a.devices {
		out = append(out, d.clone())
	}
	return out
}

// DeviceProvider 는 디바이스 로스터를 device.DeviceProvider 로 노출한다 (REQ-M4-03).
// cmd/xflowd/main.go 의 WithOnStart 인터페이스 어서션이 이 메서드를 감지해
// device 레지스트리에 자동 등록한다.
func (a *ChirpStackAgent) DeviceProvider() device.DeviceProvider {
	return &ChirpStackDeviceProvider{agent: a}
}

// ChirpStackDeviceProvider 는 ChirpStackAgent 를 device.DeviceProvider 로 래핑한다.
type ChirpStackDeviceProvider struct {
	agent *ChirpStackAgent
}

var _ device.DeviceProvider = (*ChirpStackDeviceProvider)(nil)

// Devices 는 모든 자동 생성 디바이스를 통합 Device 슬라이스로 반환한다.
func (p *ChirpStackDeviceProvider) Devices() []device.Device {
	snaps := p.agent.listDevices()
	agentName := p.agent.Name()
	out := make([]device.Device, 0, len(snaps))
	for i := range snaps {
		out = append(out, newChirpDeviceAdapter(agentName, snaps[i]))
	}
	return out
}

// Device 는 글로벌 UUID 로 디바이스를 조회한다. 매칭 실패 시 ErrDeviceNotFound.
func (p *ChirpStackDeviceProvider) Device(id string) (device.Device, error) {
	if strings.TrimSpace(id) == "" {
		return nil, device.ErrDeviceNotFound
	}
	agentName := p.agent.Name()
	for _, s := range p.agent.listDevices() {
		d := newChirpDeviceAdapter(agentName, s)
		if d.UID() == id {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// chirpDeviceAdapter 는 deviceState 를 device.Device 로 노출한다.
type chirpDeviceAdapter struct {
	agentName string
	snap      deviceState
}

var _ device.Device = (*chirpDeviceAdapter)(nil)

func newChirpDeviceAdapter(agentName string, snap deviceState) *chirpDeviceAdapter {
	return &chirpDeviceAdapter{agentName: agentName, snap: snap}
}

// ID 는 글로벌 UUID v4 를 반환한다 (SPEC-DEVICE-IDENTITY-001 Phase D: ID()==UID()).
func (a *chirpDeviceAdapter) ID() string { return a.UID() }

// UID 는 (agentName, devEui) 의 글로벌 UUID v4 를 반환한다. emit 경로(노드 승격)의
// ResolveDeviceID(agentName, devEui) 와 동일 키를 쓰므로 결과 UUID 가 일치한다.
func (a *chirpDeviceAdapter) UID() string {
	return agent.ResolveDeviceID(context.Background(), a.agentName, a.snap.devEui)
}

// Name 은 사용자 표시 이름(deviceName)을 반환한다.
func (a *chirpDeviceAdapter) Name() string { return a.snap.deviceName }

// Type 은 LoRaWAN 센서를 반영한다.
func (a *chirpDeviceAdapter) Type() device.DeviceType { return device.DeviceTypeSensor }

// Protocol 은 "chirpstack" 을 반환한다.
func (a *chirpDeviceAdapter) Protocol() string { return "chirpstack" }

// AgentName 은 owning 에이전트 이름을 반환한다.
func (a *chirpDeviceAdapter) AgentName() string { return a.agentName }

// Online 은 디바이스 online 여부를 반환한다 (comm-state offline 판정은 M5).
func (a *chirpDeviceAdapter) Online() bool { return a.snap.online }

// LastSeen 은 마지막 업링크 수신 시각을 반환한다.
func (a *chirpDeviceAdapter) LastSeen() time.Time { return a.snap.lastSeen }

// State 는 디바이스 상태 스냅샷을 반환한다.
func (a *chirpDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:   a.snap.online,
		LastSeen: a.snap.lastSeen,
	}
}

// Metadata 는 사용자 정의 메타데이터를 반환한다: Name(deviceName) + Labels(tags).
//
// REQ-M4-04: deviceName → Name, tags(location/point/spot) → Labels 로 지속화한다.
// DeviceMetadata.Tags 는 []string 이라 key-value tags 를 담을 수 없으므로, tags 는
// Labels map[string]string 에 verbatim 으로 매핑한다(키 매핑 없음).
func (a *chirpDeviceAdapter) Metadata() device.DeviceMetadata {
	var labels map[string]string
	if len(a.snap.tags) > 0 {
		labels = make(map[string]string, len(a.snap.tags))
		for k, v := range a.snap.tags {
			labels[k] = v
		}
	}
	return device.DeviceMetadata{
		Name:   a.snap.deviceName,
		Labels: labels,
	}
}

// Source 는 자동 발견 디바이스이므로 "auto" 를 반환한다.
func (a *chirpDeviceAdapter) Source() string { return "auto" }

// Capabilities 는 패시브 수신 전용 capability 를 노출한다.
func (a *chirpDeviceAdapter) Capabilities() []string { return []string{"passive-monitor"} }
