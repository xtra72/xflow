// stream_proxy.go 는 M8(그룹 J) 서버 측 스트리밍 프록시(브라우저 ↔ 노드 팬아웃)를
// 구현한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.10.2, REQ-J08/J08b).
//
// 설계(팬아웃 / teardown / 백프레셔):
//
//   - 논리 스트림(logicalStream): {instance_id, domain, stream_action, args-hash} 키로
//     식별되는 하나의 노드 구독이다. 서버가 subscription_id 를 부여하고 노드로 subscribe
//     를 1회만 보낸다. 동일 키의 다중 브라우저 소비자는 하나의 노드 구독을 공유한다
//     (중복 노드 구독 없이 팬아웃 — REQ-J08).
//   - 구독(Subscribe): 소비자(브라우저 SSE 핸들러)가 호출한다. 키가 이미 있으면 소비자만
//     추가하고, 없으면 논리 스트림을 만들어 노드로 subscribe 를 전송한다. 게이팅(IsManaged
//   - 노출 범위)은 핸들러가 Subscribe 호출 전에 평가한다(REQ-J05).
//   - stream_data 라우팅(routeStreamData): 노드가 보낸 프레임을 subscription_id 로 논리
//     스트림에 매칭해 모든 소비자에게 팬아웃한다. Error 프레임은 터미널이므로 모든
//     소비자를 종료하고 논리 스트림을 제거한다(REQ-J08 — unsubscribe 불필요: 노드가 이미
//     종료). 정상 프레임은 비차단 송신으로 팬아웃한다.
//   - teardown(REQ-J08b):
//     · 브라우저 연결 종료 → 소비자 제거. 마지막 소비자면 노드로 unsubscribe 전송 +
//     논리 스트림 제거.
//     · 노드 오프라인/세션 종료 → teardownNodeStreams 가 그 노드의 전 논리 스트림을
//     종료하고 모든 소비자를 닫는다(unsubscribe 불필요 — 노드 연결 소멸).
//     · 터미널 error 프레임 → 위 routeStreamData 처리.
//     어떤 경로에서도 소비자 채널 close + 레지스트리 제거가 보장되어 누수가 없다.
//   - 백프레셔(REQ-J08b): 소비자 채널은 bounded buffer 이다. 팬아웃은 비차단 송신이며,
//     버퍼가 가득 차면 최신값 우선으로 drop(drain-to-latest)한다. 따라서 느린 소비자가
//     서버 읽기 루프나 다른 노드/스트림을 막지 않는다(routeStreamData 는 읽기 루프에서
//     호출되므로 절대 블로킹되면 안 된다).
package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// defaultStreamConsumerBuffer 는 소비자 채널 버퍼 크기이다(백프레셔 유계).
const defaultStreamConsumerBuffer = 16

// streamConsumer 는 단일 브라우저 소비자를 나타낸다(REQ-J08).
//
// 종료 신호는 done 채널로만 전달한다(ch 는 close 하지 않는다). 이는 송신(fanOut)과
// 종료(close)가 동시에 일어나도 "닫힌 채널 송신" 경합이 발생하지 않게 하는 핵심
// 설계이다(데이터 레이스 제거). fanOut 은 done 을 함께 select 하여 종료된 소비자에는
// 송신하지 않으며, SSE 루프는 done 을 종료 신호로 사용한다. ch 는 미참조 후 GC 된다.
type streamConsumer struct {
	// ch 는 redacted stream_data 본문을 흘리는 채널이다. close 하지 않는다(레이스 회피).
	ch chan json.RawMessage
	// done 은 소비자/스트림 종료 신호이다(SSE 루프가 select 로 감지; fanOut 이 함께 select).
	done chan struct{}
	// closeOnce 는 done close 를 1회만 수행한다(멱등 teardown).
	closeOnce sync.Once
}

