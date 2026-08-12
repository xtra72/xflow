package chirpstack

import (
	"log/slog"
	"sort"
	"time"
)

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

// buildMeasurementRecords 는 업링크의 object 를 measurement 당 1개 레코드로 fan-out
// 한다 (REQ-FROZEN-01, REQ-M3-02).
//
//   - 스칼라 값만 방출한다. 비스칼라(중첩 객체/배열)는 skip + 경고 로그(REQ-M3-05).
//   - 결정성을 위해 measurement 키를 정렬한다.
//   - 모든 레코드는 동일 top-level timestamp / unit_id / tags(verbatim) 를 공유한다.
func buildMeasurementRecords(up *uplink, logger *slog.Logger) []measurementRecord {
	timeMs := parseUplinkTimeMs(up.Time)
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
