package system

// 최신 표본 스냅샷과 상태 노출.
//
// 에이전트는 표본을 채널로 방출하기만 했다. 그러면 이 데이터를 화면에서 보려면
// 반드시 플로우를 구성해야 한다 — 지표를 "그냥 보고 싶은" 사용자에게 과한 요구다.
// 여기서는 표본을 뜰 때마다 최신 것을 하나 붙들어 두고, 표준 `agent.StatefulAgent`
// 규약(`detail=full` 응답의 state 필드)으로 노출한다. 대시보드 패널은 이것만 폴링한다.
//
// 저장은 방출과 독립이다. 노드가 소비하지 않아 채널이 가득 차도 스냅샷은 갱신된다
// (`emitSample` 참조). 플로우를 만들지 않은 사용자에게 채널은 늘 가득 찬 상태이므로,
// 저장을 전송 성공에 매달면 패널이 영영 빈 화면이 된다.

import (
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// StatefulAgent 준수를 컴파일 타임에 고정한다. 이 인터페이스를 잃으면 API 응답에서
// state 가 조용히 사라지고 패널이 빈 화면이 된다 — 실패가 눈에 띄지 않는 종류다.
var _ agent.StatefulAgent = (*SysMetricsAgent)(nil)

// 상태 문자열. 패널이 화면 분기에 쓰므로 값을 바꾸면 프론트와 함께 고쳐야 한다.
const (
	// sysMetricsStatusRunning 은 표본이 있고 수집이 돌고 있는 상태이다.
	sysMetricsStatusRunning = "running"
	// sysMetricsStatusStopped 는 에이전트가 Running 이 아닌 상태이다.
	// 표본이 남아 있으면 마지막 값을 함께 돌려준다 — 마지막으로 본 값을 지우는 것보다
	// "멈췄다"고 알리며 남겨 두는 편이 낫다.
	sysMetricsStatusStopped = "stopped"
	// sysMetricsStatusNoSample 은 아직 첫 표본을 뜨지 않은 상태이다.
	// 오류가 아니다 — 시작 직후 첫 틱까지의 정상 구간이다.
	sysMetricsStatusNoSample = "no_sample"
)

// storeSnapshot 은 최신 표본을 갈아 끼운다.
//
// 표본을 뜬 직후, 방출을 시도하기 **전에** 호출해야 한다.
func (a *SysMetricsAgent) storeSnapshot(sample SysMetricsSample) {
	a.mu.Lock()
	a.snapshot = &sample
	// 이력에도 넣는다. 누적 카운터의 증가량 환산은 버퍼가 직전 표본을 들고 있으므로
	// 여기서만 성립한다 — 방출 경로는 채널이 가득 차면 표본을 버리기 때문에 기준점이
	// 끊긴다(스냅샷을 방출보다 먼저 갱신하는 것과 같은 이유).
	a.history.append(sample, a.cfg.History)
	a.mu.Unlock()
}

// State 는 최신 표본과 수집 설정을 map 으로 반환한다 (agent.StatefulAgent 구현).
//
// 반환 키:
//
//	status            running / stopped / no_sample
//	collected_at      표본 시각 (epoch ms)
//	interval_seconds  표본 주기
//	cpu / memory      스칼라 지표 (수집을 껐으면 키 자체가 없다)
//	disk_io / network / storage
//	                  인스턴스별 지표 (같음)
//	cpu_cores         논리 코어 수 (정적 정보)
//	targets           이 표본에 실제로 등장한 대상 이름들
//
// 지표 그룹의 이름과 형태는 `buildBatch` 가 만드는 것을 그대로 쓴다. 화면에서 보는
// 필드 이름과 저장되는 시계열의 필드 이름이 같아야, 패널로 보던 지표를 나중에
// 저장 경로로 옮길 때 이름을 다시 찾지 않는다.
//
// 수집을 끈 지표는 **키를 넣지 않는다**. 0 을 넣으면 "디스크가 비어 있다"와
// "관측하지 않는다"가 화면에서 구별되지 않는다.
func (a *SysMetricsAgent) State() map[string]any {
	a.mu.RLock()
	snapshot := a.snapshot
	interval := a.cfg.Interval
	a.mu.RUnlock()

	running := a.CurrentState() == lifecycle.StateRunning

	if snapshot == nil {
		// 표본 이전. 오류가 아니라 정상 구간이므로 주기 정보만 실어 돌려준다.
		return map[string]any{
			"status":           sysMetricsStatusNoSample,
			"interval_seconds": interval.Seconds(),
		}
	}

	status := sysMetricsStatusStopped
	if running {
		status = sysMetricsStatusRunning
	}

	state := map[string]any{
		"status":           status,
		"collected_at":     snapshot.Timestamp,
		"interval_seconds": interval.Seconds(),
		"targets":          snapshotTargets(*snapshot),
	}

	// 지표 그룹은 배치 구성기가 만든 것을 그대로 옮긴다 (필드 이름 단일 출처).
	for group, values := range buildBatch(*snapshot).Fields {
		state[group] = values
	}

	// 코어 수는 시계열이 아니라 정적 정보라 배치에는 담기지 않는다. 화면에서는
	// 사용률을 읽는 기준이 되므로 여기에만 따로 싣는다.
	if snapshot.CPU != nil {
		state["cpu_cores"] = snapshot.CPU.Cores
	}

	return state
}

// snapshotTargets 는 표본에 실제로 등장한 대상 이름을 모은다.
//
// 설정값이 아니라 표본에서 유도하는 이유: 설정의 목록이 비어 있으면 "전체"를 뜻하고,
// 그 전체가 무엇인지는 표본만이 안다. 설정을 그대로 돌려주면 기본 설정에서 빈 목록이
// 나가고, 패널은 고를 대상이 없다고 판단한다.
//
// 이름 순서는 수집 단계에서 이미 정렬되어 있다 (collectStorage / collectDiskIO /
// collectNetwork). 여기서 다시 정렬하지 않는다.
func snapshotTargets(sample SysMetricsSample) map[string]any {
	mountpoints := make([]string, 0, len(sample.Storage))
	for _, s := range sample.Storage {
		mountpoints = append(mountpoints, s.Mountpoint)
	}

	devices := make([]string, 0, len(sample.DiskIO))
	for _, d := range sample.DiskIO {
		devices = append(devices, d.Device)
	}

	interfaces := make([]string, 0, len(sample.Network))
	for _, n := range sample.Network {
		interfaces = append(interfaces, n.Interface)
	}

	return map[string]any{
		"mountpoints": mountpoints,
		"devices":     devices,
		"interfaces":  interfaces,
	}
}
