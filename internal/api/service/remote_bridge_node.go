// remote_bridge_node.go 는 P3 매니저 측 엔진 remote-bridge 통합을 구현한다
// (@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB01/RB05/RB06/RB07/RB09/RB12).
//
// 배경 — 살아남은 remote:// flow-node 를 라이브 노드로 실행:
//
//	ExpandSubflows 는 remote:// flow-node 를 확장하지 않고 LIVE NODE 로 남긴다(RB01). P3 은
//	이 살아남은 flow-node 를 매니저 측 엔진에서 라이브 브리지 엔드포인트로 실행한다. flow-node
//	의 INPUT 포트로 들어온 메시지는 bridge.SendInput(port,data)로 원격 노드에 주입되고(WRITE),
//	bridge.Outputs()로 수신한 {port,data}는 flow-node 의 OUTPUT 포트로 emit 되어 하류 와이어로
//	전달된다(READ). flow-node 인스턴스마다 독립 브리지(RB12)이다.
//
// 엔진 제약(단일 노드는 source 또는 processor):
//
//	엔진은 입력 와이어가 있는 노드의 SourceCh 를 드레인하지 않는다(engine.go — source 분기는
//	len(inputWires)==0 한정). 따라서 입력 수신(Process)과 비동기 출력 emit(SourceCh)을 한
//	노드가 동시에 할 수 없다. bridge_runner.go(노드 측)와 동일하게, 살아남은 remote flow-node
//	를 두 실제 노드로 재배선한다:
//	  - 입력 forwarder(ProcessNode): flow-node INPUT 포트 와이어를 받아 Process 에서
//	    controller.forwardInput(port,data) → bridge.SendInput. 즉 `upstream→rfn:inP` 를
//	    `upstream→fwd:inP` 로 재배선.
//	  - 출력 emitter(MultiSourceNode): controller.onOutput(bridge.Outputs 펌프)이 포트별
//	    채널로 흘린 메시지를 엔진이 OUTPUT 와이어로 라우팅. 즉 `rfn:outP→downstream` 를
//	    `emit:outP→downstream` 로 재배선.
//	두 노드는 NodeDef.Config 의 bridgeNodeID 로 process-global managerBridgeTable 에서 per-
//	flow-node 컨트롤러를 찾아 결선한다(tap 노드와 동일 side-channel 패턴 — registry.Create 가
//	NodeDef 만 받으므로).
//
// 라이프사이클: 컨트롤러는 forwarder 노드 Init 에서 start(OpenBridge), shutdown 에서
// stop(bridge.Close)한다(노드 라이프사이클이 브리지 라이프사이클을 구동 — RB09). 출력
// emitter 는 펌프 채널만 보유하므로 controller 가 채널을 close 해 emitter 고루틴을 종료한다.
//
// FlowBridgeOpener 주입: managerBridgeTable 에 컨트롤러를 등록할 때 opener 가 바인딩된다
// (rewireRemoteBridges 가 opener 를 받아 컨트롤러를 만든다). cmd/xflowd(server 모드)가
// FlowServiceAdapter 에 opener 를 설정하고, DeployFlow 가 재배선 시 사용한다. 비-server
// 모드(opener nil)는 remote:// flow-node 배포를 명확한 오류로 거부한다(RB 미지원 모드).
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/pkg/flow"
)

const (
	// bridgeFwdType / bridgeEmitType 은 매니저 측 입력 forwarder / 출력 emitter 노드 타입이다.
	// server-mode 엔진 레지스트리에만 등록되며(RegisterRemoteBridgeNodes), 일반 플로우에는
	// 나타나지 않는다(remote:// flow-node 재배선으로만 생성).
	bridgeFwdType  = "__remote_bridge_fwd__"
	bridgeEmitType = "__remote_bridge_emit__"

	// bridgeNodeIDConfigKey 는 fwd/emit 노드가 자신의 per-flow-node 컨트롤러를 찾는 키이다.
	bridgeNodeIDConfigKey = "__remote_bridge_id__"
)

