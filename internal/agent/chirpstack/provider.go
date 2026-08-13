package chirpstack

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
)

// maxCachedMeasurements 는 디바이스당 유지하는 서로 다른 measurement 키의 상한이다.
//
// 로스터는 프로세스 수명 동안 유지되므로, 업링크가 매번 새로운 키를 실어 오는
// (오작동/악의적) 디바이스가 있으면 캐시가 무한히 자란다. 상한 도달 후 새 키는
// 무시하고 기존 키의 값 갱신은 계속 허용한다 — 정상 센서(수 개~수십 개 measurement)
// 는 이 상한에 닿지 않는다.
const maxCachedMeasurements = 64

// ChirpStack 태그 키 중 전용 메타데이터 필드로 승격(promote)하는 키와, 에이전트가
// 소유하는 예약 라벨 키.
//
// 키 매칭 규칙(결정): 정확히 소문자 "group" / "location" 만 승격한다. 대소문자
// 무시(case-insensitive) 매칭은 채택하지 않았다 —
//
//	(1) flow message 의 $.metadata.tags.* 는 태그를 대소문자 그대로 verbatim 통과시키는
//	    동결 계약(REQ-FROZEN-A / REQ-FROZEN-02)이다. 로스터만 대소문자를 접으면 같은
//	    태그 키가 두 표면에서 서로 다르게 해석된다.
//	(2) Go 맵 키는 유일하므로 정확 매칭은 충돌이 구조적으로 불가능하고 결과가 항상
//	    결정적이다. 반면 case-insensitive 는 "Location" 과 "location" 이 함께 오는
//	    사용자 입력에서 임의의 tie-break 가 필요하고, 어떤 tie-break 든 사용자 데이터
//	    하나를 조용히 삼킨다.
//	(3) 실측 데이터의 태그 키는 소문자({location, point, spot})이며 ChirpStack 의
//	    자체 관례와도 일치한다 — 정확 매칭이 현행 동작을 보존한다.
//	(4) 실패 양상이 양성(benign)이다: "Location" 태그는 그냥 Labels 에 남는다. 값이
//	    사라지지 않고 사용자 눈에 보이므로 태그 이름을 고치면 된다.
const (
	tagKeyGroup    = "group"
	tagKeyLocation = "location"

	// labelKeyDevEui 는 에이전트가 소유하는 예약 라벨 키이다(Metadata().Labels).
	// 사용자 ChirpStack 태그와 Labels 를 공유하므로, 같은 이름의 사용자 태그가 오면
	// 에이전트 값이 정본이며 사용자 태그를 덮어쓴다(upsertDevice 에서 Warn).
	labelKeyDevEui = "dev_eui"
)

// measurementSample 은 measurement 1개의 마지막 값과 그 값이 갱신된 시각이다.
//
// 캐시가 병합(merge) 의미이므로 — 업링크가 실어온 키만 갱신되고 나머지는 그대로
// 남는다 — 한 디바이스의 measurement 들은 서로 신선도가 크게 다를 수 있다. 값만
// 노출하면 "이 온도는 방금 값인가, 사흘 전 값인가"를 구분할 방법이 없으므로 값과
// 갱신 시각을 한 쌍으로 유지한다.
//
// timeMs 는 int64 epoch milliseconds(프로젝트 timestamp 규약)이며, 벽시계가 아니라
// 업링크에서 파생한 시각이다(upsertDevice 참조) — flow message 의 $.timestamp 와
// 같은 소스라 두 표면이 서로 어긋나지 않는다.
type measurementSample struct {
	value  any
	timeMs int64
}