// close 는 소비자를 종료한다(done close — 멱등). SSE 루프가 깨어나 정리한다. ch 는
// 닫지 않는다(fanOut 과의 송신/close 경합 회피 — Done() 으로 송신을 차단).
func (c *streamConsumer) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

// logicalStream 은 하나의 노드 구독을 공유하는 소비자 집합이다(REQ-J08 팬아웃).
type logicalStream struct {
	subscriptionID string
	instanceID     string
	key            string
	consumers      map[*streamConsumer]struct{}
}

// streamManager 는 브라우저↔노드 스트림 팬아웃/teardown 을 관리한다(REQ-J08/J08b).
type streamManager struct {
	srv *Server

	mu      sync.Mutex
	bySubID map[string]*logicalStream // subscription_id → 논리 스트림
	byKey   map[string]*logicalStream // 스트림 키 → 논리 스트림(중복 노드 구독 방지)
}

// newStreamManager 는 streamManager 를 생성한다.
func newStreamManager(srv *Server) *streamManager {
	return &streamManager{
		srv:     srv,
		bySubID: make(map[string]*logicalStream),
		byKey:   make(map[string]*logicalStream),
	}
}

// streamKey 는 논리 스트림 키를 구성한다({instance_id, domain, stream_action, args-hash}).
func streamKey(instanceID, domain, streamAction string, args json.RawMessage) string {
	h := sha256.New()
	_, _ = h.Write([]byte(instanceID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(domain))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(streamAction))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(args))
	return hex.EncodeToString(h.Sum(nil))
}

// StreamHandle 은 브라우저 SSE 핸들러가 사용하는 구독 핸들이다(REQ-J08/J08b).
//
//   - Frames: redacted stream_data 본문 채널(SSE 루프가 읽어 브라우저로 전송).
//   - Done: 스트림 종료 신호(노드 오프라인/터미널 오류/teardown). SSE 루프가 select 로
//     감지해 연결을 닫는다. Frames 는 close 되지 않으므로 종료 판정은 Done 으로 한다.
//   - Unsubscribe: 브라우저 연결 종료 시 호출(마지막 소비자면 노드 unsubscribe — teardown).
type StreamHandle struct {
	Frames      <-chan json.RawMessage
	Done        <-chan struct{}
	Unsubscribe func()
}

// Subscribe 는 브라우저 소비자를 논리 스트림에 등록하고 StreamHandle 을 반환한다
// (REQ-J08). 동일 키 스트림이 없으면 노드로 subscribe 를 1회 전송한다.
//
// 게이팅(REQ-J05)은 호출자(핸들러)가 Subscribe 전에 평가한다. 본 메서드는 라이브
// 연결 확보(ErrNoConn)만 추가로 확인한다.
func (m *streamManager) Subscribe(instanceID, domain, streamAction string, args json.RawMessage) (StreamHandle, error) {
	consumer := &streamConsumer{
		ch:   make(chan json.RawMessage, defaultStreamConsumerBuffer),
		done: make(chan struct{}),
	}
	key := streamKey(instanceID, domain, streamAction, args)

	m.mu.Lock()
	ls, exists := m.byKey[key]
	if exists {
		// 기존 논리 스트림에 소비자만 추가(노드 재구독 없음 — 팬아웃).
		ls.consumers[consumer] = struct{}{}
		m.mu.Unlock()
		return StreamHandle{
			Frames:      consumer.ch,
			Done:        consumer.done,
			Unsubscribe: func() { m.removeConsumer(key, consumer) },
		}, nil
	}

	// 새 논리 스트림 — 노드로 subscribe 전송 필요. 먼저 라이브 연결 확보.
	nc, ok := m.srv.connFor(instanceID)
	if !ok {
		m.mu.Unlock()
		return StreamHandle{}, ErrNoConn
	}
	subscriptionID := uuid.NewString()
	ls = &logicalStream{
		subscriptionID: subscriptionID,
		instanceID:     instanceID,
		key:            key,
		consumers:      map[*streamConsumer]struct{}{consumer: {}},
	}
	m.byKey[key] = ls
	m.bySubID[subscriptionID] = ls
	m.mu.Unlock()

	// 노드로 subscribe 전송(락 밖 — 쓰기 블로킹이 매니저 락을 잡지 않게).
	msg, err := NewSubscribeMessage(SubscribePayload{
		SubscriptionID:   subscriptionID,
		TargetInstanceID: instanceID,
		Domain:           domain,
		StreamAction:     streamAction,
		Args:             args,
	})
	if err == nil {
		err = writeEnvelope(nc.conn, msg)
	}
	if err != nil {
		// 전송 실패 → 논리 스트림 롤백 + 소비자 종료.
		m.mu.Lock()
		delete(m.byKey, key)
		delete(m.bySubID, subscriptionID)
		m.mu.Unlock()
		consumer.close()
		m.srv.logger.Warn("스트림 subscribe 전송 실패",
			"instance_id", instanceID, "domain", domain, "stream_action", streamAction, "error", err)
		return StreamHandle{}, err
	}

	m.srv.logger.Debug("브라우저 스트림 구독 시작",
		"subscription_id", subscriptionID, "instance_id", instanceID,
		"domain", domain, "stream_action", streamAction)
	return StreamHandle{
		Frames:      consumer.ch,
		Done:        consumer.done,
		Unsubscribe: func() { m.removeConsumer(key, consumer) },
	}, nil
}

