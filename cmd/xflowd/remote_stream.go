// remote_stream.go 는 M8(그룹 J) 원격 관리 클라이언트의 스트리밍 프록시 소스 어댑터를
// 정의한다(@SPEC:SPEC-REMOTE-001 M8, spec §5.10.2, REQ-J08/J08b).
//
// 노드에는 디바이스/에이전트 변경을 remote 가 구독할 수 있는 깔끔한 push 이벤트
// 소스가 없으므로(inventory.go 의 동일 제약), 본 스트림 소스는 **주기적 폴링**으로
// 실시간 갱신을 도출한다(device.State()/AgentStats 를 ticker 로 읽어 Updates 채널로
// push). 이는 cycle-free 하고 단위 테스트 가능하며, 로컬 패널의 폴링 동작(A12)과
// 동형이다. 향후 push 소스가 생기면 ticker 를 이벤트 트리거로 대체할 수 있다(seam).
//
// 백프레셔(REQ-J08b): Updates 채널은 작은 버퍼(1)를 사용하고, full 시 폴러는 최신값
// 우선으로 drop 한다(coalesce). client 펌프도 전송 직전 drain-to-latest 로 coalesce
// 하므로, 느린 소비자에서 프레임이 누적되지 않는다.
//
// teardown(REQ-J08b): Close 는 폴러 ctx 를 취소하여 ticker 고루틴을 종료한다(멱등).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/remote"
)

// defaultStreamPollInterval 은 스트림 폴링 기본 주기이다(실시간 패널 폴링 동형).
const defaultStreamPollInterval = 2 * time.Second

// remoteStreamSource 는 로컬 실시간 소스를 remote.StreamSource 로 어댑트한다(REQ-J08).
//
// M10(그룹 L): charts/logs 는 차트 채널 hub·로그 hub 를 in-process 로 탭하는 이벤트
// 구동 소스이다(REQ-L06/L07 — 폴링이 아닌 라이브 push, /ws/chart 자가 dial 없음).
// 미바인딩(nil)이면 해당 stream-action 은 ErrQueryActionUnsupported 를 반환한다.
type remoteStreamSource struct {
	agents       queryAgentReader
	devices      queryDeviceReader
	series       querySeriesReader
	pollInterval time.Duration
	charts       *system.ChartChannelRegistry // M10: chart.chart in-process 탭 (REQ-L07)
	logs         *logStreamHub                // M10: monitor.logs in-process 탭 (REQ-L06)
}

var _ remote.StreamSource = (*remoteStreamSource)(nil)

// newRemoteStreamSource 는 실시간 소스를 바인딩한 스트림 소스를 생성한다.
// pollInterval 이 0 이면 defaultStreamPollInterval 을 사용한다.
func newRemoteStreamSource(agents queryAgentReader, devices queryDeviceReader, series querySeriesReader, pollInterval time.Duration) *remoteStreamSource {
	if pollInterval <= 0 {
		pollInterval = defaultStreamPollInterval
	}
	return &remoteStreamSource{agents: agents, devices: devices, series: series, pollInterval: pollInterval}
}

// Subscribe 는 domain/streamAction 의 실시간 소스를 구독한다(REQ-J08). 미지원
// action 은 remote.ErrQueryActionUnsupported 를 반환한다.
//
// M10(그룹 L): chart.chart/monitor.logs 는 id 기반이 아닌 이벤트 구동 소스이므로 폴링
// 분기 전에 처리한다(REQ-L06/L07). 그 외(device.state/agent.stats/agent.series)는 기존
// 폴링 경로를 유지한다.
func (s *remoteStreamSource) Subscribe(ctx context.Context, domain, action string, args json.RawMessage) (remote.StreamSubscription, error) {
	// M10: 이벤트 구동 라이브 소스(id 미사용) — 폴링 분기 전에 처리한다.
	switch {
	case domain == remote.DomainChart && action == remote.StreamActionChart:
		return s.subscribeChart(args)
	case domain == remote.DomainMonitor && action == remote.StreamActionLogs:
		return s.subscribeLogs()
	}

	id, err := queryID(args)
	if err != nil {
		return nil, err
	}

	// poll 함수는 한 번의 스냅샷 JSON 을 반환한다(redaction 은 client 가 전송 전 수행).
	var poll func() (json.RawMessage, error)
	switch {
	case domain == remote.DomainDevice && action == remote.StreamActionState:
		poll = func() (json.RawMessage, error) {
			dev, derr := s.devices.Get(id)
			if derr != nil {
				return nil, derr
			}
			return marshalQuery(dev.State())
		}
	case domain == remote.DomainAgent && action == remote.StreamActionStats:
		poll = func() (json.RawMessage, error) {
			info, serr := s.agents.AgentStats(ctx, id)
			if serr != nil {
				return nil, serr
			}
			return marshalQuery(info)
		}
	case domain == remote.DomainAgent && action == remote.StreamActionSeries:
		// agent.series 라이브 소스: TSDB 시리즈 키 목록을 주기적으로 스냅샷한다
		// (GET /tsdb/series 와 동일 형상 — REQ-J08). series 는 agent NAME 기반이므로
		// 먼저 id→name 을 해소한다.
		poll = func() (json.RawMessage, error) {
			info, gerr := s.agents.GetAgent(ctx, id, "")
			if gerr != nil {
				return nil, gerr
			}
			keys, serr := s.series.SeriesList(ctx, info.Name)
			if serr != nil {
				return nil, serr
			}
			if keys == nil {
				keys = []string{}
			}
			return marshalQuery(map[string]any{"series": keys, "count": len(keys)})
		}
	default:
		// device.stats 등 스트림 소스가 없는 조합은 미지원(폴링 폴백 대상).
		return nil, fmt.Errorf("%w: stream %s/%s", remote.ErrQueryActionUnsupported, domain, action)
	}

	return newPollingSubscription(ctx, poll, s.pollInterval), nil
}

