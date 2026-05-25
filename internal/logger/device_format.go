// Package logger provides shared logging helpers for the xflow daemon.
//
// device_format.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T6)
//
// 본 파일은 디바이스 관련 로그 라인의 표시 형식을 표준화한다.
// SPEC § M5 의 규칙:
//
//   - 로그 라인의 디바이스 표시는 "agent/name" 결합 형식.
//     예) `device "lgcnp/indoor-1" went offline`
//   - UUID 가 필요한 경우 별도 구조화 필드 `device_uid=<uuid>`.
//   - composite key ("agent:local_id") 의 raw 표시는 Phase B 부터 점진적 제거.
//
// Fallback 정책:
//
//   - Device.Name() 이 빈 문자열이면 → "agent/<short_uid>" (UID 의 첫 8자)
//   - UID 도 빈 문자열이면 → "agent/<device_id>" (composite 전체)
//   - AgentName() 도 빈 문자열이면 → "<device_id>" (composite 전체)
//
// 사용 패턴:
//
//	a.logger.Info("device went offline",
//	    "device", logger.FormatDevice(d),
//	    logger.DeviceUIDAttr(d),
//	)
//
//	// 또는 구조화 필드 일괄 추가:
//	a.logger.LogAttrs(ctx, slog.LevelInfo, "device went offline",
//	    logger.DeviceAttrs(d)...)
//
// 본 헬퍼는 그래스풀 디그라데이션을 보장한다 (nil device 입력에서도 panic 없음).

package logger

import (
	"log/slog"
	"strings"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
)

// FormatDevice 는 디바이스를 사람이 읽을 수 있는 "agent/name" 형식으로 포맷한다.
//
// Fallback 우선순위:
//  1. agent != "" AND name != "" → "agent/name"
//  2. agent != "" AND uid != ""  → "agent/<uid_short_8>"
//  3. agent != "" AND id != ""   → "agent/<id>" (composite 의 후반부 추정, B-T9 메트릭 증가)
//  4. uid != ""                  → "<uid>"
//  5. id != ""                   → "<id>" (composite fallback, B-T9 메트릭 증가)
//  6. 그 외 (nil 등)             → "unknown"
//
// 본 함수는 nil 입력에서도 panic 없이 "unknown" 을 반환한다.
//
// SPEC-DEVICE-IDENTITY-001 § B-T9 — composite 형식 (예: "agent:local_id") 으로
// fallback 되면 xflowd_device_composite_use_total{source="log"} 증가.
// agent+name 1급 경로가 정상이면 메트릭 증가 없음.
func FormatDevice(d device.Device) string {
	if d == nil {
		return "unknown"
	}

	agent := d.AgentName()
	name := d.Name()
	uid := d.UID()
	id := d.ID()

	if agent != "" && name != "" {
		return agent + "/" + name
	}
	if agent != "" && uid != "" {
		return agent + "/" + shortUID(uid)
	}
	if agent != "" && id != "" {
		// composite fallback path — id 가 "agent:local_id" 형식이면 메트릭 기록.
		if isCompositeID(id) {
			observe.IncDeviceCompositeUse(observe.CompositeUseSourceLog)
		}
		return agent + "/" + id
	}
	if uid != "" {
		return uid
	}
	if id != "" {
		if isCompositeID(id) {
			observe.IncDeviceCompositeUse(observe.CompositeUseSourceLog)
		}
		return id
	}
	return "unknown"
}

// isCompositeID 는 id 가 v0.x composite key 형식 ("agent:local_id") 인지 판별한다.
// 콜론을 포함하면 composite 로 간주 (느슨한 휴리스틱 — uid 는 슬래시/하이픈 형식,
// composite 는 콜론 형식이므로 안전한 구분).
func isCompositeID(id string) bool {
	return strings.Contains(id, ":")
}

// DeviceUIDAttr 는 디바이스의 UUID 를 `device_uid` 구조화 필드로 반환한다.
//
// UID 가 빈 문자열이면 slog.Attr 의 zero value (no-op) 를 반환한다 — slog
// 가 zero-value attribute 를 자동 생략하므로 호출자가 추가 분기 없이 사용 가능.
//
// nil 입력에서도 panic 없이 zero-value 반환.
func DeviceUIDAttr(d device.Device) slog.Attr {
	if d == nil {
		return slog.Attr{}
	}
	uid := d.UID()
	if uid == "" {
		return slog.Attr{}
	}
	return slog.String("device_uid", uid)
}

// DeviceAttrs 는 디바이스의 구조화 필드 일괄 (device, device_uid, device_agent,
// device_name) 을 반환한다.
//
// 빈 문자열인 필드는 결과에 포함되지 않는다 (slog 호환 — 호출자가 일일이 분기
// 할 필요 없음).
//
// 일반적 사용:
//
//	logger.LogAttrs(ctx, slog.LevelInfo, "device went offline",
//	    logger.DeviceAttrs(d)...)
//
// nil 입력에서도 panic 없이 빈 슬라이스 반환.
func DeviceAttrs(d device.Device) []slog.Attr {
	if d == nil {
		return nil
	}
	attrs := make([]slog.Attr, 0, 4)
	attrs = append(attrs, slog.String("device", FormatDevice(d)))
	if uid := d.UID(); uid != "" {
		attrs = append(attrs, slog.String("device_uid", uid))
	}
	if agent := d.AgentName(); agent != "" {
		attrs = append(attrs, slog.String("device_agent", agent))
	}
	if name := d.Name(); name != "" {
		attrs = append(attrs, slog.String("device_name", name))
	}
	return attrs
}

// shortUID 는 UUID 의 첫 8 자만 반환한다 (사람 친화 fallback 표시용).
// UUID 형식이 아니거나 8자 미만이면 원본을 그대로 반환한다.
func shortUID(uid string) string {
	if len(uid) < 8 {
		return uid
	}
	return uid[:8]
}
