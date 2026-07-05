// client_stream.go 는 M8(그룹 J) 노드 측 스트리밍 프록시(subscribe/stream_data/
// unsubscribe)를 구현한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.10.2, REQ-J08/J08b/J03/J05/J06).
//
// 설계(READ-ONLY, teardown, 백프레셔):
//
//   - subscribe: 승인 게이팅(REQ-J05) + 스트림 allowlist(REQ-J04/J08) 검증 후
//     StreamSource.Subscribe 로 노드의 실시간 소스를 구독한다. 펌프 고루틴을 spawn 해
//     소스 갱신을 redaction(REQ-J06)하여 stream_data 로 push 한다. 구독은 세션 streams
//     레지스트리(c.mu 보호)에 등록되어 unsubscribe/세션 종료 시 추적·정리된다.
//   - unsubscribe: 레지스트리에서 구독을 찾아 cancel → 펌프 종료 → source.Close()
//     (teardown — REQ-J08b). 누수 없음.
//   - 세션 종료: runSession defer 가 전 구독을 cancel 한다(노드 오프라인/연결 종료 시
//     teardown — REQ-J08b). 펌프는 세션 wg 로 추적되어 wg.Wait()가 완료를 보장한다.
//   - 백프레셔(REQ-J08b): 펌프는 소비자(WriteMessage)가 느려도 읽기 루프를 막지 않는
//     별도 고루틴이다. 전송 직전 소스 채널을 최신값으로 coalesce(drain-to-latest)하여
//     느린 소비자에서 프레임 누적을 방지한다(최신값 우선 — 메모리 폭증 방지).
//
// 거부(게이팅/비스트림/소스 오류)는 stream_data{subscription_id, error} 터미널
// 프레임으로 신호한다(서버가 해당 구독을 종료).
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
)

// handleSubscribe 는 subscribe 를 처리하여 노드의 실시간 소스를 구독하고 펌프
// 고루틴을 시작한다(REQ-J08). 게이팅/검증 실패는 오류 프레임으로 거부한다.
func (c *Client) handleSubscribe(ctx context.Context, conn Conn, wg *sync.WaitGroup, payload []byte) {
	var sub SubscribePayload
	if err := json.Unmarshal(payload, &sub); err != nil {
		c.logger.Debug("subscribe 디코드 실패", "error", err)
		return
	}
	if sub.SubscriptionID == "" {
		c.logger.Warn("subscribe 에 subscription_id 누락 — 무시")
		return
	}

	// 1) 승인 게이팅(REQ-J05).
	if !c.hasToken() {
		c.logger.Warn("미승인 노드 — 스트림 구독 거부",
			"subscription_id", sub.SubscriptionID, "domain", sub.Domain, "stream_action", sub.StreamAction)
		c.sendStreamError(conn, sub.SubscriptionID, "node not approved")
		return
	}

	// 2) 스트림 allowlist(REQ-J04/J08). 비스트림 action 거부(READ-ONLY — REQ-J03).
	if !IsStreamableAction(sub.Domain, sub.StreamAction) {
		c.logger.Warn("미허용 stream-action — 거부",
			"subscription_id", sub.SubscriptionID, "domain", sub.Domain, "stream_action", sub.StreamAction)
		c.sendStreamError(conn, sub.SubscriptionID,
			fmt.Sprintf("stream action not allowed: %s/%s", sub.Domain, sub.StreamAction))
		return
	}

	// 3) StreamSource 미구성이면 거부.
	if c.cfg.StreamSource == nil {
		c.logger.Warn("stream source 미구성 — 스트림 구독 거부", "subscription_id", sub.SubscriptionID)
		c.sendStreamError(conn, sub.SubscriptionID, "stream source not configured")
		return
	}

	// 4) 소스 구독(A12 — 노드의 실시간 소스). 구독 ctx 는 세션 ctx 의 자식이므로 세션
	//    종료 시 자동 취소된다(teardown — REQ-J08b).
	subCtx, cancel := context.WithCancel(ctx)
	subscription, err := c.cfg.StreamSource.Subscribe(subCtx, sub.Domain, sub.StreamAction, sub.Args)
	if err != nil {
		cancel()
		c.logger.Warn("스트림 소스 구독 실패",
			"subscription_id", sub.SubscriptionID, "domain", sub.Domain,
			"stream_action", sub.StreamAction, "error", err)
		c.sendStreamError(conn, sub.SubscriptionID, err.Error())
		return
	}

	// 5) 레지스트리 등록(unsubscribe/세션 종료 추적). 세션이 이미 끝났으면(streams nil)
	//    즉시 정리하고 종료한다(레이스 안전).
	handle := &streamSub{cancel: cancel, source: subscription}
	c.mu.Lock()
	if c.streams == nil {
		c.mu.Unlock()
		cancel()
		_ = subscription.Close()
		return
	}
	// 동일 id 재구독 방어: 기존 구독이 있으면 교체 전 정리한다.
	if prev, ok := c.streams[sub.SubscriptionID]; ok {
		prev.cancel()
	}
	c.streams[sub.SubscriptionID] = handle
	c.mu.Unlock()

	// 6) 펌프 고루틴(세션 wg 추적 — 연결 종료/취소 시 정리, 누수 없음).
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runStreamPump(subCtx, conn, sub.SubscriptionID, subscription)
	}()
}