// ErrRemoteBridgeUnavailable 은 server 모드가 아니어서(FlowBridgeOpener 미주입) remote://
// flow-node 를 실행할 수 없을 때 반환된다(비-server 모드 명확한 거부 — RB 미지원).
var ErrRemoteBridgeUnavailable = fmt.Errorf("remote bridge unavailable: remote:// flow-node requires server mode (no FlowBridgeOpener injected)")

// ---------------------------------------------------------------------------
// 매니저 측 브리지 컨트롤러 + process-global 테이블
// ---------------------------------------------------------------------------

// managerBridgeController 는 한 살아남은 remote flow-node 인스턴스의 라이브 브리지 상태를
// 보유한다(RB12 — flow-node 별 독립). forwarder 가 start/stop 을 구동하고, emitter 가
// 출력 채널을 소비한다.
type managerBridgeController struct {
	bridgeNodeID string
	instanceID   string
	remoteFlowID string
	inputPorts   []string
	outputPorts  []string
	opener       FlowBridgeOpener
	logger       *slog.Logger

	mu     sync.Mutex
	bridge FlowBridge
	status BridgeStatus
	closed bool

	// onOutput 은 출력 emitter 노드가 설정하는 콜백이다(포트별 채널로 흘림). 펌프 고루틴이
	// bridge.Outputs() 를 읽어 호출한다. nil 이면 출력은 폐기된다(emitter 미배선 — 방어).
	// 재개설(self-healing) 시 onOutput 은 보존되며 c.bridge 와 펌프만 교체된다 — 출력이
	// 동일 콜백으로 투명하게 재개된다.
	onOutput func(port string, data json.RawMessage)

	// backoff 는 재개설 시도 간 대기 시간을 반환한다(attempt 는 0-기반 시도 번호). 기본은
	// 지수 백오프 + 지터(cap ~30s)이며, 테스트는 ms 스케일로 주입해 빠르게 수렴시킨다.
	backoff func(attempt int) time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newManagerBridgeController 는 컨트롤러를 생성한다(start 전 — opener 바인딩).
func newManagerBridgeController(instanceID, remoteFlowID string, inputPorts, outputPorts []string, opener FlowBridgeOpener, logger *slog.Logger) *managerBridgeController {
	if logger == nil {
		logger = slog.Default()
	}
	return &managerBridgeController{
		instanceID:   instanceID,
		remoteFlowID: remoteFlowID,
		inputPorts:   inputPorts,
		outputPorts:  outputPorts,
		opener:       opener,
		logger:       logger,
		backoff:      defaultReopenBackoff,
	}
}

// defaultReopenBackoff 는 재개설 시도 간 기본 대기 시간이다(지수 + 지터, cap 30s). 노드가
// 내려간 동안 OpenBridge 는 빠르게 실패하므로, 핫스핀을 막기 위해 시도 사이를 항상 지킨다.
//
//	attempt=0 → ~500ms, 1 → ~1s, 2 → ~2s, ... cap 30s. 각 단계에 0~25% 지터를 더해
//	동시 재연결 thundering-herd 를 완화한다.
func defaultReopenBackoff(attempt int) time.Duration {
	const (
		base   = 500 * time.Millisecond
		maxDur = 30 * time.Second
	)
	d := base
	for i := 0; i < attempt && d < maxDur; i++ {
		d *= 2
	}
	if d > maxDur {
		d = maxDur
	}
	// 0~25% 지터.
	jitter := time.Duration(rand.Int63n(int64(d)/4 + 1))
	return d + jitter
}

// start 는 라이브 브리지를 열고(RB05) self-healing 감독 고루틴을 시작한다. PERMANENT 오설정
// (opener 미주입 — server 모드 요구)만 deploy 를 실패시키고, TRANSIENT 초기 open 실패(부팅 시
// 노드 오프라인/미관리/open 타임아웃)는 deploy 를 실패시키지 않는다.
//
//	deploy-time 게이팅(PERMANENT): opener==nil 이면 ErrRemoteBridgeUnavailable 을 반환해 배포를
//	실패시킨다(비-server 모드는 remote:// flow-node 를 실행할 수 없다, 기존 동작 유지).
//
//	초기 open 성공(fast path): caller(deploy) ctx 로 동기 개설에 성공하면 c.bridge 를 즉시
//	설정해 start() 반환 직후 forwardInput 이 동작하고, 감독자가 그 브리지로 첫 세대를 바로
//	실행한다(펌프 즉시 시작). 노드 PROGRAM 재시작으로 브리지가 죽으면(펌프 종료/채널 close)
//	감독자가 bounded 백오프로 재개설해 노드가 돌아오면 자동 재연결한다(c.bridge + 펌프만 교체,
//	onOutput 보존 → 출력 투명 재개).
//
//	초기 open TRANSIENT 실패(offline-at-boot): 매니저 부팅 시 원격 노드는 아직 연결되지 않았다
//	(노드는 server.Start 가 WS 엔드포인트를 올린 뒤에야 dial-in 하며, 이는 auto_start 보다
//	나중이다). 이 경우 deploy 를 실패시키지 않고 c.bridge 를 nil 로 둔 채(forwardInput 은
//	조용히 드롭) 감독자를 "아직 열지 못함" 모드(nil 브리지)로 시작한다. 감독자는 reopen 백오프
//	루프로 OpenBridge 를 재시도해 노드가 온라인+관리됨이 되는 즉시 브리지를 열고 펌프를 시작한다
//	— 출력이 노드 도착 시 자동으로 재개된다.
func (c *managerBridgeController) start(ctx context.Context) error {
	if c.opener == nil {
		return ErrRemoteBridgeUnavailable // PERMANENT — server 모드 요구.
	}

	// 초기 open: caller(deploy) ctx 로 한 번 동기 시도한다(fast path 보존).
	bridge, err := c.opener.OpenBridge(ctx, c.instanceID, c.remoteFlowID, c.inputPorts, c.outputPorts)

	// 감독 ctx 는 stop() 이 취소한다(백오프 sleep + 진행 중 OpenBridge + 펌프 종료). cancel 은
	// 고루틴 시작 전에 mu 하에 설정해 stop() 이 초기 open-retry 루프까지 취소할 수 있게 한다.
	superCtx, cancel := context.WithCancel(context.Background())

	if err != nil {
		// TRANSIENT(노드 오프라인/미관리/타임아웃) — deploy 를 실패시키지 않는다. c.bridge 는
		// nil 로 두고(forwardInput 조용히 드롭), 감독자를 nil 브리지로 시작해 노드 도착 시 연다.
		c.mu.Lock()
		c.cancel = cancel
		c.mu.Unlock()

		c.wg.Add(1)
		go c.supervise(superCtx, nil)

		c.logger.Info("매니저 라이브 브리지 초기 미가용 — 노드 연결 대기(오프라인 시작)",
			"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
			"input_ports", c.inputPorts, "output_ports", c.outputPorts, "err", err)
		return nil
	}

	c.mu.Lock()
	c.cancel = cancel
	// 초기 브리지를 동기적으로 설정한다 — start() 반환 직후 forwardInput 이 즉시 동작하도록
	// (deploy-time 계약). 감독자의 첫 세대 runGeneration 이 같은 값을 재설정하지만 멱등이다.
	c.bridge = bridge
	c.mu.Unlock()

	c.wg.Add(1)
	go c.supervise(superCtx, bridge)

	c.logger.Info("매니저 라이브 브리지 시작",
		"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
		"input_ports", c.inputPorts, "output_ports", c.outputPorts)
	return nil
}

// supervise 는 self-healing 라이프사이클을 소유한다: 현재 세대 브리지를 펌프로 실행하고,
// 두 펌프가 종료하면(세대 종료) 의도적 stop 인지 노드 드롭인지 판별한다. stop(c.closed)이면
// 종료, 아니면 bounded 백오프로 재개설한 뒤 다음 세대를 실행한다.
//
//	bridge==nil(never-opened-yet — offline-at-boot)로 진입하면 먼저 reopen 백오프 루프로 첫
//	라이브 브리지를 연다(노드 도착까지 재시도). 이는 노드 드롭 후 재개설과 동일한 reopen 경로를
//	재사용하므로 "초기 미가용"과 "사후 드롭"이 단일 코드 경로로 자가치유된다. bridge!=nil(초기
//	open 성공 fast path)이면 그 브리지로 첫 세대를 바로 실행한다(펌프 즉시 시작).
//
//	감독자 자신이 c.wg 에 1 을 보유한 채로 매 세대의 펌프(c.wg.Add(2))를 추가하므로,
//	stop() 의 c.wg.Wait() 가 Add 와 경합해 카운터가 0 으로 떨어지는 일이 없다(감독자가 살아
//	있는 동안 카운터 >= 1). 따라서 단일 c.wg 로 안전하게 모든 고루틴을 추적한다.
func (c *managerBridgeController) supervise(ctx context.Context, bridge FlowBridge) {
	defer c.wg.Done()
	attempt := 0

	// never-opened-yet: 초기 open 이 transient 로 실패해 nil 브리지로 시작했다 — 노드가
	// 도착할 때까지 reopen 백오프 루프로 첫 브리지를 연다. stop(ctx 취소) 시 깔끔히 종료한다.
	if bridge == nil {
		first, ok := c.reopen(ctx, &attempt)
		if !ok {
			return // 첫 open 전 stop — 펌프 미시작, 회수할 브리지 없음.
		}
		bridge = first
	}

	for {
		genWG := c.runGeneration(ctx, bridge)
		// 세대 종료 대기: 두 펌프가 종료하면 반환한다. stop(ctx 취소) 또는 노드 드롭(채널
		// close) 모두에서 펌프가 종료하므로 Wait 는 항상 풀린다.
		genWG.Wait()

		c.mu.Lock()
		closed := c.closed
		c.bridge = nil // 갭 구간: forwardInput 이 조용히 드롭하도록 nil 로 비운다.
		c.mu.Unlock()
		if closed {
			return // 의도적 stop — 재개설하지 않는다.
		}

		// 노드 드롭으로 브리지가 죽었다 — bounded 백오프로 재개설한다.
		newBridge, ok := c.reopen(ctx, &attempt)
		if !ok {
			return // 백오프/재개설 중 stop.
		}
		bridge = newBridge
	}
}

// runGeneration 은 한 세대 브리지를 활성화한다: c.bridge 를 swap 하고 출력/상태 펌프를
// 시작한 뒤, 두 펌프 종료를 신호하는 WaitGroup 을 반환한다. 펌프는 ctx 와 c.wg 양쪽에
// 추적되어 stop(ctx 취소)과 노드 드롭(채널 close) 모두에서 종료한다.
func (c *managerBridgeController) runGeneration(ctx context.Context, bridge FlowBridge) *sync.WaitGroup {
	c.mu.Lock()
	c.bridge = bridge
	c.mu.Unlock()

	genWG := &sync.WaitGroup{}
	genWG.Add(2)
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		defer genWG.Done()
		c.pumpOutputs(ctx, bridge.Outputs())
	}()
	go func() {
		defer c.wg.Done()
		defer genWG.Done()
		c.pumpStatus(ctx, bridge.Status())
	}()
	return genWG
}

