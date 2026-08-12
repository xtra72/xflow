package chirpstack

import (
	"encoding/json"
	"errors"
	"fmt"
)

// uplinkDeviceInfo 는 ChirpStack 업링크의 deviceInfo 하위 필드 중 본 에이전트가
// 사용하는 항목만 담는다.
type uplinkDeviceInfo struct {
	DevEui            string            `json:"devEui"`
	DeviceName        string            `json:"deviceName"`
	DeviceProfileName string            `json:"deviceProfileName"`
	Tags              map[string]string `json:"tags"`
}

// uplink 는 ChirpStack LoRaWAN 업링크 이벤트의 디코드 대상 필드이다.
//
// rxInfo(게이트웨이 rssi/snr) 는 comm-state(best-gateway) 산출에만 쓰이므로
// M5(comm-state) 범위이며 본 마일스톤에서는 디코드하지 않는다.
type uplink struct {
	Time       string           `json:"time"`
	DeviceInfo uplinkDeviceInfo `json:"deviceInfo"`
	Object     map[string]any   `json:"object"`
}

// decodeUplink 는 원시 ChirpStack 업링크 JSON 을 디코드한다 (REQ-M3-01).
//
// 방어적 파싱: JSON 오류 또는 devEui 누락은 에러로 반환한다. devEui 는 device_id
// 키잉(REQ-M4-01)에 필수이므로 없으면 처리를 진행할 수 없다.
func decodeUplink(raw []byte) (*uplink, error) {
	var up uplink
	if err := json.Unmarshal(raw, &up); err != nil {
		return nil, fmt.Errorf("chirpstack decode: %w", err)
	}
	if up.DeviceInfo.DevEui == "" {
		return nil, errors.New("chirpstack decode: missing deviceInfo.devEui")
	}
	return &up, nil
}
