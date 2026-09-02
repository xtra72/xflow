package modbus

import (
	"encoding/json"
)

// ---------------------------------------------------------------------------
// F1 — 백엔드 list_devices (디바이스 목록 조회, REQ-MODBUS-009-01, M1)
// ---------------------------------------------------------------------------
//
// 노드→에이전트 Process() 명령 경로로 현재 디바이스 목록 스냅샷을 반환한다. 부작용 없는 조회이며
// a.mu RLock 스냅샷으로 devices/설정을 안전하게 읽는다(폴링 goroutine 과 경합 없이, AC-02).
//
// 응답 형태(프론트 소비 표면 정렬):
//   - 최상위 `data` 는 디바이스 배열이다. 프론트가 이미 소비 중인 형태(AgentDetailPanel.tsx:3617 의
//     `res.data` 배열, device_id 키)와 정렬한다.
//   - 각 원소는 modbus-client 고유 메타(id/host/port/unit_id/transport/online/share_session/
//     register_groups 요약)를 확장 필드로 포함한다.
//   - `device_count` 는 modbus-gateway list_devices(modbusserver/agent.go:325) 표면과 정렬한다.

// processListDevices 는 현재 디바이스 목록 스냅샷을 반환한다(AC-01/AC-02).
//
// 동작 규약(read-only, HARD):
//   - a.mu RLock 하에서 devices/config 를 읽어 응답을 구성한다. 공유 상태(devices, 트랜스포트,
//     통계 맵)를 일절 변형하지 않는다. dev.config.* 는 set_config/update_device 가 Lock 하에
//     copy-on-write 로 교체하므로 반드시 RLock 하에서 읽는다(processReadRegisters 선례와 동일).
//   - dev.UnitID()(원자값 SSOT)와 dev.IsOnline()(dev 자체 뮤텍스)은 a.mu 와 독립적인 락이므로
//     RLock 보유 중 호출해도 락 순서 역전이 없다(어떤 경로도 dev.mu 보유 중 a.mu 를 잡지 않는다).
//   - 0-device 이면 빈 배열을 오류 없이 반환한다(AC-02).
func (a *ModbusAgent) processListDevices() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cfg := a.config
	data := make([]map[string]any, 0, len(a.devices))
	for _, dev := range a.devices {
		dc := dev.config

		groups := make([]map[string]any, 0, len(dc.RegisterGroups))
		for _, rg := range dc.RegisterGroups {
			g := map[string]any{
				"name":          rg.Name,
				"function_code": rg.FunctionCode,
				"start_address": rg.StartAddress,
				"quantity":      rg.Quantity,
				// enabled 는 항상 유효값(미지정 → true)으로 방출한다. 프론트 체크박스가
				// 키 부재를 별도 분기하지 않도록 하기 위함이다(SPEC-MODBUS-013 REQ-01/REQ-05).
				"enabled": rg.IsEnabled(),
			}
			if rg.DataType != "" {
				g["data_type"] = rg.DataType
			}
			// poll_interval 은 지정된 그룹만 포함한다(0 이면 에이전트 기본 케이던스로 폴백).
			if rg.PollInterval > 0 {
				g["poll_interval"] = rg.PollInterval.String()
			}
			groups = append(groups, g)
		}

		data = append(data, map[string]any{
			"device_id":       dc.ID, // 프론트 소비 키(AgentDetailPanel.tsx:3617)와 정렬.
			"id":              dc.ID, // modbus-client 고유 필드(편집 폼 id 와 정합).
			"host":            dc.Host,
			"port":            dc.Port,
			"unit_id":         dev.UnitID(),                    // 런타임 SSOT(원자값) — 살아있는 값.
			"transport":       effectiveTransportKind(dc, cfg), // 유효 트랜스포트(override ?? 에이전트 기본).
			"online":          dev.IsOnline(),                  // 현재 연결 상태.
			"share_session":   effectiveShareSession(dc, cfg),  // 유효 세션 공유 여부.
			"register_groups": groups,
		})
	}

	resp := map[string]any{
		"data":         data,
		"device_count": len(data),
	}
	return json.Marshal(resp)
}