// reopen 은 노드가 돌아올 때까지 bounded 백오프로 OpenBridge 를 재시도한다. 성공 시 새
// 브리지와 true 를, stop(ctx 취소) 시 nil 과 false 를 반환한다. attempt 는 호출 간 누적되어
// 백오프가 점증한다(핫스핀 방지). 백오프 sleep 과 OpenBridge 모두 ctx 로 취소 가능하다.
func (c *managerBridgeController) reopen(ctx context.Context, attempt *int) (FlowBridge, bool) {
	for {
		// 시도 사이를 항상 지킨다(노드가 내려간 동안 OpenBridge 가 빠르게 실패하므로
		// 핫스핀 방지). 백오프 sleep 은 ctx 로 취소 가능하다.
		delay := c.backoff(*attempt)
		*attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
		}

		bridge, err := c.opener.OpenBridge(ctx, c.instanceID, c.remoteFlowID, c.inputPorts, c.outputPorts)
		if err != nil {
			if ctx.Err() != nil {
				return nil, false // stop 으로 인한 취소.
			}
			c.logger.Debug("매니저 라이브 브리지 재연결 대기(노드 미가용)",
				"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
				"attempt", *attempt, "err", err)
			continue
		}
		c.logger.Info("매니저 라이브 브리지 재연결",
			"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
			"attempt", *attempt)
		*attempt = 0 // 성공 — 다음 드롭을 위해 백오프 리셋.
		return bridge, true
	}
}

