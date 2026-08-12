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

// uplinkRxInfo 는 업링크 rxInfo[] 항목 중 comm-state(best-gateway) 산출에 쓰는
// 게이트웨이 수신 품질 필드만 담는다 (M5, REQ-M5-05).
type uplinkRxInfo struct {
	GatewayID string  `json:"gatewayId"`
	RSSI      int     `json:"rssi"`
	SNR       float64 `json:"snr"`
}

// uplink 는 ChirpStack LoRaWAN 업링크 이벤트의 디코드 대상 필드이다.
//
// rxInfo(게이트웨이 rssi/snr) 는 comm-state(best-gateway) 산출에 쓰인다 (M5).
type uplink struct {
	Time       string           `json:"time"`
	DeviceInfo uplinkDeviceInfo `json:"deviceInfo"`
	Object     map[string]any   `json:"object"`
	RxInfo     []uplinkRxInfo   `json:"rxInfo"`
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

// bestGateway 는 rxInfo[] 중 최대 rssi 게이트웨이의 rssi/snr/gatewayId 를 반환한다
// (REQ-M5-05). rxInfo 가 비어 있으면 ok=false.
//
// 최대 rssi 를 "최적" 대표 게이트웨이로 본다(신호 세기 우선). 동률이면 먼저 나온
// 항목을 유지한다(> 비교이므로 갱신하지 않음 → 결정적).
func bestGateway(rx []uplinkRxInfo) (rssi int, snr float64, gatewayID string, ok bool) {
	for i := range rx {
		if !ok || rx[i].RSSI > rssi {
			rssi, snr, gatewayID, ok = rx[i].RSSI, rx[i].SNR, rx[i].GatewayID, true
		}
	}
	return rssi, snr, gatewayID, ok
}
