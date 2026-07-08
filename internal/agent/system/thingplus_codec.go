package system

import (
	"encoding/json"
	"fmt"
	"time"
)

// thingplus_codec.go 는 ThingsBoard Gateway MQTT API 의 업링크/다운링크 페이로드를
// 조립(build)하고 해석(parse)하는 순수 함수 모음이다.
//
// 코덱은 무상태(stateless)이며 브로커나 에이전트 상태에 의존하지 않는다.
// 상태 머신/매핑/버퍼링 등 상태 로직은 thingplus_agent.go 에 둔다 (관심사 분리).

// === 업링크 빌더 (플로우 → 게이트웨이) ===

// telemetryEntry 는 텔레메트리 배열의 단일 항목이다.
//
// TS 는 epoch milliseconds(int64)이며, TSSet 이 false 이면 "ts" 키를 생략하여
// 서버 시각을 사용하도록 한다 (REQ-up-ts / A9).
type telemetryEntry struct {
	// TS 는 epoch milliseconds(time.Time.UnixMilli() 결과)이다.
	TS int64
	// TSSet 은 TS 값이 유효한지(직렬화 시 "ts" 키를 포함할지) 나타낸다.
	TSSet bool
	// Values 는 텔레메트리 키-값 쌍이다.
	Values map[string]any
}

// buildDeviceTelemetry 는 ThingsBoard Device API 텔레메트리 페이로드를 조립한다.
//
// 출력 형식(Device API): {"ts":<UnixMilli>,"values":{"<name>":{<telemetry kv>}}}
//
//   - name 은 유닛/디바이스 NAME 으로, values 맵의 키가 된다. 인입 메시지 하나당
//     values 에 하나의 유닛 항목만 담긴다(형식 자체는 다중 유닛을 지원한다).
//   - ts 가 nil 이면 "ts" 키를 생략하여 서버 시각을 사용하도록 한다(Device API 기본).
//   - ts 가 non-nil 이면 time.Time.UnixMilli() (epoch milliseconds int64) 로 직렬화한다.
//
// Gateway API 의 buildTelemetry({"<NAME>":[{ts,values}]}) 와 달리 최상위에 ts 를 두고
// values 를 NAME→kv 맵으로 구성한다.
func buildDeviceTelemetry(name string, ts *time.Time, values map[string]any) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("thingplus codec: device telemetry name 이 비어 있음")
	}
	// values 가 nil 이면 빈 객체로 직렬화하여 서버 파싱 오류를 방지한다.
	if values == nil {
		values = map[string]any{}
	}

	payload := map[string]any{
		"values": map[string]any{name: values},
	}
	if ts != nil {
		payload["ts"] = ts.UnixMilli()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("thingplus codec: device telemetry 직렬화 실패: %w", err)
	}
	return data, nil
}

// buildDeviceClientAttributes 는 ThingsBoard Device API 클라이언트 속성 업링크
// 페이로드를 조립한다.
//
// 출력 형식(Device API): {"<k>":<v>} — flat 객체. Gateway API 의
// buildClientAttributes({"<NAME>":{...}}) 와 달리 NAME 으로 감싸지 않는다.
func buildDeviceClientAttributes(attrs map[string]any) ([]byte, error) {
	if attrs == nil {
		attrs = map[string]any{}
	}
	data, err := json.Marshal(attrs)
	if err != nil {
		return nil, fmt.Errorf("thingplus codec: device client attributes 직렬화 실패: %w", err)
	}
	return data, nil
}

// parseDeviceSharedAttributes 는 ThingsBoard Device API 공유 속성 다운링크 페이로드를
// tolerant 하게 파싱하여 속성 맵을 반환한다.
//
// Device API 공유 속성 업데이트는 "me" 를 대상으로 하므로 최상위 "device" 필드가 없다.
// 브로커/버전에 따라 두 가지 형태가 관찰되므로 모두 수용한다:
//   - flat:            {"<k>":<v>, ...}
//   - shared 래핑:     {"shared":{"<k>":<v>, ...}}
//
// 둘 다 아니면(그러나 유효한 JSON 객체이면) flat 으로 간주하여 그대로 반환한다.
func parseDeviceSharedAttributes(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("thingplus codec: device 공유 속성 파싱 실패: %w", err)
	}
	// "shared" 키가 객체이면 그 내부를 속성으로 사용한다.
	if shared, ok := raw["shared"].(map[string]any); ok {
		return shared, nil
	}
	// 그 외에는 flat 객체 전체를 속성으로 간주한다.
	return raw, nil
}

