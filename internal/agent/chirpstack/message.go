package chirpstack

import (
	"log/slog"
	"sort"
	"time"
)

// recordKindDeviceState 는 device_state 레코드의 판별자(discriminator) 값이다.
// 노드(buildChirpStackMessage)는 record 필드로 event/device_state 를 구분한다.
// measurementRecord 는 record 를 비워 두므로(omitempty) 노드는 event 로 취급한다.
const recordKindDeviceState = "device_state"

// recordKindMeasurements 는 combined(측정치 통합) 레코드의 판별자 값이다.
// 별도 채널을 만들지 않고 기존 record peek 관례(recordKindDeviceState)를 그대로 확장한다.
const recordKindMeasurements = "measurements"

// comm-state 트리거 종류 (REQ-FROZEN-03). Century 관례(change/report)를 따른다.
const (
	commTriggerChange = "change"
	commTriggerReport = "report"
)

// commStateGroup 은 device_state 이벤트의 nested state 그룹이다 (REQ-FROZEN-03).
//
// online:bool, rssi:int, snr:float, gateway_id:string, last_seen_ms:int64(UnixMilli).
type commStateGroup struct {
	Online     bool    `json:"online"`
	RSSI       int     `json:"rssi"`
	SNR        float64 `json:"snr"`
	GatewayID  string  `json:"gateway_id"`
	LastSeenMs int64   `json:"last_seen_ms"`
}

// deviceStateRecord 는 comm-state fold 스트림의 device_state 중간 레코드이다
// (REQ-FROZEN-03). 노드는 이 레코드를 소비해 type="device_state.<trigger>" 메시지로
// 빌드한다(별도 device_connection 타입 금지).
//
//   - Record: 항상 "device_state" (판별자).
//   - Trigger: "change" | "report" → 노드가 msg.Type 계층 값으로 승격.
//   - UnitID(=devEui): 노드가 promoteDevIDWithUUID 로 device 그룹(UUID/name/type) 승격.
//   - TimeMs/LastSeenMs: int64 UnixMilli (프로젝트 epoch 규약).
type deviceStateRecord struct {
	Record     string         `json:"record"`
	Trigger    string         `json:"trigger"`
	UnitID     string         `json:"unit_id"`
	TimeMs     int64          `json:"time_ms"`
	LastSeenMs int64          `json:"last_seen_ms"`
	State      commStateGroup `json:"state"`
}

// buildDeviceStateRecord 는 comm-state 스냅샷으로부터 device_state 레코드를 만든다.
// last_seen 은 top-level timestamp 소스(TimeMs)와 state 그룹 양쪽에 노출한다
// (Century 관례: state 내부 중복 노출).
func buildDeviceStateRecord(devEui, trigger string, e commEntry) deviceStateRecord {
	return deviceStateRecord{
		Record:     recordKindDeviceState,
		Trigger:    trigger,
		UnitID:     devEui,
		TimeMs:     e.lastSeenMs,
		LastSeenMs: e.lastSeenMs,
		State: commStateGroup{
			Online:     e.online,
			RSSI:       e.rssi,
			SNR:        e.snr,
			GatewayID:  e.gatewayID,
			LastSeenMs: e.lastSeenMs,
		},
	}
}