// removeConsumer 는 소비자를 논리 스트림에서 제거한다. 마지막 소비자면 노드로
// unsubscribe 를 전송하고 논리 스트림을 제거한다(REQ-J08b teardown).
func (m *streamManager) removeConsumer(key string, consumer *streamConsumer) {
	m.mu.Lock()
	ls, ok := m.byKey[key]
	if !ok {
		m.mu.Unlock()
		consumer.close()
		return
	}
	if _, present := ls.consumers[consumer]; present {
		delete(ls.consumers, consumer)
	}
	last := len(ls.consumers) == 0
	var (
		instanceID string
		subID      string
	)
	if last {
		instanceID = ls.instanceID
		subID = ls.subscriptionID
		delete(m.byKey, key)
		delete(m.bySubID, subID)
	}
	m.mu.Unlock()

	consumer.close()

	if last {
		// 마지막 소비자 — 노드 구독 해제(teardown — REQ-J08b).
		if nc, ok := m.srv.connFor(instanceID); ok {
			if msg, err := NewUnsubscribeMessage(UnsubscribePayload{SubscriptionID: subID}); err == nil {
				_ = writeEnvelope(nc.conn, msg)
			}
		}
		m.srv.logger.Debug("브라우저 스트림 구독 해제(마지막 소비자)",
			"subscription_id", subID, "instance_id", instanceID)
	}
}

// dispatch 는 노드 stream_data 프레임을 소비자에게 팬아웃한다(REQ-J08). 비차단 송신 +
// 최신값 coalesce 로 백프레셔를 흡수한다(느린 소비자가 읽기 루프를 막지 않음). Error
// 프레임은 터미널이므로 모든 소비자를 종료하고 논리 스트림을 제거한다.
func (m *streamManager) dispatch(p StreamDataPayload) {
	if p.SubscriptionID == "" {
		return
	}
	m.mu.Lock()
	ls, ok := m.bySubID[p.SubscriptionID]
	if !ok {
		m.mu.Unlock()
		return
	}

	if p.Error != "" {
		// 터미널 오류 — 모든 소비자 종료 + 논리 스트림 제거(unsubscribe 불필요).
		delete(m.byKey, ls.key)
		delete(m.bySubID, ls.subscriptionID)
		consumers := make([]*streamConsumer, 0, len(ls.consumers))
		for c := range ls.consumers {
			consumers = append(consumers, c)
		}
		m.mu.Unlock()
		for _, c := range consumers {
			c.close()
		}
		m.srv.logger.Debug("스트림 터미널 오류 — 소비자 종료",
			"subscription_id", p.SubscriptionID)
		return
	}

	// 정상 프레임 — 소비자 스냅샷을 잡고 락 밖에서 비차단 팬아웃한다.
	consumers := make([]*streamConsumer, 0, len(ls.consumers))
	for c := range ls.consumers {
		consumers = append(consumers, c)
	}
	m.mu.Unlock()

	for _, c := range consumers {
		fanOut(c, p.Payload)
	}
}

