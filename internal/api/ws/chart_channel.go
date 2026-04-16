// Package ws 의 chart_channel.go 는 SPEC-CHART-001 M2 요구사항을 구현한다:
// - GET /ws/chart/{channel} WebSocket 엔드포인트
// - channel_name 정규식 검증 (REQ-M2-07, HTTP 400 + JSON, 업그레이드 전)
// - 채널 미존재 시 chart.error 전송 후 연결 종료 (REQ-M2-05)
// - backfill + append fan-out (REQ-M2-02/03/04)
// - 클라이언트 종료 시 구독자 자동 제거 (REQ-M2-06)
package ws

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/xtra/xflow/internal/agent/system"
)

// 차트 WS 상수 (개별 연결당 적용).
const (
	// chartWSWriteWait 는 한 프레임 쓰기 타임아웃이다.
	chartWSWriteWait = 10 * time.Second

	// chartWSPongWait 는 클라이언트 pong 대기 시간이다 (read deadline).
	chartWSPongWait = 60 * time.Second

	// chartWSReadLimit 은 클라이언트→서버 프레임 최대 크기이다.
	// 차트 구독은 단방향에 가까우므로 작게 둔다.
	chartWSReadLimit = 512

	// chartWSSendBufferSize 는 구독자 전송 채널 용량이다.
	// 가득 차면 slow consumer 로 간주하고 즉시 구독을 종료한다.
	chartWSSendBufferSize = 256
)

// chartWSPingPeriod 는 서버→클라이언트 ping 주기이다 (pongWait 의 ~90%).
// var 로 선언하여 테스트에서 짧게 오버라이드할 수 있도록 한다.
var chartWSPingPeriod = 54 * time.Second

// chartWSUpgrader 는 차트 채널 전용 업그레이더이다.
// 기존 /ws 의 upgrader 와 동일 설정이지만 모듈 독립성을 위해 별도 보유한다.
var chartWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: 프로덕션에서는 허용 오리진 제한 필요.
		return true
	},
}

// chartSubscriberSeq 는 구독자 식별자 생성용 원자 카운터이다.
var chartSubscriberSeq atomic.Int64

// ChartChannelHandler 는 /ws/chart/{channel} 엔드포인트를 처리하는 HTTP 핸들러이다.
// 각 업그레이드된 연결에 대해 chartWSSubscriber 를 생성하고
// system.ChartChannelRegistry 의 채널에 구독시킨다.
type ChartChannelHandler struct {
	reg    *system.ChartChannelRegistry
	logger *slog.Logger
}

// NewChartChannelHandler 는 ChartChannelHandler 를 생성한다.
// reg 는 반드시 비-nil 이어야 한다 (production 에서는 main.go 에서 주입).
// 테스트에서는 각 테스트가 자체 레지스트리를 주입한다.
func NewChartChannelHandler(reg *system.ChartChannelRegistry, logger *slog.Logger) *ChartChannelHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChartChannelHandler{reg: reg, logger: logger}
}

// HandleUpgrade 는 /ws/chart/{channel} 엔드포인트의 http.HandlerFunc 이다.
//
// 동작 순서:
//  1. path var {channel} 추출 → ValidateChartChannelName. 실패 시 400 JSON 응답 (업그레이드 없음).
//  2. 레지스트리 조회 실패 시: 업그레이드 → chart.error 전송 → 종료.
//  3. 성공 시: 구독자 생성, backfill 전송, read loop 로 클라이언트 disconnect 감지.
func (h *ChartChannelHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	channelName := r.PathValue("channel")

	// (1) 이름 형식 검증 — 업그레이드 전에 HTTP 400 응답.
	if err := system.ValidateChartChannelName(channelName); err != nil {
		h.writeJSONError(w, http.StatusBadRequest, "invalid_channel_name", err.Error())
		return
	}

	// (2) 레지스트리 조회. nil 레지스트리는 프로세스 구성 오류이므로 500.
	if h.reg == nil {
		h.writeJSONError(w, http.StatusInternalServerError,
			"registry_unavailable", "chart channel registry is not initialized")
		return
	}

	channel, found := h.reg.Get(channelName)

	// (3) 업그레이드.
	conn, err := chartWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade 는 이미 응답을 썼을 수 있으므로 추가 응답 없음.
		h.logger.Error("chart ws: upgrade failed",
			"channel", channelName,
			"error", err.Error(),
			"remote_addr", r.RemoteAddr,
		)
		return
	}

	// (3a) 미존재 채널: chart.error 전송 후 종료.
	if !found {
		h.sendFrameAndClose(conn, channelName, system.EncodeChartError, "channel_not_found")
		return
	}

	// (3b) 구독자 생성 + 등록 + backfill 전송 + read loop 시작.
	sub := newChartWSSubscriber(conn, h.logger)
	sub.start()

	backfill := channel.Subscribe(sub)
	backfillBytes, err := system.EncodeChartBackfill(channelName, backfill)
	if err != nil {
		h.logger.Error("chart ws: encode backfill failed",
			"channel", channelName, "error", err.Error(),
		)
		channel.Unsubscribe(sub)
		_ = sub.Close()
		return
	}
	if sendErr := sub.Send(backfillBytes); sendErr != nil {
		h.logger.Warn("chart ws: backfill send failed",
			"channel", channelName, "sub_id", sub.ID(), "error", sendErr.Error(),
		)
		channel.Unsubscribe(sub)
		return
	}

	// read loop: 클라이언트 disconnect / close frame 감지용.
	// Publish 는 별도로 channel.Publish → sub.Send 경로로 이루어진다.
	go h.runReadLoop(conn, channel, sub, channelName)
}