// pumpOutputs 는 bridge.Outputs() 를 읽어 onOutput 콜백으로 전달한다(READ — RB07). 출력
// emitter 가 이를 OUTPUT 포트 채널로 흘려 엔진이 하류 와이어로 라우팅한다. ctx 취소(stop)
// 또는 채널 close(노드 드롭/Close) 시 반환한다. wg 회계는 runGeneration 래퍼가 소유한다.
func (c *managerBridgeController) pumpOutputs(ctx context.Context, src <-chan BridgeOutput) {
	for {
		select {
		case <-ctx.Done():
			return
		case out, ok := <-src:
			if !ok {
				return
			}
			c.mu.Lock()
			cb := c.onOutput
			c.mu.Unlock()
			if cb != nil {
				cb(out.Port, out.Data)
			}
		}
	}
}

// pumpStatus 는 bridge.Status() 를 읽어 lastStatus 를 갱신한다(RB09 — P4 web 노출/오프라인
// 무출력 토대). 여기서는 노출/로그만 한다.
func (c *managerBridgeController) pumpStatus(ctx context.Context, src <-chan BridgeStatus) {
	for {
		select {
		case <-ctx.Done():
			return
		case st, ok := <-src:
			if !ok {
				return
			}
			c.mu.Lock()
			c.status = st
			c.mu.Unlock()
			c.logger.Debug("매니저 라이브 브리지 상태",
				"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
				"state", st.State, "detail", st.Detail)
		}
	}
}

