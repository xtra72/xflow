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

	"github.com/xtra/xflow/internal/remote"
)

// defaultStreamPollInterval 은 스트림 폴링 기본 주기이다(실시간 패널 폴링 동형).
const defaultStreamPollInterval = 2 * time.Second

// remoteStreamSource 는 로컬 실시간 소스를 remote.StreamSource 로 어댑트한다(REQ-J08).
type remoteStreamSource struct {
	agents       queryAgentReader
	devices      queryDeviceReader
	series       querySeriesReader
	pollInterval time.Duration
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

// Subscribe 는 domain/streamAction 의 실시간 소스를 폴링 구독한다(REQ-J08). 미지원
// action 은 remote.ErrQueryActionUnsupported 를 반환한다.
func (s *remoteStreamSource) Subscribe(ctx context.Context, domain, action string, args json.RawMessage) (remote.StreamSubscription, error) {
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
