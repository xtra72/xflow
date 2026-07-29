package airpurifier

import (
	"encoding/json"
	"fmt"
)

// handleRequestState 는 request_state 명령을 처리한다 (REQ-AIRPUR-001-05-04).
//
// 캐시된 로스터 상태를 즉시 JSON 으로 반환하며, MQTT/브로커 통신을 일절 하지 않는다. device_id
// 가 지정되면 단일 디바이스를, 없으면 전체 디바이스를 반환한다. 반환 shape 는 관측 게이팅
// (StateForJSON)을 재사용해 device_state_changed emit 과 일관된다.
func (a *AirPurifierAgent) handleRequestState(req processRequest) ([]byte, error) {
	if req.DeviceID != "" {
		a.mu.RLock()
		dev, ok := a.devices[req.DeviceID]
		if !ok {
			a.mu.RUnlock()
			return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, req.DeviceID)
		}
		d := dev.clone()
		a.mu.RUnlock()
		return json.Marshal(deviceStateJSON(&d))
	}

	// 전체 디바이스: ListDevices 가 RLock 하에 정렬된 값 복사본을 반환한다.
	devs := a.ListDevices()
	out := make([]map[string]any, 0, len(devs))
	for i := range devs {
		out = append(out, deviceStateJSON(&devs[i]))
	}
	return json.Marshal(map[string]any{"status": "ok", "devices": out})
}

// deviceStateJSON 은 디바이스의 캐시 상태를 관측 게이팅된 JSON map 으로 만든다.
// device_id/online + (설정 시)group_id + 관측된 상태 축(StateForJSON)을 담는다.
func deviceStateJSON(d *Device) map[string]any {
	m := map[string]any{
		"device_id": d.DeviceID,
		"online":    d.Online,
	}
	if d.GroupID != "" {
		m["group_id"] = d.GroupID
	}
	for k, v := range d.StateForJSON() {
		m[k] = v
	}
	return m
}
