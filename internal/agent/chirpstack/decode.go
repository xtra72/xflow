package chirpstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// uplinkDeviceInfo 는 ChirpStack 업링크의 deviceInfo 하위 필드 중 본 에이전트가
// 사용하는 항목만 담는다.
//
// ApplicationID 는 다운링크 토픽(application/{applicationId}/device/{devEui}/command/down)
// 구성에 필요하다 (SPEC-CHIRPSTACK-002 REQ-M2-05).
type uplinkDeviceInfo struct {
	DevEui            string            `json:"devEui"`
	DeviceName        string            `json:"deviceName"`
	DeviceProfileName string            `json:"deviceProfileName"`
	ApplicationID     string            `json:"applicationId"`
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
//
// devEui 는 여기서 단 한 번 소문자로 정규화한다(normalizeDevEui). 디코드 직후를
// 정규화 지점으로 택한 이유는 devEui 가 이 이후 (a) 로스터 맵 키, (b) comm 맵 키,
// (c) emit 레코드의 unit_id, (d) 디바이스 정보 dev_eui 로 갈라져 쓰이기 때문이다.
// 이 값들은 모두 device_id 조회 키(ResolveDeviceID 의 unitID)와 같은 문자열이어야
// 하므로, 갈라진 뒤에 각각 정규화하면 한 곳만 누락돼도 동일 물리 디바이스가 두 개의
// device_id 를 발급받아 쪼개진다 — 유일한 상류 지점에서 정규화해 그 실패 양상을
// 원천 차단한다.
func decodeUplink(raw []byte) (*uplink, error) {
	var up uplink
	if err := json.Unmarshal(raw, &up); err != nil {
		return nil, fmt.Errorf("chirpstack decode: %w", err)
	}
	if up.DeviceInfo.DevEui == "" {
		return nil, errors.New("chirpstack decode: missing deviceInfo.devEui")
	}
	up.DeviceInfo.DevEui = normalizeDevEui(up.DeviceInfo.DevEui)
	return &up, nil
}

// normalizeDevEui 는 devEui 를 소문자 hex 로 정규화한다.
//
// 소문자를 정본으로 택한 근거: ChirpStack 은 EUI64 를 소문자 hex 로 직렬화하며
// 실제 캡처 픽스처(testdata/packet.json)의 devEui 도 "24e124141d180806" 로 소문자다.
// 즉 소문자는 현행 동작을 그대로 보존하는 선택이고(정상 경로에서 무변화), 대문자
// 변형이 섞여 들어오는 경우에만 동일 디바이스로 접힌다.
func normalizeDevEui(devEui string) string {
	return strings.ToLower(devEui)
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
