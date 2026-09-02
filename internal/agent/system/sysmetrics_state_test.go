package system

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// --- 헬퍼 ---

// newStateAgent 는 수집 함수를 스텁으로 바꾼 뒤 에이전트를 만든다.
func newStateAgent(t *testing.T, opts map[string]any) *SysMetricsAgent {
	t.Helper()
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(opts))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}
	return a
}

// startAgent 는 에이전트를 띄우고 종료를 예약한다.
func startAgent(t *testing.T, a *SysMetricsAgent) {
	t.Helper()
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() 오류 = %v", err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background()) })
}

// --- AC-01: 표본 이전 ---

func TestSysMetricsState_표본_이전은_오류가_아니다(t *testing.T) {
	a := newStateAgent(t, nil)

	state := a.State()

	if state == nil {
		t.Fatal("State() 가 nil 을 반환했다 — 표본 이전도 정상 구간이다")
	}
	if got := state["status"]; got != sysMetricsStatusNoSample {
		t.Errorf("status = %v, want %q", got, sysMetricsStatusNoSample)
	}
	// 주기는 표본과 무관하게 설정에서 나오므로 이때도 실려야 한다.
	if _, ok := state["interval_seconds"]; !ok {
		t.Error("표본 이전에도 interval_seconds 는 실려야 한다")
	}
	if _, ok := state["collected_at"]; ok {
		t.Error("표본이 없으면 collected_at 이 없어야 한다")
	}
}

// --- AC-02: 표본 1회 후 구성 ---

func TestSysMetricsState_표본_한번_뒤_구성(t *testing.T) {
	a := newStateAgent(t, map[string]any{"interval": "1s"})
	startAgent(t, a)

	a.emitSample(a.done)
	state := a.State()

	if got := state["status"]; got != sysMetricsStatusRunning {
		t.Errorf("status = %v, want %q", got, sysMetricsStatusRunning)
	}
	ts, ok := state["collected_at"].(int64)
	if !ok || ts <= 0 {
		t.Fatalf("collected_at = %v, epoch ms 가 실려야 한다", state["collected_at"])
	}
	if got := state["interval_seconds"]; got != float64(1) {
		t.Errorf("interval_seconds = %v, want 1", got)
	}

	for _, key := range []string{"cpu", "memory", "storage", "disk_io", "network"} {
		if _, ok := state[key]; !ok {
			t.Errorf("지표 그룹 %q 가 없다", key)
		}
	}

	// 필드 이름은 배치 구성기와 같은 출처를 쓴다 — 패널로 보던 이름과 저장되는
	// 시계열의 이름이 갈라지면 안 된다.
	cpu, ok := state["cpu"].(MetricGroup)
	if !ok {
		t.Fatalf("cpu = %T, MetricGroup 이어야 한다", state["cpu"])
	}
	if cpu["usage_percent"] != 42.5 {
		t.Errorf("cpu.usage_percent = %v, want 42.5", cpu["usage_percent"])
	}
	// 코어 수는 시계열이 아니라 정적 정보라 배치에는 없고 state 에만 있다.
	if state["cpu_cores"] != 8 {
		t.Errorf("cpu_cores = %v, want 8", state["cpu_cores"])
	}
}

// --- AC-03: 소비자 없이도 갱신 (회귀 가드) ---

// 이 테스트가 이 기능의 핵심이다. 플로우를 만들지 않은 사용자에게 표본 채널은
// 늘 가득 찬 상태이며, 그때도 패널은 값을 봐야 한다.
func TestSysMetricsState_채널이_가득_차도_스냅샷은_갱신된다(t *testing.T) {
	a := newStateAgent(t, map[string]any{"interval": "1s"})
	startAgent(t, a)

	// 버퍼를 끝까지 채워 emitSample 의 방출이 반드시 실패(default 분기)하게 만든다.
	a.mu.RLock()
	ch := a.recvCh
	a.mu.RUnlock()
	for len(ch) < cap(ch) {
		ch <- []byte("x")
	}

	a.emitSample(a.done)
	first, ok := a.State()["collected_at"].(int64)
	if !ok {
		t.Fatal("방출이 막혀도 첫 스냅샷은 실려야 한다")
	}

	// 두 번째 표본도 같은 조건에서 갱신되어야 한다 (한 번만 되고 마는 것이 아니다).
	time.Sleep(2 * time.Millisecond)
	a.emitSample(a.done)
	second, ok := a.State()["collected_at"].(int64)
	if !ok {
		t.Fatal("두 번째 스냅샷이 없다")
	}

	if second < first {
		t.Errorf("collected_at 이 역행했다: %d → %d", first, second)
	}
	if len(ch) != cap(ch) {
		t.Errorf("버퍼가 소비되었다 — 테스트 전제가 깨졌다 (len=%d cap=%d)", len(ch), cap(ch))
	}
}