// deviceState 는 devEui 로 키잉되는 자동 생성 디바이스의 런타임 스냅샷이다.
//
// applicationID 는 다운링크 토픽 구성에 필요한 ChirpStack applicationId 캐시이다
// (SPEC-CHIRPSTACK-002 REQ-M2-05). comm 맵이 아닌 로스터(devices)에 캐시하는 이유:
// upsertDevice 는 모든 업링크마다 실행되는 반면 comm 맵 갱신은 emit_comm_state 노브
// 뒤에 게이팅되므로, comm 맵에 캐시하면 제어 경로가 그 노브에 숨은 의존을 갖게 된다
// (REQ-M4-04 의 "새 노브 없음" 취지에 반함).
//
// measurements 도 같은 이유로 로스터에 캐시한다 — emit_comm_state 와 무관하게
// 모든 업링크에서 갱신되어야 "이 디바이스의 현재 온도" 질의에 답할 수 있다.
//
// online 불리언 필드는 제거되었다: 저장된 값을 false 로 되돌리는 주체가 없어 영구
// online 으로 굳는 결함의 원인이었고, 이제 lastSeen + offline 임계에서 파생한다
// (deviceOnline 참조). 남겨 두면 오해를 부르는 죽은 상태가 된다.
type deviceState struct {
	devEui            string
	deviceName        string
	deviceProfileName string
	applicationID     string
	tags              map[string]string
	lastSeen          time.Time

	// measurements 는 디바이스가 마지막으로 보고한 스칼라 measurement 캐시이다.
	// 값 하나가 아니라 (값, 갱신 시각) 쌍을 담는다 — measurementSample 주석 참조.
	measurements map[string]measurementSample

	// links 는 gatewayId 키 (device, gateway) 링크 캐시이다
	// (SPEC-CHIRPSTACK-003 REQ-M2-01/02). comm 맵이 아니라 로스터에 두는 이유는
	// applicationID / measurements 와 동일하다 — upsertDevice 는 모든 업링크마다
	// 무조건 실행되지만 comm 맵 갱신은 emit_comm_state(기본 false) 게이트 뒤에
	// 있으므로, comm 맵에 두면 그 노브가 꺼진 에이전트에서 게이트웨이 조회가
	// 조용히 빈 결과를 낸다.
	//
	// 게이트웨이 로스터는 이 맵을 질의 시점에 역인덱싱해 파생한다 — 두 번째 권위
	// 맵을 두지 않으므로 두 표면이 갈라질 수 없고, 신규 mutex/락 순서 엣지도
	// 생기지 않는다 (REQ-M2-04, REQ-FROZEN-B).
	links map[string]gatewayLink
}

// clone 은 tags/measurements/links 맵을 포함해 깊은 복사한다 (roster 스냅샷용).
//
// 얕은 복사이면 호출자가 락 보호 상태 내부를 가리키는 참조를 받게 되어 데이터 레이스가
// 된다 — listDevices 의 "깊은 복사" 계약은 세 맵 모두에 적용된다.
func (d *deviceState) clone() deviceState {
	cp := *d
	if d.tags != nil {
		cp.tags = make(map[string]string, len(d.tags))
		for k, v := range d.tags {
			cp.tags[k] = v
		}
	}
	if d.measurements != nil {
		// measurementSample 은 값 타입이므로 대입만으로 복사된다. 값(any)에는 스칼라만
		// 담기므로(scalarMeasurementKeys 가 비스칼라를 걸러낸다) 내부 참조가 없다 —
		// 별도 재귀 복사가 필요하지 않다.
		cp.measurements = make(map[string]measurementSample, len(d.measurements))
		for k, v := range d.measurements {
			cp.measurements[k] = v
		}
	}
	if d.links != nil {
		// gatewayLink 은 스칼라 필드만 갖는 값 타입이므로 대입만으로 복사된다 —
		// 내부 참조가 없어 재귀 복사가 필요하지 않다. 다만 맵 자체는 반드시 새로
		// 만들어야 한다(공유하면 호출자가 devicesMu 보호 상태를 직접 가리킨다).
		cp.links = make(map[string]gatewayLink, len(d.links))
		for k, v := range d.links {
			cp.links[k] = v
		}
	}
	return cp
}

