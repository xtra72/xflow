package chirpstack

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// downlinkTopicFormat 은 ChirpStack MQTT 다운링크 명령 토픽 템플릿이다 (A1, REQ-M2-03).
// ChirpStack 은 이 토픽으로 발행된 명령을 디바이스 다운링크 큐에 넣는다.
const downlinkTopicFormat = "application/%s/device/%s/command/down"

// downlinkPayload 는 ChirpStack 다운링크 명령 페이로드 JSON 이다 (A1).
//
// Data 는 base64 문자열이며 표준 base64(StdEncoding, 패딩 포함)를 사용한다.
//
// 근거(ChirpStack v4 소스 대조 기준, 실브로커 미검증): ChirpStack v4 는 command/down 페이로드를
// integration.DownlinkCommand proto 로 파싱하고, 그 JSON 디코더는 pbjson 이 생성한다.
// pbjson 은 표준 알파벳(STANDARD_INDIFFERENT_PAD)을 1순위로 시도하고 URL-safe 는
// '-'/'_' 를 만났을 때만 fallback 으로 재시도하므로, StdEncoding 이 fallback 에
// 의존하지 않는 경로다. 패딩은 Indifferent(선택)이다.
// 필드명은 lowerCamelCase(devEui/confirmed/fPort/data)가 ChirpStack 공식 예제 형태이며,
// devEui 는 MQTT 핸들러가 토픽의 DevEUI 와 대조하므로 필수다.
// 주의: ChirpStack 파서는 ignore_unknown_fields 이므로 필드명 오타는 에러 없이
// 무시되고 기본값이 쓰인다 — 필드명은 골든 벡터 테스트로 고정한다.
//
// 출처:
//   - chirpstack/chirpstack: api/proto/integration/integration.proto (DownlinkCommand)
//   - chirpstack/chirpstack: chirpstack/src/integration/mqtt.rs (serde_json 파싱 + dev_eui 토픽 대조)
//   - influxdata/pbjson: pbjson/src/lib.rs (STANDARD_INDIFFERENT_PAD / URL_SAFE fallback)
//   - https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html
type downlinkPayload struct {
	DevEui    string `json:"devEui"`
	Confirmed bool   `json:"confirmed"`
	FPort     uint8  `json:"fPort"`
	Data      string `json:"data"`
}

// BuildDownlinkTopic 은 applicationId + devEui 로 다운링크 명령 토픽을 구성한다
// (REQ-M2-03).
//
// applicationId 는 업링크 deviceInfo.applicationId 캐시에서 온다 — 최초 업링크 이전에는
// 토픽을 구성할 수 없다(REQ-M2-05, 순서 제약 R5).
func BuildDownlinkTopic(applicationID, devEui string) string {
	return fmt.Sprintf(downlinkTopicFormat, applicationID, devEui)
}

// BuildDownlinkPayload 는 다운링크 명령 페이로드 JSON 을 구성한다 (REQ-M2-03).
//
// 계약: {"devEui":<string>, "confirmed":<bool>, "fPort":<uint8>, "data":<base64>}.
// confirmed 는 페이로드로 통과되며 큐/ack 처리는 v1 범위 밖이다(A4, fire-and-publish).
func BuildDownlinkPayload(devEui string, confirmed bool, fPort uint8, data []byte) ([]byte, error) {
	if devEui == "" {
		return nil, fmt.Errorf("chirpstack-downlink: devEui 가 비어 있습니다")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("chirpstack-downlink: 다운링크 데이터가 비어 있습니다")
	}
	return json.Marshal(downlinkPayload{
		DevEui:    devEui,
		Confirmed: confirmed,
		FPort:     fPort,
		Data:      base64.StdEncoding.EncodeToString(data),
	})
}