// runStreamPump 은 소스 갱신을 redaction 하여 stream_data 로 push 한다(REQ-J08/J06).
//
// 백프레셔(REQ-J08b): 전송 직전 소스 채널을 최신값으로 coalesce 하여, 느린 소비자에서
// 프레임이 누적되지 않게 한다(최신값 우선 drop). 펌프 종료 시(ctx 취소/소스 close/
// 레지스트리 정리) source.Close()로 소스 구독을 해제하고 레지스트리에서 제거한다.
func (c *Client) runStreamPump(ctx context.Context, conn Conn, subID string, sub StreamSubscription) {
	// teardown 가드: 펌프 종료 경로 어디서든 소스를 닫고 레지스트리에서 제거한다
	// (unsubscribe cancel / 세션 종료 / 소스 close 모두 포함 — 누수 없음).
	defer func() {
		_ = sub.Close()
		c.removeStream(subID)
	}()
	// panic 복구: 소스/직렬화 panic 이 데몬을 죽이지 않도록 한다(시크릿 비노출 — REQ-J06).
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("스트림 펌프 panic 복구",
				"subscription_id", subID, "panic", r, "stack", string(debug.Stack()))
		}
	}()

	updates := sub.Updates()
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-updates:
			if !ok {
				// 소스가 채널을 닫음 → 정상 종료(teardown).
				return
			}
			// 백프레셔: 대기 중인 추가 갱신을 최신값으로 coalesce(drain-to-latest).
			v = drainLatest(updates, v)
			if err := c.sendStreamData(conn, subID, v); err != nil {
				// 연결 종료 등 전송 실패 → 펌프 종료(teardown).
				c.logger.Debug("stream_data 전송 실패 — 펌프 종료",
					"subscription_id", subID, "error", err)
				return
			}
		}
	}
}

// drainLatest 는 채널에 대기 중인 모든 값을 비차단으로 읽어 가장 최신값을 반환한다
// (백프레셔 coalesce — REQ-J08b). 대기 값이 없으면 first 를 그대로 반환한다.
func drainLatest(ch <-chan json.RawMessage, first json.RawMessage) json.RawMessage {
	latest := first
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return latest
			}
			latest = v
		default:
			return latest
		}
	}
}

// handleUnsubscribe 는 unsubscribe 를 처리하여 대상 구독을 teardown 한다(REQ-J08b).
func (c *Client) handleUnsubscribe(payload []byte) {
	var u UnsubscribePayload
	if err := json.Unmarshal(payload, &u); err != nil {
		c.logger.Debug("unsubscribe 디코드 실패", "error", err)
		return
	}
	if u.SubscriptionID == "" {
		return
	}
	c.mu.Lock()
	handle, ok := c.streams[u.SubscriptionID]
	c.mu.Unlock()
	if !ok {
		return
	}
	// cancel → 펌프 종료 → 펌프 defer 가 source.Close() + removeStream 수행(teardown).
	handle.cancel()
}

// removeStream 은 레지스트리에서 구독을 제거한다(펌프 종료 시 호출). 세션이 이미
// 끝났으면(streams nil) no-op.
func (c *Client) removeStream(subID string) {
	c.mu.Lock()
	if c.streams != nil {
		delete(c.streams, subID)
	}
	c.mu.Unlock()
}

// sendStreamData 는 갱신 본문을 redaction 하여 stream_data 로 전송한다(REQ-J06/J08).
func (c *Client) sendStreamData(conn Conn, subID string, payload json.RawMessage) error {
	msg, err := NewStreamDataMessage(StreamDataPayload{
		SubscriptionID: subID,
		Payload:        c.redact(payload),
	})
	if err != nil {
		return err
	}
	return writeEnvelope(conn, msg)
}

// sendStreamError 는 구독 거부/종료를 stream_data 터미널 오류 프레임으로 전송한다
// (게이팅 위반·비스트림 action·소스 오류 — REQ-J05/J08).
func (c *Client) sendStreamError(conn Conn, subID, errMsg string) {
	msg, err := NewStreamDataMessage(StreamDataPayload{
		SubscriptionID: subID,
		Error:          errMsg,
	})
	if err != nil {
		c.logger.Error("stream_data(error) 인코딩 실패", "error", err)
		return
	}
	if werr := writeEnvelope(conn, msg); werr != nil {
		c.logger.Debug("stream_data(error) 전송 실패", "subscription_id", subID, "error", werr)
	}
}