// mergeMeasurements 는 이번 업링크의 스칼라 값을 measurement 캐시에 병합한다.
//
// 병합 의미(덮어쓰기 아님): 이번 업링크가 실어온 키만 갱신하고 나머지 키는 그대로
// 둔다. 매 업링크마다 temperature 를 보내지만 battery 는 1시간에 한 번만 보내는
// 디바이스가 battery 값을 잃지 않도록 하기 위함이다.
//
// keys 는 호출자가 락 밖에서 미리 걸러/정렬한 스칼라 키 목록이다 — 상한(cap) 도달 시
// 어떤 키가 채택되는지가 맵 순회 순서에 좌우되지 않도록 결정적으로 만든다.
//
// timeMs 는 이번 업링크에서 파생한 갱신 시각이며, 이번에 실어온 키에만 기록된다.
// 갱신되지 않은 키는 값뿐 아니라 이전 timeMs 도 그대로 유지한다 — 그것이 키별
// 신선도를 구분 가능하게 만드는 지점이다.
func (d *deviceState) mergeMeasurements(obj map[string]any, keys []string, timeMs int64) {
	if len(keys) == 0 {
		return
	}
	if d.measurements == nil {
		d.measurements = make(map[string]measurementSample, len(keys))
	}
	for _, k := range keys {
		if _, exists := d.measurements[k]; !exists && len(d.measurements) >= maxCachedMeasurements {
			continue // 상한 도달 — 새 키는 무시하되 기존 키 갱신은 위 조건에서 통과한다.
		}
		d.measurements[k] = measurementSample{value: obj[k], timeMs: timeMs}
	}
}

