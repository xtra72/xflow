package samsung

// SPEC-HVACR-SYNC-001 Module 6: 토픽 스킴 (Topic Scheme)
//
// 게이트웨이 식별 + 업링크/다운링크 분리를 토픽으로 표현한다(REQ-SYNC-001-06-01).
// gateway_id 는 payload 가 아니라 토픽이 전담한다(방향 격리로 루프백 원천 차단).
//
//	업링크(디코드 메시지)  {prefix}/{gateway_id}/up/nasa       게이트웨이 → 서버
//	다운링크(제어)         {prefix}/{gateway_id}/down/control  서버 → 게이트웨이
//	제어 ack(선택)         {prefix}/{gateway_id}/up/ack        게이트웨이 → 서버
//	상태 스냅샷(7a, 선택)  {prefix}/{gateway_id}/up/snapshot/{addr}  게이트웨이 → 서버

import "fmt"

// defaultMirrorTopicPrefix 는 토픽 prefix 기본값이다(설정으로 변경 가능).
const defaultMirrorTopicPrefix = "xflow/hvacr"

// mirrorTopics 는 특정 게이트웨이 스코프의 토픽 빌더이다.
type mirrorTopics struct {
	prefix    string
	gatewayID string
}

// newMirrorTopics 는 prefix/gatewayID 로 토픽 빌더를 생성한다.
// prefix 가 비어 있으면 defaultMirrorTopicPrefix 를 사용한다.
func newMirrorTopics(prefix, gatewayID string) mirrorTopics {
	if prefix == "" {
		prefix = defaultMirrorTopicPrefix
	}
	return mirrorTopics{prefix: prefix, gatewayID: gatewayID}
}

// uplinkNasa 는 디코드 NASA 메시지 업링크 토픽을 반환한다.
func (t mirrorTopics) uplinkNasa() string {
	return fmt.Sprintf("%s/%s/up/nasa", t.prefix, t.gatewayID)
}

// downlinkControl 은 제어 다운링크 토픽을 반환한다.
func (t mirrorTopics) downlinkControl() string {
	return fmt.Sprintf("%s/%s/down/control", t.prefix, t.gatewayID)
}

// uplinkAck 는 제어 ack 업링크 토픽을 반환한다.
func (t mirrorTopics) uplinkAck() string {
	return fmt.Sprintf("%s/%s/up/ack", t.prefix, t.gatewayID)
}

// snapshotPrefix 는 스냅샷 토픽의 와일드카드 구독 패턴을 반환한다(서버 구독용).
func (t mirrorTopics) snapshotPrefix() string {
	return fmt.Sprintf("%s/%s/up/snapshot/+", t.prefix, t.gatewayID)
}

// snapshot 은 특정 디바이스 주소의 retained 스냅샷 토픽을 반환한다(게이트웨이 발행용).
// addr 은 compact hex(NasaAddress.Hex())를 기대한다.
func (t mirrorTopics) snapshot(addr string) string {
	return fmt.Sprintf("%s/%s/up/snapshot/%s", t.prefix, t.gatewayID, addr)
}