// --- AC-04: 수집 꺼짐은 키 부재 ---

func TestSysMetricsState_수집을_끈_지표는_키가_없다(t *testing.T) {
	a := newStateAgent(t, map[string]any{
		"interval":        "1s",
		"collect_storage": false,
		"collect_disk_io": false,
	})
	startAgent(t, a)

	a.emitSample(a.done)
	state := a.State()

	// 0 이나 빈 맵이 아니라 부재여야 한다 — 0 으로 실으면 "비어 있다" 와
	// "관측하지 않는다" 가 화면에서 구별되지 않는다.
	for _, key := range []string{"storage", "disk_io"} {
		if _, ok := state[key]; ok {
			t.Errorf("수집을 끈 %q 가 state 에 실렸다: %v", key, state[key])
		}
	}
	for _, key := range []string{"cpu", "memory", "network"} {
		if _, ok := state[key]; !ok {
			t.Errorf("켜 둔 %q 가 없다", key)
		}
	}
}

// --- AC-05: targets 는 표본에서 유도 ---

func TestSysMetricsState_targets는_표본에서_유도한다(t *testing.T) {
	// 목록 설정을 비워 둔다(= 전체). 설정을 그대로 돌려주면 빈 목록이 나가므로,
	// 표본에서 유도하는지가 이 테스트의 핵심이다.
	a := newStateAgent(t, map[string]any{"interval": "1s"})
	startAgent(t, a)

	a.emitSample(a.done)
	targets, ok := a.State()["targets"].(map[string]any)
	if !ok {
		t.Fatalf("targets = %T, map 이어야 한다", a.State()["targets"])
	}

	cases := []struct {
		key  string
		want []string
	}{
		// 수집 단계에서 이름순 정렬되므로 순서까지 결정적이다.
		{key: "interfaces", want: []string{"en0", "lo0"}},
		{key: "devices", want: []string{"disk0", "disk1"}},
		{key: "mountpoints", want: []string{"/", "/data"}},
	}
	for _, tc := range cases {
		got, ok := targets[tc.key].([]string)
		if !ok {
			t.Errorf("targets[%q] = %T, []string 이어야 한다", tc.key, targets[tc.key])
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("targets[%q] = %v, want %v", tc.key, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("targets[%q] = %v, want %v", tc.key, got, tc.want)
				break
			}
		}
	}
}

// --- AC-06: 동시 호출 ---

func TestSysMetricsState_표본_루프와_동시_호출(t *testing.T) {
	// 주기를 하한(1s)으로 두고 emitSample 을 직접 돌려 표본 쓰기와 State 읽기를
	// 겹친다. `-race` 로 돌 때 의미가 있다.
	a := newStateAgent(t, map[string]any{"interval": "1s"})
	startAgent(t, a)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				a.emitSample(a.done)
			}
		}
	}()

	for i := 0; i < 100; i++ {
		state := a.State()
		if state["status"] == nil {
			t.Error("State() 가 status 없는 맵을 돌려줬다")
			break
		}
	}

	close(stop)
	wg.Wait()
}

// --- 중지 상태 ---

func TestSysMetricsState_중지되면_마지막_표본과_함께_stopped(t *testing.T) {
	a := newStateAgent(t, map[string]any{"interval": "1s"})
	startAgent(t, a)
	a.emitSample(a.done)

	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() 오류 = %v", err)
	}

	state := a.State()
	if got := state["status"]; got != sysMetricsStatusStopped {
		t.Errorf("status = %v, want %q", got, sysMetricsStatusStopped)
	}
	// 마지막으로 본 값을 지우지 않는다 — 지우면 화면이 갑자기 비고, 멈춘 것인지
	// 값이 0 이 된 것인지 구별되지 않는다.
	if _, ok := state["collected_at"]; !ok {
		t.Error("중지 후에도 마지막 표본 시각은 남아야 한다")
	}
}

// --- StatefulAgent 준수 ---

func TestSysMetricsAgent_StatefulAgent_구현(t *testing.T) {
	a := newStateAgent(t, nil)

	// 이 단언이 깨지면 API 응답에서 state 가 조용히 사라진다.
	var _ agent.StatefulAgent = a
}
