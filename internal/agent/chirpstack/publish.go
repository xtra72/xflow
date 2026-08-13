package chirpstack

import "fmt"

// PublishMessage 는 ChirpStack MQTT 브로커에 메시지를 발행한다
// (agent.MessagePublisher, REQ-M1-01).
//
// 가드는 internal/agent/system/mqtt_agent.go 의 PublishMessage 를 미러링한다:
//   - 클라이언트 스냅샷을 a.mu.RLock 하에 캡처한 뒤 락을 해제하고 발행한다.
//   - nil/미연결 클라이언트, 빈 토픽은 거부한다(발행 0건).
//   - 발행 실패는 external error 통계로 계상하고 에러를 감싸 반환한다.
//
// 추가 가드(REQ-M1-04): Stop 이후에는 Paho 백그라운드 재연결이 성공하더라도 발행하지
// 않는다 — subscribe() 의 stopped 가드와 동일한 취지(중지된 에이전트의 부활 방지).
func (a *ChirpStackAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	if a.stopped.Load() {
		return fmt.Errorf("chirpstack: 에이전트가 중지됨 — 발행하지 않음")
	}

	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("chirpstack: 브로커에 연결되어 있지 않음")
	}
	if topic == "" {
		return fmt.Errorf("chirpstack: 발행 토픽이 지정되지 않음")
	}

	// 여기까지 온 호출은 노드 → 에이전트 방향의 내부 트래픽이다(제어 노드가
	// 다운링크 명령을 넘긴 것). ReceiveMessage 의 InternalMessagesSent 와 짝을
	// 이루는 반대 축이며, External(에이전트 ↔ 브로커)과는 별개로 계상한다.
	//
	// 가드를 모두 통과한 뒤에 세는 이유: 중지/미연결/빈 토픽으로 거부된 명령은
	// 에이전트가 처리를 수락한 적이 없으므로 "수신"으로 계상하면 안 된다.
	a.stats.IncrInternalMessagesReceived()

	token := client.Publish(topic, qos, retained, payload)
	token.Wait()
	if token.Error() != nil {
		a.stats.IncrExternalMessagesErrored()
		return fmt.Errorf("chirpstack: 발행 실패: %w", token.Error())
	}

	a.stats.IncrExternalMessagesSent()
	a.stats.AddBytesWritten(int64(len(payload)))
	a.logger.Debug("chirpstack: 메시지 발행 완료",
		"topic", topic,
		"qos", qos,
		"retained", retained,
		"bytes", len(payload),
	)
	return nil
}
