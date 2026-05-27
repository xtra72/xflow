// Package logger provides shared logging helpers for the xflow daemon.
//
// device_format.go (SPEC-DEVICE-IDENTITY-001 Phase D — D-T14)
//
// 본 파일은 디바이스 관련 로그 라인의 표시 형식을 표준화한다.
// SPEC § M5 의 규칙:
//
//   - 로그 라인의 디바이스 표시는 "agent/name" 결합 형식.
//     예) `device "lg_hvacr01/indoor-1" went offline`
//   - UUID 가 필요한 경우 별도 구조화 필드 `device_uid=<uuid>`.
//   - composite key ("agent:local_id") 의 raw 표시는 Phase D 부터 완전 금지.
//     composite id 가 fallback 으로 들어와도 그 형식을 노출하지 않고
//     "agent/<short_id>" 형태로 단순화한다.
//
// Fallback 정책 (Phase D — composite raw 표시 금지):
//
//   - Device.Name() 이 빈 문자열이면 → "agent/<short_uid>" (UID 의 첫 8자)
//   - UID 도 빈 문자열이면 → "agent/<truncated_id>"
//     (composite 형식이면 콜론 이후만 추출 — agent 가 이미 prefix 이므로 중복 회피)
//   - AgentName() 도 빈 문자열이면 → "<short_uid>" 또는 "<truncated_id>"
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
)

// FormatDevice 는 디바이스를 사람이 읽을 수 있는 "agent/name" 형식으로 포맷한다.
//
// Fallback 우선순위 (Phase D — composite raw 표시 금지):
//  1. agent != "" AND name != "" → "agent/name"
//  2. agent != "" AND uid != ""  → "agent/<uid_short_8>"
//  3. agent != "" AND id != ""   → "agent/<id_after_colon>" (composite 의 후반부)
//  4. uid != ""                  → "<uid>"
//  5. id != ""                   → "<id_after_colon>" (composite 의 후반부)
//  6. 그 외 (nil 등)             → "unknown"
//
// 본 함수는 nil 입력에서도 panic 없이 "unknown" 을 반환한다.
//
// SPEC-DEVICE-IDENTITY-001 § M5 (Phase D § D-T14) — composite ("agent:local_id")
// 의 raw 형식을 더 이상 노출하지 않는다. 콜론을 포함한 id 가 fallback 경로에
// 도달하면 콜론 이후 부분만 추출하여 표시한다.
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
		// composite "agent:local_id" 가 들어오면 local_id 만 추출 — agent 중복 회피
		// 및 raw composite 표시 금지 정책 (Phase D § D-T14).
		return agent + "/" + stripCompositePrefix(id)
	}
	if uid != "" {
		return uid
	}
	if id != "" {
		return stripCompositePrefix(id)
	}
	return "unknown"
}

// stripCompositePrefix 는 composite id ("agent:local_id") 에서 "agent:" 접두사를
// 제거하여 local_id 만 반환한다. 콜론이 없으면 입력 그대로 반환.
//
// 예시:
//   - "lg_icp01:81"       → "81"
//   - "century:bus0:3b" → "bus0:3b" (첫 콜론만 제거 — agent 접두사 정의)
//   - "plain-id"       → "plain-id"
func stripCompositePrefix(id string) string {
	if idx := strings.IndexByte(id, ':'); idx >= 0 {
		return id[idx+1:]
	}
	return id
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