// buildTelemetry 는 단일 텔레메트리 항목을 게이트웨이 텔레메트리 페이로드로 조립한다
// (REQ-up-telemetry / REQ-up-ts).
//
// NOTE(Device API 전환): 이 게이트웨이 빌더는 dormant 이다. 업링크 경로는
// buildDeviceTelemetry 를 사용한다. connect/RPC 등 dormant 게이트웨이 경로 및
// 관련 테스트와의 일관성을 위해 유지한다.
//
// 출력 형식: {"<NAME>":[{"ts":<UnixMilli>,"values":{...}}]}
//
// ts 가 nil 이면 "ts" 키를 생략하여 서버 시각을 사용하도록 한다.
func buildTelemetry(name string, ts *time.Time, values map[string]any) ([]byte, error) {
	entry := telemetryEntry{Values: values}
	if ts != nil {
		entry.TS = ts.UnixMilli()
		entry.TSSet = true
	}
	return buildTelemetryBatch(name, []telemetryEntry{entry})
}

// buildTelemetryBatch 는 동일 디바이스의 다수 텔레메트리 항목을 하나의 페이로드
// 배열로 배치 조립한다 (REQ-up-batch).
//
// 출력 형식: {"<NAME>":[{"ts":<ms>,"values":{...}},{"values":{...}}, ...]}
//
// 각 항목은 TSSet 이 false 이면 "ts" 키를 생략한다.
func buildTelemetryBatch(name string, entries []telemetryEntry) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("thingplus codec: telemetry name 이 비어 있음")
	}

	arr := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		item := map[string]any{}
		if e.TSSet {
			item["ts"] = e.TS
		}
		// values 가 nil 이면 빈 객체로 직렬화하여 서버 파싱 오류를 방지한다.
		if e.Values != nil {
			item["values"] = e.Values
		} else {
			item["values"] = map[string]any{}
		}
		arr = append(arr, item)
	}

	payload := map[string]any{name: arr}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("thingplus codec: telemetry 직렬화 실패: %w", err)
	}
	return data, nil
}

// buildClientAttributes 는 디바이스 클라이언트 속성 업링크 페이로드를 조립한다
// (REQ-up-attributes).
//
// 출력 형식: {"<NAME>":{"<k>":<v>}}
func buildClientAttributes(name string, attrs map[string]any) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("thingplus codec: attributes name 이 비어 있음")
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	payload := map[string]any{name: attrs}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("thingplus codec: client attributes 직렬화 실패: %w", err)
	}
	return data, nil
}

// === 다운링크 파서 (게이트웨이 → 플로우) ===

// rpcRequestData 는 RPC 요청 본문(data)이다.
type rpcRequestData struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

// rpcRequest 는 v1/gateway/rpc 다운링크 RPC 요청 메시지이다.
//
// 입력 형식: {"device":"<NAME>","data":{"id":<n>,"method":"<m>","params":{...}}}
type rpcRequest struct {
	Device string         `json:"device"`
	Data   rpcRequestData `json:"data"`
}

// parseRPCRequest 는 v1/gateway/rpc 다운링크 페이로드를 파싱한다 (REQ-dn-rpc-recv).
func parseRPCRequest(data []byte) (rpcRequest, error) {
	var req rpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return rpcRequest{}, fmt.Errorf("thingplus codec: RPC 요청 파싱 실패: %w", err)
	}
	if req.Device == "" {
		return rpcRequest{}, fmt.Errorf("thingplus codec: RPC 요청에 device NAME 이 없음")
	}
	return req, nil
}

// sharedAttrMsg 는 v1/gateway/attributes 다운링크 공유 속성 변경 메시지이다.
//
// 입력 형식: {"device":"<NAME>","data":{"<k>":<v>}}
type sharedAttrMsg struct {
	Device string         `json:"device"`
	Data   map[string]any `json:"data"`
}

// parseSharedAttributes 는 v1/gateway/attributes 다운링크 페이로드를 파싱한다
// (REQ-dn-shared).
func parseSharedAttributes(data []byte) (sharedAttrMsg, error) {
	var msg sharedAttrMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return sharedAttrMsg{}, fmt.Errorf("thingplus codec: 공유 속성 파싱 실패: %w", err)
	}
	if msg.Device == "" {
		return sharedAttrMsg{}, fmt.Errorf("thingplus codec: 공유 속성에 device NAME 이 없음")
	}
	if msg.Data == nil {
		msg.Data = map[string]any{}
	}
	return msg, nil
}

