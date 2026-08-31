package modbus

// ---------------------------------------------------------------------------
// 런타임 디바이스 로스터 영속화 (SPEC-MODBUS-013 M6)
// ---------------------------------------------------------------------------
//
// add_device / update_device / remove_device 는 메모리 상태(a.devices)만 바꾼다.
// 저장소에 반영되지 않으면 에이전트를 재시작할 때 런타임에 등록한 디바이스가 사라진다.
//
// API 계층의 persistDeviceRosterAfterExec 가 exec 성공 후 이 함수를 호출해
// AgentConfig.Transport.Options["devices"] 를 갱신하고 repo.Save 한다.
// xsfm/samsung/lgap 는 agent.DeviceEntry(주소 중심 평탄 구조)로 충분하지만,
// modbus 디바이스는 register_groups·type_map·시리얼 오버라이드까지 담아야 하므로
// parseDeviceConfig 가 그대로 소비하는 맵 형상을 직접 만든다(왕복 무손실).

// GetPersistableDeviceConfigs 는 현재 디바이스 로스터를 설정 파일 형상으로 반환한다.
//
// 숫자는 모두 int 로 방출한다. 저장소를 거치면 JSON/YAML 왕복으로 어차피 일반 수치 타입이
// 되고, byte/uint16 같은 좁은 타입은 파서의 toByte/toUint16 이 해석하지 못해 조용히 0 이 된다
// (function_code 0 → "must be 1, 2, 3, or 4" 로 복원 실패).
//
// 반환값은 parseDeviceConfig 의 입력과 정확히 같은 구조이므로, 저장 후 재시작하면
// 동일한 디바이스 집합이 복원된다. 선택 필드는 설정된 경우에만 키를 포함해
// 기존 설정 파일과의 차이를 최소화한다(미지정 → 키 생략 → 기본값 적용).
func (a *ModbusAgent) GetPersistableDeviceConfigs() []any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make([]any, 0, len(a.devices))
	for _, dev := range a.devices {
		dc := dev.config

		entry := map[string]any{
			"id":      dc.ID,
			"unit_id": int(dev.UnitID()), // 런타임 SSOT(원자값) — update_device 로 바뀐 값을 보존한다.
		}
		if dc.Host != "" {
			entry["host"] = dc.Host
		}
		if dc.Port != 0 {
			entry["port"] = dc.Port
		}
		// per-device 오버라이드는 명시된 경우에만 기록한다(미지정 → 에이전트 값 상속).
		if dc.Transport != "" {
			entry["transport"] = dc.Transport
		}
		if dc.ShareSession != nil {
			entry["share_session"] = *dc.ShareSession
		}
		if dc.MaxBlockRegisters != nil {
			entry["max_block_registers"] = int(*dc.MaxBlockRegisters)
		}
		// 시리얼 파라미터는 per-device rtu 오버라이드일 때만 파싱되므로 같은 조건으로만 방출한다.
		if dc.Transport == TransportRTU {
			entry["serial_port"] = dc.Serial.Port
			entry["baud_rate"] = dc.Serial.BaudRate
			entry["data_bits"] = dc.Serial.DataBits
			entry["stop_bits"] = dc.Serial.StopBits
			entry["parity"] = dc.Serial.Parity
		}

		groups := make([]any, 0, len(dc.RegisterGroups))
		for _, rg := range dc.RegisterGroups {
			g := map[string]any{
				"function_code": int(rg.FunctionCode),
				"start_address": int(rg.StartAddress),
				"quantity":      int(rg.Quantity),
			}
			if rg.Name != "" {
				g["name"] = rg.Name
			}
			if rg.DataType != "" {
				g["data_type"] = rg.DataType
			}
			if rg.PollInterval > 0 {
				g["poll_interval"] = rg.PollInterval.String()
			}
			// enabled 는 미사용일 때만 기록한다(사용 중이면 키 생략 → 기존 설정과 동일 형상).
			if !rg.IsEnabled() {
				g["enabled"] = false
			}
			if len(rg.TypeMap) > 0 {
				tm := make([]any, 0, len(rg.TypeMap))
				for _, e := range rg.TypeMap {
					entry := map[string]any{
						"address":   int(e.Address),
						"data_type": e.DataType,
					}
					if e.ByteOrder != "" {
						entry["byte_order"] = e.ByteOrder
					}
					tm = append(tm, entry)
				}
				g["type_map"] = tm
			}
			groups = append(groups, g)
		}
		entry["register_groups"] = groups

		out = append(out, entry)
	}
	return out
}
