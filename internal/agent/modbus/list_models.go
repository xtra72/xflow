package modbus

import (
	"encoding/json"
)

// ---------------------------------------------------------------------------
// SPEC-MODBUS-013 REQ-04 — list_models exec (디바이스 모델 카탈로그 조회)
// ---------------------------------------------------------------------------
//
// 프론트 디바이스 편집 화면의 모델 선택기가 소비한다. 부작용 없는 조회이며,
// 호출 시점에 디렉터리를 다시 스캔하므로 에이전트 재시작 없이 모델 파일 추가가 반영된다.
//
// 응답 형태는 list_devices 표면과 정렬한다:
//   - `data` 는 모델 배열
//   - `model_count` 는 개수
//   - `models_dir` 는 스캔한 디렉터리(비어 있으면 홈 디렉터리 해석 실패)

// processListModels 는 모델 카탈로그 스냅샷을 반환한다(AC-15/AC-16/AC-17/AC-18).
//
// 동작 규약(read-only, HARD): 공유 상태(devices/캐시/통계)를 일절 읽거나 변형하지 않는다.
// 카탈로그는 파일시스템에서만 유래하므로 a.mu 를 잡지 않는다.
func (a *ModbusAgent) processListModels() ([]byte, error) {
	dir := ModelsDir()
	models := LoadDeviceModels(dir, a.logger)

	data := make([]map[string]any, 0, len(models))
	for _, m := range models {
		registers := make([]map[string]any, 0, len(m.Registers))
		for _, r := range m.Registers {
			e := map[string]any{
				"fc":      r.FunctionCode,
				"address": r.Address,
				"count":   r.Count,
				// enabled 는 항상 유효값으로 방출한다(미지정 → true). 프론트가 키 부재를
				// 별도 분기하지 않도록 하기 위함이며 list_devices 규약과 동일하다.
				"enabled": r.Enabled == nil || *r.Enabled,
			}
			if r.DataType != "" {
				e["data_type"] = r.DataType
			}
			// 레지스터가 자체 주기를 지정하지 않으면 모델 기본 주기를 상속시켜 방출한다.
			if r.PollInterval != "" {
				e["poll_interval"] = r.PollInterval
			} else if m.Defaults.PollInterval != "" {
				e["poll_interval"] = m.Defaults.PollInterval
			}
			if r.Name != "" {
				e["name"] = r.Name
			}
			registers = append(registers, e)
		}

		entry := map[string]any{
			"id":             m.ID,
			"name":           m.Name,
			"register_count": len(m.Registers),
			"registers":      registers,
		}
		if m.Vendor != "" {
			entry["vendor"] = m.Vendor
		}
		if m.Description != "" {
			entry["description"] = m.Description
		}
		if m.Transport != "" {
			entry["transport"] = m.Transport
		}
		if m.Defaults.MaxBlockRegisters > 0 {
			entry["max_block_registers"] = m.Defaults.MaxBlockRegisters
		}
		data = append(data, entry)
	}

	return json.Marshal(map[string]any{
		"data":        data,
		"model_count": len(data),
		"models_dir":  dir,
	})
}