// fanOut 은 소비자 채널로 비차단 송신한다(백프레셔 — REQ-J08b). 버퍼가 가득 차면
// 가장 오래된 프레임을 drop 하고 최신값을 넣는다(최신값 우선 — coalesce). 종료된
// 소비자(done close)에는 송신하지 않는다(닫힌 채널 송신 경합 회피). 따라서 느린
// 소비자가 읽기 루프나 다른 소비자를 막지 않는다(비차단 + done 가드).
func fanOut(c *streamConsumer, v json.RawMessage) {
	select {
	case <-c.done:
		return // 종료된 소비자 — 송신하지 않는다(레이스/누수 방지).
	case c.ch <- v:
		return
	default:
	}
	// 버퍼 full — 오래된 프레임 1개를 비우고 최신값을 넣는다(coalesce).
	select {
	case <-c.ch:
	default:
	}
	select {
	case <-c.done:
	case c.ch <- v:
	default:
	}
}

// teardownNodeStreams 는 한 노드의 모든 논리 스트림을 종료한다(노드 오프라인/세션 종료
// — REQ-J08b). 모든 소비자를 닫고 레지스트리에서 제거한다(unsubscribe 불필요 — 연결
// 소멸). 누수 없음.
func (m *streamManager) teardownNode(instanceID string) {
	m.mu.Lock()
	var victims []*logicalStream
	for _, ls := range m.bySubID {
		if ls.instanceID == instanceID {
			victims = append(victims, ls)
		}
	}
	for _, ls := range victims {
		delete(m.byKey, ls.key)
		delete(m.bySubID, ls.subscriptionID)
	}
	// 종료할 소비자 스냅샷.
	var consumers []*streamConsumer
	for _, ls := range victims {
		for c := range ls.consumers {
			consumers = append(consumers, c)
		}
	}
	m.mu.Unlock()

	for _, c := range consumers {
		c.close()
	}
	if len(victims) > 0 {
		m.srv.logger.Debug("노드 스트림 전체 teardown",
			"instance_id", instanceID, "streams", len(victims))
	}
}

// --- Server 위임 메서드(읽기 루프/세션 종료에서 호출) -------------------------

// routeStreamData 는 노드 stream_data 프레임을 streamManager 로 위임한다(REQ-J08).
// 읽기 루프에서 호출되므로 비차단이어야 한다(streamManager.dispatch 가 보장).
func (s *Server) routeStreamData(payload []byte) {
	var p StreamDataPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("stream_data 디코드 실패", "error", err)
		return
	}
	if s.streamManager != nil {
		s.streamManager.dispatch(p)
	}
}

// teardownNodeStreams 는 노드 세션 종료 시 그 노드의 브라우저 스트림을 정리한다
// (REQ-J08b).
func (s *Server) teardownNodeStreams(instanceID string) {
	if s.streamManager != nil {
		s.streamManager.teardownNode(instanceID)
	}
}

// SubscribeStream 은 핸들러(브라우저 SSE)가 논리 스트림에 소비자를 등록하는 공개
// 진입점이다(REQ-J08). 반환된 StreamHandle.Unsubscribe 는 연결 종료 시 호출해야
// 한다(teardown — REQ-J08b).
func (s *Server) SubscribeStream(instanceID, domain, streamAction string, args json.RawMessage) (StreamHandle, error) {
	if s.streamManager == nil {
		return StreamHandle{}, ErrNoConn
	}
	return s.streamManager.Subscribe(instanceID, domain, streamAction, args)
}