// forwardInput 은 forwarder 노드가 호출한다: 입력 메시지를 bridge.SendInput 으로 주입한다
// (WRITE — RB07, 이름 기반 포트 매핑 — RB06).
func (c *managerBridgeController) forwardInput(port string, data json.RawMessage) error {
	c.mu.Lock()
	b := c.bridge
	closed := c.closed
	c.mu.Unlock()
	if closed || b == nil {
		return nil // 미시작/이미 종료 — 조용히 무시(노드 정지 경합).
	}
	return b.SendInput(port, data)
}

// lastStatus 는 마지막으로 관측된 브리지 상태를 반환한다(RB09 노출).
func (c *managerBridgeController) lastStatus() BridgeStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// setOnOutput 은 출력 emitter 노드가 자신의 포트별 채널 흘림 콜백을 등록한다.
func (c *managerBridgeController) setOnOutput(fn func(port string, data json.RawMessage)) {
	c.mu.Lock()
	c.onOutput = fn
	c.mu.Unlock()
}

// stop 은 브리지를 닫고 감독자 + 펌프 + 진행 중 백오프를 종료한다(멱등 — RB09).
//
//	closed=true 를 mu 하에 설정해 감독자의 세대-종료 후 판별이 stop 을 인지하게 하고(재개설
//	금지), 감독 ctx 를 취소해 백오프 sleep·진행 중 OpenBridge·펌프(ctx.Done)를 모두 깨운다.
//	그 뒤 현재 브리지를 닫고 c.wg.Wait() 로 감독자+펌프를 회수한다. 재개설 갭 중 stop 이
//	들어와 b==nil 을 캡처했더라도, 감독자가 막 swap 한 브리지를 Wait 후 한 번 더 닫아 원격
//	tap 누수를 방지한다.
func (c *managerBridgeController) stop(_ context.Context) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	b := c.bridge
	cancel := c.cancel
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if b != nil {
		_ = b.Close() // bridge_close + 채널 close → 펌프 종료.
	}
	c.wg.Wait()

	// 갭 경합 방어: 감독자가 종료 직전 swap 한 브리지가 남아 있으면 닫는다(멱등 Close).
	c.mu.Lock()
	last := c.bridge
	c.bridge = nil
	c.mu.Unlock()
	if last != nil && last != b {
		_ = last.Close()
	}

	c.logger.Info("매니저 라이브 브리지 종료",
		"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID)
}