// scalarMeasurementKeys 는 업링크 object 에서 스칼라 값의 키만 골라 정렬해 반환한다.
//
// 스칼라 판정은 emit 경로(buildMeasurementRecords / buildCombinedMeasurementRecord)와
// 동일한 isScalar 를 재사용한다 — 두 번째 술어를 만들면 캐시와 방출이 조용히 갈라진다.
// 비스칼라(중첩 객체/배열)는 emit 경로와 똑같이 skip 된다.
func scalarMeasurementKeys(obj map[string]any) []string {
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k, v := range obj {
		if isScalar(v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// deviceOnline 은 lastSeen 과 offline 임계로 online 여부를 파생한다.
//
// 저장된 불리언 대신 파생하는 이유: upsertDevice 가 online=true 를 쓰고 이를 false 로
// 되돌리는 주체가 없어 로스터가 영구 online 으로 굳었다(chirpstack-status 는 offline
// 이라 보고하는데 inventory 는 online 이라 보고하는 불일치). watchdog 이 로스터를
// 쓰게 하는 대안은 (a) devicesMu ↔ commMu 락 순서 엣지를 새로 만들고, (b)
// emit_comm_state=false 이면 watchdog 자체가 기동하지 않아(watchdog.go) 여전히 굳는다.
// 파생은 두 문제를 모두 구조적으로 제거하며 드리프트가 불가능하다.
//
// 판정 방향은 watchdog 의 checkStaleness 와 동일하다(경과 > 임계 이면 offline).
//
// zero-threshold 정책: threshold<=0(미설정/0)이면 defaultOfflineThreshold(300s)로
// 폴백한다. 0 을 그대로 임계로 쓰면 "경과 > 0" 이 즉시 참이 되어 방금 업링크를 받은
// 디바이스까지 전부 조용히 offline 으로 뒤집히므로, 그 실패 양상을 금지한다.
func deviceOnline(lastSeen time.Time, threshold time.Duration) bool {
	if lastSeen.IsZero() {
		return false // 업링크 수신 전 — unknown 을 online 으로 보고하지 않는다.
	}
	if threshold <= 0 {
		threshold = defaultOfflineThreshold
	}
	return time.Since(lastSeen) <= threshold
}

// upsertDevice 는 업링크의 디바이스를 devEui 키로 자동 생성/갱신하고, 시스템 UID
// 발급 및 런타임 DeviceInfo 등록을 수행한다 (REQ-M4-01/02/04).
//
//   - ResolveDeviceID(agentName, devEui): 시스템 UID(UUID v4) 발급/조회
//     (저장소 미설정 시 "" — graceful).
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

	// measurement 캐시용 스칼라 키 선별/정렬도 락 밖에서 수행한다 (락 보유 구간 최소화).
	mkeys := scalarMeasurementKeys(up.Object)

	now := time.Now()

	// measurement 갱신 시각은 emit 경로(buildMeasurementRecords /
	// buildCombinedMeasurementRecord)와 동일한 parseUplinkTimeMs 로 업링크에서 파생한다.
	// time.Now() 를 쓰면 같은 업링크가 만든 flow message 의 $.timestamp 와 로스터의
	// time_ms 가 어긋나, 두 표면을 대조하는 소비자가 두 개의 서로 다른 "측정 시각"을
	// 보게 된다.
	//
	// 폴백(업링크 time 이 없거나 RFC3339 파싱 실패 → parseUplinkTimeMs 가 0):
	// 수신 시각(now)을 쓴다. 0 을 그대로 두면 모든 측정치가 1970 으로 보여 신선도
	// 비교라는 이 필드의 목적 자체가 무너진다. 수신 시각은 flow message 와도 어긋나지
	// 않는다 — 노드는 time_ms<=0 이면 timestamp 를 세팅하지 않고 message.New() 의
	// 기본값(수신 시각)을 쓰므로(buildChirpStackMessage), 양쪽 모두 수신 시각이 된다.
	timeMs := parseUplinkTimeMs(up.Time)
	if timeMs <= 0 {
		timeMs = now.UnixMilli()
	}

	// (device, gateway) 링크 샘플 산출도 락 밖에서 수행한다 (REQ-M2-06):
	// rxInfo 전량을 gatewayId 로 키잉하고 프레임 레벨 txInfo 를 부착한 뒤
	// gatewayId 오름차순으로 정렬한다 — cap 도달 시 어떤 신규 게이트웨이가
	// 입장하는지가 rxInfo 배열 순서에 좌우되지 않도록 만드는 지점이다
	// (scalarMeasurementKeys 가 measurement 키에 대해 하는 것과 동형).
	links := buildGatewayLinks(up, timeMs)

	// 예약 라벨 키 충돌 경고: 사용자 ChirpStack 태그가 dev_eui 라는 이름을 쓰면
	// Metadata().Labels 에서 에이전트 값이 덮어쓴다(Metadata 참조). 조용히 삼키지 않고
	// 디바이스를 지목해 알린다. 읽기(Metadata) 경로가 아니라 여기(수집 경로)에서 내는
	// 이유: Metadata() 는 API 조회마다 호출되므로 로그가 범람한다.
	if _, collides := up.DeviceInfo.Tags[labelKeyDevEui]; collides && a.logger != nil {
		a.logger.Warn("chirpstack: 예약 라벨 키 충돌 — 사용자 태그 dev_eui 를 에이전트 값으로 덮어씀",
			"devEui", devEui, "deviceName", up.DeviceInfo.DeviceName)
	}

	// 시스템 UID 발급/조회 + 런타임 device_type/label 등록.
	//
	// device_id 는 저장소가 발급하는 UUID v4 이며 에이전트가 프로토콜 식별자(devEui)로
	// 덮어쓰지 않는다 — UUID 형식은 API 경계에서 강제되는 플랫폼 불변식이다
	// (SPEC-DEVICE-IDENTITY-001 Phase D, device.ClassifyDeviceRef). devEui 는 id 가
	// 아니라 디바이스 정보(Metadata().Labels["dev_eui"])로 노출한다(Metadata 참조).
	//
	// 저장소 I/O 이므로 어떤 락도 보유하지 않은 채 호출한다(REQ-FROZEN-B).
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
	d.lastSeen = now
	// 최신 measurement 캐시 병합 (online 은 lastSeen 에서 파생하므로 저장하지 않는다).
	d.mergeMeasurements(up.Object, mkeys, timeMs)
	// (device, gateway) 링크 캐시 병합 (SPEC-CHIRPSTACK-003 REQ-M2-01/02/03).
	// links 는 락 밖에서 미리 만들어 정렬한 값 슬라이스이므로, 락 보유 구간은
	// 맵 갱신뿐이다.
	d.mergeGatewayLinks(links)
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

// deviceAdapters 는 로스터 스냅샷과 comm 스냅샷을 병합해 어댑터 목록을 만든다.
//
// 락 규율(REQ-FROZEN-B) — devicesMu 와 commMu 를 절대 중첩하지 않는다:
//
//	(1) 락 보유 전에 agentName(a.mu) 과 offline 임계(atomic)를 선캡처한다.
//	    → 락 보유 중 a.Name() 재-lock 을 유발하지 않는다 (v0.18.6 HVAC 재귀 RLock 트랩).
//	(2) devicesMu 를 잡아 로스터를 깊은 복사하고 즉시 해제한다 (listDevices).
//	(3) commMu 를 잡아 comm 엔트리를 값 복사하고 즉시 해제한다 (commSnapshots).
//	(4) 병합은 어떤 락도 보유하지 않은 채 수행한다.
//
// 두 락이 동시에 보유되는 지점이 없으므로 둘 사이에 락 순서 엣지 자체가 생기지 않는다
// (watchdog 의 "락 해제 후 emit" 규율과 동형).
func (p *ChirpStackDeviceProvider) deviceAdapters() []*chirpDeviceAdapter {
	a := p.agent
	agentName := a.Name()                // (1) 락 보유 전 캡처.
	threshold := a.cs().OfflineThreshold // atomic 스냅샷 — 락 없음.

	snaps := a.listDevices()   // (2) devicesMu 획득 → 복사 → 해제.
	comms := a.commSnapshots() // (3) commMu 획득 → 복사 → 해제.

	out := make([]*chirpDeviceAdapter, 0, len(snaps)) // (4) 락 밖 병합.
	for i := range snaps {
		var ce *commEntry
		if e, ok := comms[snaps[i].devEui]; ok {
			cp := e // 값 복사본의 주소 — comm 맵 내부 포인터를 노출하지 않는다.
			ce = &cp
		}
		out = append(out, newChirpDeviceAdapter(agentName, snaps[i], threshold, ce))
	}
	return out
}

// commSnapshots 는 comm 맵 전체를 값 복사 맵으로 반환한다 (commMu 만 사용).
//
// CommSnapshot(단건)의 전량 조회 형태이며, 동일한 규율을 따른다: commMu 보유 구간에서
// a.Name() 등 a.mu 를 다시 잡는 메서드를 호출하지 않고, 내부 포인터(*commEntry)도
// 노출하지 않는다.
//
// emit_comm_state=false 이면 comm 맵이 아예 채워지지 않으므로 빈 맵이 반환된다 —
// 호출자에게는 "엔트리 부재"로 자연스럽게 나타난다(별도 게이트 불필요).
func (a *ChirpStackAgent) commSnapshots() map[string]commEntry {
	a.commMu.Lock()
	defer a.commMu.Unlock()
	out := make(map[string]commEntry, len(a.comm))
	for devEui, e := range a.comm {
		if e != nil {
			out[devEui] = *e
		}
	}
	return out
}

// Devices 는 모든 자동 생성 디바이스를 통합 Device 슬라이스로 반환한다.
func (p *ChirpStackDeviceProvider) Devices() []device.Device {
	adapters := p.deviceAdapters()
	out := make([]device.Device, 0, len(adapters))
	for _, d := range adapters {
		out = append(out, d)
	}
	return out
}

// Device 는 글로벌 UUID 로 디바이스를 조회한다. 매칭 실패 시 ErrDeviceNotFound.
func (p *ChirpStackDeviceProvider) Device(id string) (device.Device, error) {
	if strings.TrimSpace(id) == "" {
		return nil, device.ErrDeviceNotFound
	}
	for _, d := range p.deviceAdapters() {
		if d.UID() == id {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// chirpDeviceAdapter 는 deviceState 를 device.Device 로 노출한다.
//
// offlineThreshold 는 online 파생(deviceOnline)에 쓰는 설정 스냅샷이고, comm 은 링크
// 품질(rssi/snr/gateway_id) 스냅샷이다. comm 은 nil 일 수 있으며 nil 은 "정보 없음"을
// 뜻한다 — zero-value 로 채우지 않는다.
type chirpDeviceAdapter struct {
	agentName        string
	snap             deviceState
	offlineThreshold time.Duration
	comm             *commEntry
}

var _ device.Device = (*chirpDeviceAdapter)(nil)

func newChirpDeviceAdapter(agentName string, snap deviceState, offlineThreshold time.Duration, comm *commEntry) *chirpDeviceAdapter {
	return &chirpDeviceAdapter{
		agentName:        agentName,
		snap:             snap,
		offlineThreshold: offlineThreshold,
		comm:             comm,
	}
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

// Online 은 디바이스 online 여부를 lastSeen + offline 임계에서 파생해 반환한다.
// 저장된 불리언이 아니므로 오래된 디바이스가 영구 online 으로 굳지 않는다
// (파생 근거는 deviceOnline 주석 참조).
func (a *chirpDeviceAdapter) Online() bool {
	return deviceOnline(a.snap.lastSeen, a.offlineThreshold)
}

// LastSeen 은 마지막 업링크 수신 시각을 반환한다.
func (a *chirpDeviceAdapter) LastSeen() time.Time { return a.snap.lastSeen }

// State 는 디바이스 상태 스냅샷을 반환한다 (Properties 는 protocol 고유 속성).
func (a *chirpDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.Online(),
		LastSeen:   a.snap.lastSeen,
		Properties: a.properties(),
	}
}

// properties 는 protocol 고유 속성 맵을 만든다: 링크 품질(rssi/snr/gateway_id)
// + 최신 measurement 캐시.
//
// dev_eui 는 여기 없다(의도적): devEui 는 변하지 않는 디바이스 식별 정보이지 런타임
// 상태가 아니므로 Metadata().Labels 로 옮겼다(Metadata 참조). flow message 의
// metadata.device.dev_eui(노드의 setChirpStackDevEui)는 별개의 표면이며 그대로 남는다
// — 본 이동은 로스터 사본만 옮긴다.
//
// 노출 경로 선택 근거: device.DeviceState 는 이미 범용 Properties map[string]any 를
// 갖고 있으므로 공유 구조체를 확장하지 않는다 — 다른 프로바이더(xsfm/century/lg/
// samsung/modbus/thingplus…)의 blast radius 가 0 이다. Metadata().Labels 는
// map[string]string 이라 숫자/불리언 measurement 를 문자열로 변환해야 해 손실적이므로
// measurement 에는 쓰지 않는다(문자열인 dev_eui 는 반대로 Labels 가 맞다).
// inventory 노드는 state.properties 를 이미 그대로 직렬화한다.
//
// 부재 표현(중요): comm 엔트리가 없으면(emit_comm_state=false 이거나 아직 미수신)
// rssi/snr/gateway_id 키를 아예 넣지 않는다. rssi:0 은 "미상"과 "실제 0dBm"을 구분할
// 수 없는 데이터 품질 함정이므로 zero-value 로 채우지 않는다.
//
// measurements 는 키 충돌(예: "rssi" 라는 이름의 measurement)을 피하려고 중첩 맵으로
// 네임스페이스한다. 캐시가 비어 있으면 키 자체를 생략한다. measurement 하나는
// {value, time_ms} 객체이다 — 병합 캐시라 키마다 신선도가 다르기 때문이다
// (measurementSample 주석 참조).
//
// 깊은 복사: 스냅샷의 맵을 그대로 넘기지 않고, 바깥 맵과 measurement 당 안쪽 맵을
// 모두 새로 만든다. 어댑터는 여러 번 State() 호출에 재사용될 수 있으므로, 호출자가
// 받은 구조를 어느 깊이에서 변조하든 다음 호출 결과가 오염되지 않아야 한다.
func (a *chirpDeviceAdapter) properties() map[string]any {
	props := make(map[string]any, 4)
	if a.comm != nil {
		props["rssi"] = a.comm.rssi
		props["snr"] = a.comm.snr
		props["gateway_id"] = a.comm.gatewayID
	}
	if len(a.snap.measurements) > 0 {
		m := make(map[string]any, len(a.snap.measurements))
		for k, s := range a.snap.measurements {
			m[k] = map[string]any{"value": s.value, "time_ms": s.timeMs}
		}
		props["measurements"] = m
	}
	return props
}

// Metadata 는 사용자 정의 메타데이터를 반환한다: Name(deviceName) + Group/Location
// (승격된 태그) + Labels(나머지 태그 + 예약 dev_eui).
//
// REQ-M4-04 는 tags 를 Labels 에 verbatim 매핑했으나, DeviceMetadata 에는 이미
// Group/Location 전용 필드가 있어 group/location 태그가 범용 라벨 더미에 묻혀 있었다.
// 이제 그 둘만 전용 필드로 승격하고 Labels 에서는 제거한다(복사가 아니라 이동 —
// 같은 값이 두 곳에 있으면 어느 쪽이 정본인지 모호해진다). 승격 키 매칭 규칙과
// 그 근거는 tagKeyGroup/tagKeyLocation 상수 주석 참조.
//
// group 태그가 없으면 Group 은 빈 문자열로 남긴다 — deviceProfileName 등으로 값을
// 지어내지 않는다. 사용자가 지정하지 않은 그룹을 시스템이 발명하면 그룹 기반 조회가
// 조용히 거짓 결과를 낸다.
//
// dev_eui 예약 키: 로스터 엔트리의 키 자체이므로 이 디바이스가 존재하는 한 반드시
// 알려져 있고, "미상" 상태가 없어 게이팅하지 않는다. Labels 는 이제 에이전트 소유
// 값과 사용자 태그가 공유하는 공간이므로, 사용자 태그를 먼저 쓰고 dev_eui 를 마지막에
// 써서 에이전트 값이 정본이 되도록 고정한다(충돌 경고는 upsertDevice 에서).
func (a *chirpDeviceAdapter) Metadata() device.DeviceMetadata {
	md := device.DeviceMetadata{Name: a.snap.deviceName}

	var labels map[string]string
	for k, v := range a.snap.tags {
		switch k {
		case tagKeyGroup:
			md.Group = v
		case tagKeyLocation:
			md.Location = v
		default:
			if labels == nil {
				labels = make(map[string]string, len(a.snap.tags)+1)
			}
			labels[k] = v
		}
	}

	// 예약 키는 사용자 태그 이후에 기록한다 — 순서가 곧 우선순위이다.
	if a.snap.devEui != "" {
		if labels == nil {
			labels = make(map[string]string, 1)
		}
		labels[labelKeyDevEui] = a.snap.devEui
	}

	md.Labels = labels
	return md
}

// Source 는 자동 발견 디바이스이므로 "auto" 를 반환한다.
func (a *chirpDeviceAdapter) Source() string { return "auto" }

// Capabilities 는 패시브 수신 전용 capability 를 노출한다.
func (a *chirpDeviceAdapter) Capabilities() []string { return []string{"passive-monitor"} }
