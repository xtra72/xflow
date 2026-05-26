// influxdb_dualtag.go (SPEC-DEVICE-IDENTITY-001 Phase C § C3)
//
// 본 파일은 InfluxDBAgent.Process 의 validateWriteData 직후, client.Write 직전에
// WriteData.Tags 에 dual-tag (composite + uid) 를 부착하는 헬퍼 로직을 제공한다.
//
// 설계 결정:
//   - 위치: agent.Process 단계 — v2/v3 client 양쪽에 자동 적용되며 client 코드
//     무수정 (client 순수성 유지).
//   - lookup: device.DeviceRegistry.ResolveDevice(ref) — Phase B1 의 인프라
//     재사용. composite / UUID / agent-name 자동 dispatch.
//   - Soft Deprecation: composite tag 는 그대로 유지 — 외부 쿼리/대시보드
//     호환 보장. uid tag 만 추가한다.
//   - 메트릭: 모든 분기에서 IncTSDBDualTag(state) 호출 — 마이그레이션 진척
//     추적 (xflowd_tsdb_dual_tag_total).
//
// 4 state 분류:
//   - both           : composite source 발견 + ResolveDevice 성공 → uid 부착.
//   - composite_only : composite source 발견 + ResolveDevice 실패 (unmapped).
//   - uid_only       : 이미 uid tag 보유 → 변경 없음 (idempotent).
//   - unmapped       : composite source 자체가 없음 → 변경 없음 (정보용).

package system

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
)

// DeviceResolver 는 dual-tag 부착에 필요한 최소 디바이스 lookup 인터페이스이다.
//
// device.DeviceRegistry 가 본 인터페이스를 자연스럽게 만족한다 (ResolveDevice
// 메서드 보유). 별도 인터페이스로 분리하여 (1) 테스트 시 mock 주입 용이, (2)
// device 패키지에 대한 직접 의존을 최소화한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B1 의 device.Registry.ResolveDevice 와 동일
// 시그니처.
type DeviceResolver interface {
	ResolveDevice(ref string) (device.Device, device.DeviceRefKind, error)
}

// uidTagKey 는 부착되는 UUID tag 의 key 이다. SPEC § 6.2 emit payload 진화
// 규약에 따라 "uid" 로 정규화한다 (Phase A/B 의 emit 필드명과 일관).
const uidTagKey = "uid"

// augmentWriteDataWithUID 는 WriteData.Tags 를 검사하여 dual-tag 부착을 수행하고
// 메트릭에 기록할 state 라벨을 반환한다.
//
// 동작 규칙:
//  1. wd.Tags 에 "uid" 키가 이미 있으면 → "uid_only" 반환 (변경 없음, idempotent).
//  2. sourceKeys 순회하며 매칭되는 첫 번째 tag value 를 찾는다.
//     - 매칭 없음 → "unmapped" 반환 (변경 없음).
//  3. 찾은 value 로 resolver.ResolveDevice 호출.
//     - 에러 또는 빈 UID → "composite_only" 반환 (composite 만 유지).
//     - 성공 + 유효 UID → wd.Tags["uid"] 부착 + "both" 반환.
//
// 호출자 사전조건:
//   - wd != nil. Tags 가 nil 이면 함수가 안전하게 nil 검사 후 처리.
//   - resolver 가 nil 이면 호출자가 함수 호출 자체를 건너뛰어야 한다.
//   - sourceKeys 가 비어 있으면 항상 "unmapped" 를 반환한다.
//
// 본 함수는 wd 를 in-place 수정한다 (Tags map 에 새 키 추가만 발생).
func augmentWriteDataWithUID(wd *WriteData, resolver DeviceResolver, sourceKeys []string) string {
	if wd == nil {
		return observe.TSDBDualTagStateUnmapped
	}

	// (1) 이미 uid tag 보유 → idempotent skip.
	if _, exists := wd.Tags[uidTagKey]; exists {
		return observe.TSDBDualTagStateUIDOnly
	}

	// (2) source key 우선순위 매칭.
	var compositeValue string
	var found bool
	for _, key := range sourceKeys {
		if key == "" {
			continue
		}
		if v, ok := wd.Tags[key]; ok && v != "" {
			compositeValue = v
			found = true
			break
		}
	}
	if !found {
		return observe.TSDBDualTagStateUnmapped
	}

	// (3) Resolver lookup.
	dev, _, err := resolver.ResolveDevice(compositeValue)
	if err != nil || dev == nil {
		return observe.TSDBDualTagStateCompositeOnly
	}
	uid := dev.UID()
	if uid == "" {
		// Phase A graceful degradation — UID() 가 빈 문자열인 경우.
		// composite 만 유지 (uid 부착하지 않음).
		return observe.TSDBDualTagStateCompositeOnly
	}

	// Tags map 이 nil 이면 안전을 위해 lazy 초기화.
	// 단 validateWriteData 단계에서 일반적으로 Tags 가 비어 있어도 nil 은 흔치 않다.
	if wd.Tags == nil {
		wd.Tags = make(map[string]string, 1)
	}
	wd.Tags[uidTagKey] = uid
	return observe.TSDBDualTagStateBoth
}