// pollingSubscription 은 주기적 폴링으로 갱신을 흘리는 remote.StreamSubscription
// 구현이다(REQ-J08). ctx 취소/Close 시 폴러 고루틴이 종료된다(teardown — REQ-J08b).
type pollingSubscription struct {
	updates   chan json.RawMessage
	cancel    context.CancelFunc
	closeOnce sync.Once
}

var _ remote.StreamSubscription = (*pollingSubscription)(nil)

// newPollingSubscription 은 poll 을 interval 주기로 호출하는 구독을 시작한다.
func newPollingSubscription(parent context.Context, poll func() (json.RawMessage, error), interval time.Duration) *pollingSubscription {
	ctx, cancel := context.WithCancel(parent)
	sub := &pollingSubscription{
		updates: make(chan json.RawMessage, 1), // 버퍼 1 — 백프레셔(최신값 우선).
		cancel:  cancel,
	}
	go sub.run(ctx, poll, interval)
	return sub
}

// run 은 ticker 로 poll 을 호출하여 갱신을 push 한다. 채널 full 시 최신값 우선으로
// 기존 대기 값을 비우고 새 값을 넣는다(coalesce — REQ-J08b 백프레셔).
func (p *pollingSubscription) run(ctx context.Context, poll func() (json.RawMessage, error), interval time.Duration) {
	defer close(p.updates)

	emit := func() {
		v, err := poll()
		if err != nil || v == nil {
			return // 일시적 오류는 스킵(다음 주기 재시도). 소스 영구 오류는 상위 teardown.
		}
		// 백프레셔 coalesce: 채널이 full 이면 기존 값을 버리고 최신값을 넣는다.
		select {
		case p.updates <- v:
		default:
			select {
			case <-p.updates: // 기존 대기 값 제거.
			default:
			}
			select {
			case p.updates <- v:
			default:
			}
		}
	}

	// 즉시 1회 방출(첫 스냅샷) 후 주기 폴링.
	emit()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			emit()
		}
	}
}

// Updates 는 갱신 채널을 반환한다.
func (p *pollingSubscription) Updates() <-chan json.RawMessage { return p.updates }

// Close 는 폴러를 종료한다(멱등 — REQ-J08b teardown).
func (p *pollingSubscription) Close() error {
	p.closeOnce.Do(p.cancel)
	return nil
}

// --- M10 그룹 L: 차트 in-process 스트림 (REQ-L07) -----------------------------

// chartStreamArgs 는 chart.chart 구독 인자({channelName})이다(REQ-L07).
type chartStreamArgs struct {
	ChannelName string `json:"channelName"`
}

// subscribeChart 는 노드의 in-process 차트 채널 hub 를 직접 구독해 backfill/append
// 프레임을 중계한다(REQ-L07 — /ws/chart/{channel} 자가 dial 금지, 노드 내부 탭).
//
// channelName 으로 레지스트리에서 채널을 찾고, in-process ChartSubscriber 어댑터를
// 등록한다. Subscribe 가 반환한 backfill 을 chart.backfill 프레임으로 즉시 흘리고,
// 이후 Publish 가 어댑터.Send 로 흘리는 chart.append 프레임을 Updates 로 중계한다.
// Close 시 channel.Unsubscribe 로 teardown 한다(REQ-J08b — 누수 없음).
func (s *remoteStreamSource) subscribeChart(args json.RawMessage) (remote.StreamSubscription, error) {
	if s.charts == nil {
		return nil, fmt.Errorf("%w: chart registry 미바인딩", remote.ErrQueryActionUnsupported)
	}
	var p chartStreamArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("chart 구독 인자 디코드 실패: %w", err)
	}
	if p.ChannelName == "" {
		return nil, fmt.Errorf("chart 구독: channelName 은 필수입니다")
	}
	channel, ok := s.charts.Get(p.ChannelName)
	if !ok {
		return nil, fmt.Errorf("chart 채널 미존재: %s", p.ChannelName)
	}

	sub := newChartStreamSubscription(channel)
	// 노드 채널에 in-process 구독자 등록 → backfill 수신.
	backfill := channel.Subscribe(sub)
	if frame, err := system.EncodeChartBackfill(p.ChannelName, backfill); err == nil {
		sub.enqueue(frame) // backfill 프레임을 첫 갱신으로 흘린다.
	}
	return sub, nil
}