// runReadLoop 는 클라이언트로부터 프레임을 읽어 disconnect 를 감지한다.
// 현재 SPEC 범위에서는 클라이언트 → 서버 메시지 처리가 없으므로 모든 수신은 무시한다.
// 읽기 에러 발생 시 구독 해제 및 연결 종료를 수행한다.
func (h *ChartChannelHandler) runReadLoop(
	conn *websocket.Conn,
	channel *system.ChartChannel,
	sub *chartWSSubscriber,
	channelName string,
) {
	defer func() {
		channel.Unsubscribe(sub)
		_ = sub.Close()
	}()

	conn.SetReadLimit(chartWSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(chartWSPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(chartWSPongWait))
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if !isExpectedWSCloseErr(err) {
				h.logger.Debug("chart ws: read loop exit",
					"channel", channelName, "sub_id", sub.ID(), "error", err.Error(),
				)
			}
			return
		}
	}
}

// writeJSONError 는 {"error":code,"reason":detail} 형식으로 HTTP JSON 에러를 기록한다.
func (h *ChartChannelHandler) writeJSONError(
	w http.ResponseWriter,
	status int,
	code string,
	reason string,
) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  code,
		"reason": reason,
	})
}

// sendFrameAndClose 는 한 프레임(encoder 가 직렬화) 전송 후 WS 연결을 종료한다.
// 채널 미존재 등 "즉시 알리고 종료" 시나리오에 사용한다.
func (h *ChartChannelHandler) sendFrameAndClose(
	conn *websocket.Conn,
	channelName string,
	encoder func(string, string) ([]byte, error),
	reason string,
) {
	defer func() {
		_ = conn.Close()
	}()

	frame, err := encoder(channelName, reason)
	if err != nil {
		h.logger.Error("chart ws: encode frame failed",
			"channel", channelName, "reason", reason, "error", err.Error(),
		)
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(chartWSWriteWait))
	if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
		h.logger.Debug("chart ws: write frame failed",
			"channel", channelName, "reason", reason, "error", err.Error(),
		)
	}
}

// isExpectedWSCloseErr 는 websocket.IsCloseError + 일반적인 netClosed/ io.EOF 패턴을
// 정상 종료로 간주할지 판정한다.
func isExpectedWSCloseErr(err error) bool {
	if err == nil {
		return true
	}
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		return true
	}
	return errors.Is(err, websocket.ErrCloseSent)
}

// --- chartWSSubscriber ---

// chartWSSubscriber 는 system.ChartSubscriber 를 구현하는 WebSocket 어댑터이다.
//
// 설계 포인트:
//   - Send 는 버퍼드 채널에 enqueue 만 수행 (비차단). 가득 차면 slow_consumer 로 판정하여
//     Close 호출 후 에러 반환 → publisher 측 데드락 방지.
//   - 단일 write pump 고루틴만 conn.WriteMessage 를 호출하므로 write mutex 불필요.
//   - Close 는 sync.Once 로 멱등하다 (send 채널 닫기, close frame 전송, conn 닫기).
type chartWSSubscriber struct {
	id     string
	conn   *websocket.Conn
	logger *slog.Logger

	sendCh chan []byte
	done   chan struct{}

	closeOnce sync.Once
}