// measurementRecord 는 에이전트가 노드로 전달하는 measurement 당 1개 중간 레코드이다.
//
// 노드(internal/node/chirpstack.go)는 이 레코드를 소비하여 flow message 로 빌드한다:
//   - type="event", timestamp=UnixMilli(TimeMs)
//   - payload.value=Value, payload.unit_id=UnitID (노드가 device 그룹으로 승격)
//   - metadata.measurement=Measurement, metadata.tags=Tags(verbatim group)
//
// TimeMs 는 프로젝트 규약(payload epoch = int64 UnixMilli)을 따른다.
type measurementRecord struct {
	Measurement string            `json:"measurement"`
	Value       any               `json:"value"`
	UnitID      string            `json:"unit_id"`
	TimeMs      int64             `json:"time_ms"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// combinedMeasurementRecord 는 업링크 1건의 모든 스칼라 measurement 를 하나로 담는
// 레코드이다 (measurement_emit_mode="combined" opt-in 경로).
//
// measurementRecord 와의 차이는 Values(flat map) 하나뿐이다 — unit_id / time_ms /
// tags 는 동일한 의미/값을 갖는다. 노드는 Record 판별자를 보고
// buildChirpStackCombinedMessage 로 분기하여 Values 를 payload 최상위 flat 키로 편다
// (코드베이스 지배적 관례: flattenStateToPayload 계열). measurement 가 하나로 특정되지
// 않으므로 metadata.measurement 는 방출하지 않는다.
type combinedMeasurementRecord struct {
	Record string            `json:"record"`
	Values map[string]any    `json:"values"`
	UnitID string            `json:"unit_id"`
	TimeMs int64             `json:"time_ms"`
	Tags   map[string]string `json:"tags,omitempty"`
}

// buildCombinedMeasurementRecord 는 업링크의 object 를 단일 combined 레코드로 접는다.
//
// per-measurement 경로(buildMeasurementRecords)와 동일한 규칙을 공유한다:
//   - 스칼라 값만 담는다. 비스칼라(중첩 객체/배열)는 동일 경고 로그와 함께 skip.
//   - 결정성을 위해 measurement 키를 정렬해 순회한다(경고 로그 순서 고정).
//   - top-level timestamp / unit_id / tags(verbatim) 는 per-measurement 와 동일.
//
// timeMs 는 호출자(handleUplink)가 업링크 1건마다 1회 확정한 타임스탬프이다 —
// 이 함수가 직접 설정을 읽어 파생하지 않는다(resolveUplinkTimeMs 주석 참조).
//
// 스칼라 measurement 가 하나도 없으면 ok=false 를 반환한다 — 빈 payload 메시지를
// 방출하지 않는다(다운스트림에 의미 없는 이벤트를 흘리지 않음).
func buildCombinedMeasurementRecord(up *uplink, timeMs int64, logger *slog.Logger) (combinedMeasurementRecord, bool) {
	devEui := up.DeviceInfo.DevEui

	keys := make([]string, 0, len(up.Object))
	for k := range up.Object {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	values := make(map[string]any, len(keys))
	for _, k := range keys {
		v := up.Object[k]
		if !isScalar(v) {
			if logger != nil {
				logger.Warn("chirpstack: 비스칼라 measurement skip",
					"measurement", k, "devEui", devEui)
			}
			continue
		}
		values[k] = v
	}
	if len(values) == 0 {
		return combinedMeasurementRecord{}, false
	}

	return combinedMeasurementRecord{
		Record: recordKindMeasurements,
		Values: values,
		UnitID: devEui,
		TimeMs: timeMs,
		Tags:   up.DeviceInfo.Tags,
	}, true
}

// isScalar 는 값이 스칼라(문자열/숫자/불리언 등)인지 판정한다.
// JSON unmarshal 기준으로 중첩 객체(map)와 배열([]any)만 비스칼라이다.
func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

// parseUplinkTimeMs 는 RFC3339(오프셋/소수초 포함) 문자열을 UnixMilli 로 변환한다.
// 빈 문자열/파싱 실패는 0 을 반환한다(호출자가 fallback 처리).
func parseUplinkTimeMs(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// resolveUplinkTimeMs 는 timestamp_source 설정에 따라 업링크 1건의 타임스탬프를 확정한다.
//
// 이 함수는 업링크 처리 경로에서 단 한 번만 호출되며(handleUplink), 그 결과가 그
// 업링크에서 파생되는 모든 레코드(per-measurement / combined / 로스터 measurement
// 캐시)에 그대로 전달된다. 각 빌더가 설정을 따로 읽어 각자 파생하면 세 표면이 서로
// 어긋날 수 있고(특히 server 모드에서는 호출 시점마다 마이크로초가 달라진다), 그
// 불일치는 "flow message 의 시각과 캐시의 시각이 다르다" 는 형태로 소비자에게 드러난다.
// 단일 확정 지점이 그 어긋남을 구조적으로 불가능하게 만든다.
//
//   - server: receivedAt(업링크를 처리하기 시작한 서버 시각)의 UnixMilli.
//     게이트웨이/디바이스 시계가 틀어져 있어도 서버 기준의 일관된 순서를 얻는다.
//   - uplink(기본) 및 그 외 모든 값: 기존 동작 그대로 parseUplinkTimeMs(up.Time).
//     업링크 time 이 없거나 파싱에 실패하면 0 을 반환하며, 0 의 의미(다운스트림이
//     "타임스탬프 없음" 으로 보아 메시지 생성 시각으로 폴백)까지 동결 경로와 동일하다 —
//     여기서 receivedAt 으로 폴백하면 REQ-FROZEN-02 / REQ-FROZEN-A 의 $.timestamp
//     계약이 바뀐다.
func resolveUplinkTimeMs(up *uplink, source string, receivedAt time.Time) int64 {
	if source == timestampSourceServer {
		return receivedAt.UnixMilli()
	}
	return parseUplinkTimeMs(up.Time)
}

// buildMeasurementRecords 는 업링크의 object 를 measurement 당 1개 레코드로 fan-out
// 한다 (REQ-FROZEN-01, REQ-M3-02).
//
//   - 스칼라 값만 방출한다. 비스칼라(중첩 객체/배열)는 skip + 경고 로그(REQ-M3-05).
//   - 결정성을 위해 measurement 키를 정렬한다.
//   - 모든 레코드는 동일 top-level timestamp / unit_id / tags(verbatim) 를 공유한다.
//
// timeMs 는 호출자(handleUplink)가 업링크 1건마다 1회 확정한 타임스탬프이다
// (resolveUplinkTimeMs 주석 참조).
func buildMeasurementRecords(up *uplink, timeMs int64, logger *slog.Logger) []measurementRecord {
	devEui := up.DeviceInfo.DevEui
	tags := up.DeviceInfo.Tags

	keys := make([]string, 0, len(up.Object))
	for k := range up.Object {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]measurementRecord, 0, len(keys))
	for _, k := range keys {
		v := up.Object[k]
		if !isScalar(v) {
			if logger != nil {
				logger.Warn("chirpstack: 비스칼라 measurement skip",
					"measurement", k, "devEui", devEui)
			}
			continue
		}
		out = append(out, measurementRecord{
			Measurement: k,
			Value:       v,
			UnitID:      devEui,
			TimeMs:      timeMs,
			Tags:        tags,
		})
	}
	return out
}