// === RPC 응답 빌더 (플로우 → 게이트웨이) ===

// buildRPCResponse 는 RPC 응답 발행 페이로드를 조립한다 (REQ-dn-rpc-reply).
//
// 출력 형식: {"device":"<NAME>","id":<n>,"data":{...}}
func buildRPCResponse(name string, id int, data map[string]any) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("thingplus codec: RPC 응답 name 이 비어 있음")
	}
	if data == nil {
		data = map[string]any{}
	}
	payload := map[string]any{
		"device": name,
		"id":     id,
		"data":   data,
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("thingplus codec: RPC 응답 직렬화 실패: %w", err)
	}
	return out, nil
}

// === (OPTIONAL) 속성 요청/응답 상관 (REQ-dn-attr-req) ===
//
// NOTE(A8): v1/gateway/attributes/response 의 정확한 다중 키(client/shared) 인코딩은
// 라이브 브로커 검증이 필요하다. 아래 요청 빌더와 응답 파서는 예상 구조를 기반으로 하며,
// 응답 파싱은 tolerant/best-effort 로 구현했다. 라이브 브로커 스모크 테스트(A8) 이후
// 확정한다. 이 경로에 의존하는 코어 흐름은 없다.

// topicGatewayAttributesRequest 는 속성 요청 발행 토픽이다.
const topicGatewayAttributesRequest = "v1/gateway/attributes/request"

// topicGatewayAttributesResponse 는 속성 응답 수신 토픽이다.
const topicGatewayAttributesResponse = "v1/gateway/attributes/response"

// buildAttributesRequest 는 디바이스 속성 요청 페이로드를 조립한다 (Optional REQ-dn-attr-req).
//
// 반환값: (발행 토픽, 페이로드, error). 발행 토픽은 topicGatewayAttributesRequest 로,
// 요청 빌더가 발행 대상 토픽을 함께 반환하여 호출자가 토픽을 임의로 조립하지 않도록 한다.
//
// 출력 형식: {"id":<n>,"device":"<NAME>","client":[...],"shared":[...]}
//
// id 는 응답 상관(correlation)에 사용된다. client/shared 는 조회할 속성 키 목록이다.
func buildAttributesRequest(id int, name string, client, shared []string) (string, []byte, error) {
	if name == "" {
		return "", nil, fmt.Errorf("thingplus codec: attributes 요청 name 이 비어 있음")
	}
	payload := map[string]any{
		"id":     id,
		"device": name,
	}
	if client != nil {
		payload["client"] = client
	}
	if shared != nil {
		payload["shared"] = shared
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("thingplus codec: attributes 요청 직렬화 실패: %w", err)
	}
	return topicGatewayAttributesRequest, out, nil
}

// attributesResponse 는 v1/gateway/attributes/response 응답이다 (Optional / A8 tolerant).
//
// NOTE(A8): 예상 구조 {"id":<n>,"device":"<NAME>","value":...} 를 기반으로 하되,
// value 필드 타입이 브로커/버전에 따라 다를 수 있으므로 json.RawMessage 로 보관하여
// 라이브 검증 전까지 데이터 손실 없이 best-effort 처리한다.
type attributesResponse struct {
	ID     int             `json:"id"`
	Device string          `json:"device"`
	Value  json.RawMessage `json:"value,omitempty"`
}

// parseAttributesResponse 는 속성 응답을 tolerant 하게 파싱한다 (Optional / A8).
//
// 반환값: (수신 토픽, 응답, error). 수신 토픽은 topicGatewayAttributesResponse 로,
// 이 응답이 상관되는 구독 토픽을 함께 반환하여 요청/응답 토픽 쌍을 코덱이 소유하게 한다.
//
// NOTE(A8): 정확한 다중 키 인코딩은 라이브 브로커 검증 후 확정한다. 현재는 id 상관에
// 필요한 최소 필드(id, device)만 신뢰하고 value 는 원시 바이트로 보관한다.
func parseAttributesResponse(data []byte) (string, attributesResponse, error) {
	var resp attributesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", attributesResponse{}, fmt.Errorf("thingplus codec: attributes 응답 파싱 실패: %w", err)
	}
	return topicGatewayAttributesResponse, resp, nil
}