// chartStreamSubscription 은 in-process ChartSubscriber 이자 remote.StreamSubscription
// 이다(REQ-L07). Send(노드 채널 fan-out)가 Updates 채널로 프레임을 흘린다.
type chartStreamSubscription struct {
	channel   *system.ChartChannel
	updates   chan json.RawMessage
	id        string
	closeOnce sync.Once
}

var (
	_ remote.StreamSubscription = (*chartStreamSubscription)(nil)
	_ system.ChartSubscriber    = (*chartStreamSubscription)(nil)
)

// chartSubSeq 는 in-process 차트 구독자 식별자 카운터이다.
var chartSubSeq struct {
	mu  sync.Mutex
	val int64
}

// newChartStreamSubscription 은 차트 채널을 래핑한 구독을 생성한다.
func newChartStreamSubscription(channel *system.ChartChannel) *chartStreamSubscription {
	chartSubSeq.mu.Lock()
	chartSubSeq.val++
	seq := chartSubSeq.val
	chartSubSeq.mu.Unlock()
	return &chartStreamSubscription{
		channel: channel,
		updates: make(chan json.RawMessage, chartStreamBuffer),
		id:      fmt.Sprintf("remote-chart-%d", seq),
	}
}

// chartStreamBuffer 는 차트 구독 채널 버퍼 크기이다(backfill + 다수 append 수용).
const chartStreamBuffer = 256

// ID 는 구독자 식별자를 반환한다(ChartSubscriber).
func (c *chartStreamSubscription) ID() string { return c.id }

// Send 는 노드 차트 채널의 fan-out 프레임(backfill/append/closed)을 Updates 로 흘린다
// (ChartSubscriber). 백프레셔: 버퍼 full 시 최신값 우선 coalesce(느린 소비자 보호).
func (c *chartStreamSubscription) Send(msgBytes []byte) error {
	c.enqueue(append(json.RawMessage(nil), msgBytes...))
	return nil
}

// enqueue 는 프레임을 Updates 로 비차단 송신한다(백프레셔 — 최신값 우선).
func (c *chartStreamSubscription) enqueue(frame json.RawMessage) {
	select {
	case c.updates <- frame:
		return
	default:
	}
	select {
	case <-c.updates:
	default:
	}
	select {
	case c.updates <- frame:
	default:
	}
}

// Updates 는 갱신 채널을 반환한다(StreamSubscription).
func (c *chartStreamSubscription) Updates() <-chan json.RawMessage { return c.updates }

// Close 는 노드 차트 채널에서 구독을 해제한다(teardown — REQ-J08b, 멱등).
// updates 는 닫지 않는다(Send 와의 송신/close 경합 회피 — client 펌프가 ctx 로 종료).
func (c *chartStreamSubscription) Close() error {
	c.closeOnce.Do(func() {
		c.channel.Unsubscribe(c)
	})
	return nil
}

// --- M10 그룹 L: 로그 in-process 스트림 (REQ-L06) -----------------------------

// subscribeLogs 는 노드의 로그 hub 를 구독해 라이브 tail 을 중계한다(REQ-L06 —
// 라이브 스트림, 캐시 우회). Close 시 hub 에서 구독자를 제거한다(teardown — REQ-J08b).
func (s *remoteStreamSource) subscribeLogs() (remote.StreamSubscription, error) {
	if s.logs == nil {
		return nil, fmt.Errorf("%w: log hub 미바인딩", remote.ErrQueryActionUnsupported)
	}
	sub := s.logs.subscribe()
	return &logStreamSubscription{hub: s.logs, sub: sub}, nil
}

// logStreamSubscription 은 로그 hub 구독을 remote.StreamSubscription 으로 어댑트한다.
type logStreamSubscription struct {
	hub       *logStreamHub
	sub       *logStreamSub
	closeOnce sync.Once
}

var _ remote.StreamSubscription = (*logStreamSubscription)(nil)

// Updates 는 로그 라인 채널을 반환한다(StreamSubscription).
func (l *logStreamSubscription) Updates() <-chan json.RawMessage { return l.sub.ch }

// Close 는 hub 에서 구독을 해제한다(teardown — REQ-J08b, 멱등).
func (l *logStreamSubscription) Close() error {
	l.closeOnce.Do(func() {
		l.hub.unsubscribe(l.sub)
	})
	return nil
}