// managerBridgeTable 은 bridgeNodeID → 컨트롤러 매핑의 process-global 레지스트리이다.
// fwd/emit 노드 Init 이 NodeDef.Config 의 bridgeNodeID 로 컨트롤러를 찾아 결선한다.
type managerBridgeTable struct {
	mu sync.RWMutex
	m  map[string]*managerBridgeController
}

func newManagerBridgeTable() *managerBridgeTable {
	return &managerBridgeTable{m: make(map[string]*managerBridgeController)}
}

func (t *managerBridgeTable) register(id string, c *managerBridgeController) {
	t.mu.Lock()
	t.m[id] = c
	t.mu.Unlock()
}

func (t *managerBridgeTable) unregister(id string) {
	t.mu.Lock()
	delete(t.m, id)
	t.mu.Unlock()
}

func (t *managerBridgeTable) lookup(id string) (*managerBridgeController, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	c, ok := t.m[id]
	return c, ok
}

// globalManagerBridgeTable 은 fwd/emit 노드와 deploy 경로가 공유하는 process-global 테이블이다.
var globalManagerBridgeTable = newManagerBridgeTable()

var managerBridgeIDCounter atomic.Uint64

// newManagerBridgeID 는 프로세스 내 유일한 bridgeNodeID 를 생성한다(RB12 — flow-node 별).
func newManagerBridgeID() string {
	return fmt.Sprintf("mbridge-%d", managerBridgeIDCounter.Add(1))
}

// ---------------------------------------------------------------------------
// 재배선: 살아남은 remote flow-node → fwd + emit 쌍
// ---------------------------------------------------------------------------

// bridgeFwdNodeID / bridgeEmitNodeID 는 remote flow-node id 에서 파생한 fwd/emit 노드 id 이다.
func bridgeFwdNodeID(fnID string) string  { return "__rbridge_fwd__" + fnID }
func bridgeEmitNodeID(fnID string) string { return "__rbridge_emit__" + fnID }