// newChartWSSubscriber 는 주어진 WS 연결을 래핑하는 구독자를 생성한다.
// 호출자는 반드시 start() 를 호출하여 write pump 고루틴을 시작해야 한다.
func newChartWSSubscriber(conn *websocket.Conn, logger *slog.Logger) *chartWSSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	seq := chartSubscriberSeq.Add(1)
	return &chartWSSubscriber{
		id:     subscriberID(seq),
		conn:   conn,
		logger: logger,
		sendCh: make(chan []byte, chartWSSendBufferSize),
		done:   make(chan struct{}),
	}
}

// subscriberID 는 atomic 카운터로부터 구독자 식별자 문자열을 생성한다.
// 형식은 "chartws-<seq>" 로 디버깅에 충분하다.
func subscriberID(seq int64) string {
	// strconv 사용을 피해 fmt 대신 수동 포맷 — 빠르고 할당 최소화.
	// (음수 고려 없음: Add(1) 은 양의 증가만 한다)
	return "chartws-" + formatInt64(seq)
}

// formatInt64 는 int64 를 10진 문자열로 변환한다 (seq 만 다루므로 간단 구현).
func formatInt64(v int64) string {
	if v == 0 {
		return "0"
	}
	// 최대 20자리 (int64 한계) + 여유.
	var buf [20]byte
	pos := len(buf)
	n := v
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// ID 는 구독자 식별자를 반환한다.
func (s *chartWSSubscriber) ID() string { return s.id }

// Send 는 이미 직렬화된 JSON 바이트를 write pump 채널에 enqueue 한다.
// 채널이 가득 찬 경우 slow consumer 로 판정하고 Close 를 호출한 뒤 에러를 반환한다.
// 이 에러는 publisher 가 fan-out 중 폐기하도록 system.ChartChannel.Publish 에서
// `_ = s.Send(bytes)` 로 무시되므로 다른 구독자에게 영향을 주지 않는다.
func (s *chartWSSubscriber) Send(msgBytes []byte) error {
	select {
	case <-s.done:
		return errors.New("chart ws subscriber: closed")
	default:
	}

	select {
	case s.sendCh <- msgBytes:
		return nil
	case <-s.done:
		return errors.New("chart ws subscriber: closed")
	default:
		// 버퍼 full → slow consumer
		s.logger.Warn("chart ws subscriber: send buffer full, closing",
			"sub_id", s.id,
		)
		_ = s.Close()
		return errors.New("chart ws subscriber: send buffer full (slow_consumer)")
	}
}

// Close 는 구독자 전송 채널을 닫고 WS 연결을 종료한다 (멱등).
func (s *chartWSSubscriber) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		// write pump 가 done 신호 감지 후 스스로 종료되도록 신호만 보낸다.
		// conn.Close 는 write pump 에서 수행한다 (단일 writer 원칙 유지).
	})
	return nil
}

// start 는 write pump 고루틴을 시작한다.
// 이 고루틴만이 conn.WriteMessage 를 호출한다 (단일 writer).
func (s *chartWSSubscriber) start() {
	go s.writePump()
}

// writePump 는 sendCh 드레인 + 주기 ping + conn 정리를 담당한다.
func (s *chartWSSubscriber) writePump() {
	ticker := time.NewTicker(chartWSPingPeriod)
	defer func() {
		ticker.Stop()
		_ = s.conn.Close()
	}()

	for {
		select {
		case <-s.done:
			// 남은 버퍼를 flush 시도한 뒤 close 프레임 전송.
			for {
				select {
				case msg := <-s.sendCh:
					_ = s.writeText(msg)
				default:
					_ = s.conn.SetWriteDeadline(time.Now().Add(chartWSWriteWait))
					_ = s.conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					return
				}
			}
		case msg := <-s.sendCh:
			// 참고: Close 는 sendCh 를 닫지 않고 done 채널만 닫는다.
			// 따라서 ok=false 검증은 불필요하다 (single-writer 패턴).
			if err := s.writeText(msg); err != nil {
				// 쓰기 실패 → 상위 read loop 가 감지하여 unsubscribe 처리.
				s.logger.Debug("chart ws: write text failed",
					"sub_id", s.id, "error", err.Error(),
				)
				return
			}
		case <-ticker.C:
			_ = s.conn.SetWriteDeadline(time.Now().Add(chartWSWriteWait))
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// writeText 는 텍스트 프레임 한 개를 WS 로 전송한다.
func (s *chartWSSubscriber) writeText(msg []byte) error {
	_ = s.conn.SetWriteDeadline(time.Now().Add(chartWSWriteWait))
	return s.conn.WriteMessage(websocket.TextMessage, msg)
}