// rewireRemoteBridges 는 f 의 살아남은 remote:// flow-node 를 입력 forwarder + 출력 emitter
// 쌍으로 재배선한 새 플로우와, 등록된 컨트롤러 목록을 반환한다(RB01/RB07/RB12).
//
//	각 remote flow-node 마다 고유 bridgeNodeID 컨트롤러를 만들고 tbl 에 등록한다. 부모 와이어
//	중 flow-node 를 INPUT 으로 갖는 와이어(upstream→fn:inP)는 fwd 노드로, flow-node 를 OUTPUT
//	으로 갖는 와이어(fn:outP→downstream)는 emit 노드로 재배선한다. flow-node 자체는 결과에서
//	제거된다(fwd+emit 로 치환).
//
// opener 가 nil 이고 remote flow-node 가 존재하면 ErrRemoteBridgeUnavailable 을 반환한다
// (비-server 모드 명확한 거부 — RB 미지원).
func rewireRemoteBridges(f flow.Flow, opener FlowBridgeOpener, tbl *managerBridgeTable, logger *slog.Logger) (flow.Flow, []*managerBridgeController, error) {
	srcNodes := f.Nodes()
	srcWires := f.Wires()

	// 살아남은 remote flow-node 를 식별한다.
	type remoteFN struct {
		id           string
		instanceID   string
		remoteFlowID string
		inputPorts   []string
		outputPorts  []string
	}
	remotes := make(map[string]*remoteFN)
	for _, n := range srcNodes {
		if n.Type != flowNodeType {
			continue
		}
		flowID, _ := n.Config[flowNodeFlowIDKey].(string)
		instanceID, remoteFlowID, isRemote, perr := parseRemoteFlowRef(flowID)
		if perr != nil {
			return nil, nil, fmt.Errorf("flow-node %q: %w", n.ID, perr)
		}
		if !isRemote {
			continue // LOCAL flow-node 는 ExpandSubflows 에서 이미 소비됨(여기 도달 시 방어).
		}
		rf := &remoteFN{id: n.ID, instanceID: instanceID, remoteFlowID: remoteFlowID}
		for _, p := range n.Inputs {
			rf.inputPorts = append(rf.inputPorts, p.Name)
		}
		for _, p := range n.Outputs {
			rf.outputPorts = append(rf.outputPorts, p.Name)
		}
		remotes[n.ID] = rf
	}

	if len(remotes) == 0 {
		return f, nil, nil // 원격 참조 없음 — 원본 그대로(회귀 0).
	}
	if opener == nil {
		return nil, nil, ErrRemoteBridgeUnavailable
	}

	// 비-remote 노드는 보존, remote flow-node 는 제거 + fwd/emit 추가.
	resultNodes := make([]flow.NodeDef, 0, len(srcNodes)+len(remotes)*2)
	for _, n := range srcNodes {
		if _, isRemote := remotes[n.ID]; isRemote {
			continue // remote flow-node 제거(fwd+emit 로 치환).
		}
		resultNodes = append(resultNodes, n)
	}

	controllers := make([]*managerBridgeController, 0, len(remotes))
	for _, rf := range remotes {
		bridgeNodeID := newManagerBridgeID()
		ctrl := newManagerBridgeController(rf.instanceID, rf.remoteFlowID,
			rf.inputPorts, rf.outputPorts, opener, logger)
		ctrl.bridgeNodeID = bridgeNodeID
		tbl.register(bridgeNodeID, ctrl)
		controllers = append(controllers, ctrl)

		// fwd 노드: flow-node 입력 포트를 입력 포트로 선언(엔진이 입력 와이어 매핑).
		if len(rf.inputPorts) > 0 {
			resultNodes = append(resultNodes,
				bridgeNodeDef(bridgeFwdNodeID(rf.id), bridgeFwdType, bridgeNodeID, rf.inputPorts, nil))
		}
		// emit 노드: flow-node 출력 포트를 출력 포트로 선언(엔진이 출력 와이어 라우팅).
		if len(rf.outputPorts) > 0 {
			resultNodes = append(resultNodes,
				bridgeNodeDef(bridgeEmitNodeID(rf.id), bridgeEmitType, bridgeNodeID, nil, rf.outputPorts))
		}
	}

	// 와이어 재배선: flow-node 를 INPUT/OUTPUT 으로 갖는 와이어를 fwd/emit 로 향하게 한다.
	resultWires := make([]flow.Wire, 0, len(srcWires))
	for _, w := range srcWires {
		nw := w
		if _, ok := remotes[w.TargetNodeID]; ok {
			// upstream → fn:inPort  ⇒  upstream → fwd:inPort.
			nw.TargetNodeID = bridgeFwdNodeID(w.TargetNodeID)
		}
		if _, ok := remotes[w.SourceNodeID]; ok {
			// fn:outPort → downstream  ⇒  emit:outPort → downstream.
			nw.SourceNodeID = bridgeEmitNodeID(w.SourceNodeID)
		}
		resultWires = append(resultWires, nw)
	}

	return flow.RebuildFlow(f, resultNodes, resultWires), controllers, nil
}

// bridgeNodeDef 는 fwd/emit 노드 정의를 만든다. Config 에 bridgeNodeID 를 실어 노드 Init 이
// 컨트롤러를 찾게 한다.
func bridgeNodeDef(id, typ, bridgeNodeID string, inputs, outputs []string) flow.NodeDef {
	nd := flow.NodeDef{
		ID:     id,
		Name:   id,
		Type:   typ,
		Config: map[string]any{bridgeNodeIDConfigKey: bridgeNodeID},
	}
	for _, p := range inputs {
		nd.Inputs = append(nd.Inputs, flow.Port{ID: p, Name: p, Direction: flow.PortInput})
	}
	for _, p := range outputs {
		nd.Outputs = append(nd.Outputs, flow.Port{ID: p, Name: p, Direction: flow.PortOutput})
	}
	return nd
}
