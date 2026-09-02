package system

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	psdisk "github.com/shirou/gopsutil/v4/disk"
	psmem "github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"

	"github.com/xtra/xflow/internal/agent"
)

// --- 수집 함수 스텁 ---

// stubCollectors 는 gopsutil 호출을 전부 결정적 값으로 바꾸고 복구를 예약한다.
// 테스트가 호스트 상태에 묶이면 CI 와 로컬에서 다른 결과가 나온다.
func stubCollectors(t *testing.T) {
	t.Helper()

	prevCPU, prevCounts := cpuPercent, cpuCounts
	prevMem := memVirtual
	prevParts, prevUsage := diskPartitions, diskUsage
	prevIO := diskIOCounters
	prevNet := sysNetIOCounters

	cpuPercent = func(time.Duration, bool) ([]float64, error) { return []float64{42.5}, nil }
	cpuCounts = func(bool) (int, error) { return 8, nil }
	memVirtual = func() (*psmem.VirtualMemoryStat, error) {
		return &psmem.VirtualMemoryStat{
			Total: 16_000, Used: 8_000, Available: 8_000, UsedPercent: 50,
		}, nil
	}
	diskPartitions = func(bool) ([]psdisk.PartitionStat, error) {
		return []psdisk.PartitionStat{{Mountpoint: "/"}, {Mountpoint: "/data"}}, nil
	}
	diskUsage = func(path string) (*psdisk.UsageStat, error) {
		return &psdisk.UsageStat{Path: path, Total: 1_000, Used: 250, Free: 750, UsedPercent: 25}, nil
	}
	diskIOCounters = func(...string) (map[string]psdisk.IOCountersStat, error) {
		return map[string]psdisk.IOCountersStat{
			"disk1": {ReadBytes: 10, WriteBytes: 20, ReadCount: 1, WriteCount: 2},
			"disk0": {ReadBytes: 30, WriteBytes: 40, ReadCount: 3, WriteCount: 4},
		}, nil
	}
	sysNetIOCounters = func(bool) ([]psnet.IOCountersStat, error) {
		return []psnet.IOCountersStat{
			{Name: "lo0", BytesRecv: 5},
			{Name: "en0", BytesSent: 100, BytesRecv: 200, PacketsSent: 1, PacketsRecv: 2},
		}, nil
	}

	t.Cleanup(func() {
		cpuPercent, cpuCounts = prevCPU, prevCounts
		memVirtual = prevMem
		diskPartitions, diskUsage = prevParts, prevUsage
		diskIOCounters = prevIO
		sysNetIOCounters = prevNet
	})
}

// --- 설정 파싱 ---

func agentCfg(opts map[string]any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:        "a1",
		Name:      "sys",
		Type:      SysMetricsAgentType,
		Transport: agent.TransportConfig{Options: opts},
	}
}

func TestParseSysMetricsConfig_기본값(t *testing.T) {
	cfg, err := parseSysMetricsConfig(agentCfg(nil))
	if err != nil {
		t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
	}

	if cfg.Interval != defaultSysMetricsInterval {
		t.Errorf("Interval = %s, want %s", cfg.Interval, defaultSysMetricsInterval)
	}
	// 기본은 다섯 지표 모두 켜짐 — 만든 직후 아무것도 안 나오면 설정 화면을
	// 찾아 헤매게 된다.
	if !cfg.CollectCPU || !cfg.CollectMemory || !cfg.CollectStorage ||
		!cfg.CollectDiskIO || !cfg.CollectNetwork {
		t.Errorf("기본 설정에서 모든 지표가 켜져야 한다: %+v", cfg)
	}
}

func TestParseSysMetricsConfig_주기(t *testing.T) {
	tests := []struct {
		name  string
		raw   any
		want  time.Duration
		isErr bool
	}{
		{name: "기간 문자열", raw: "30s", want: 30 * time.Second},
		{name: "숫자는 초로 읽는다", raw: float64(10), want: 10 * time.Second},
		{name: "정수도 초", raw: 15, want: 15 * time.Second},
		{name: "하한 미만은 오류", raw: "500ms", isErr: true},
		{name: "상한 초과는 오류", raw: "2h", isErr: true},
		{name: "형식 오류", raw: "abc", isErr: true},
		{name: "해석 불가 타입", raw: true, isErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseSysMetricsConfig(agentCfg(map[string]any{"interval": tt.raw}))
			if tt.isErr {
				if err == nil {
					t.Fatalf("오류가 없다, want 오류 (raw=%v)", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
			}
			if cfg.Interval != tt.want {
				t.Errorf("Interval = %s, want %s", cfg.Interval, tt.want)
			}
		})
	}
}

func TestParseSysMetricsConfig_지표토글(t *testing.T) {
	cfg, err := parseSysMetricsConfig(agentCfg(map[string]any{
		"collect_storage": false,
		"collect_disk_io": false,
		"collect_network": false,
	}))
	if err != nil {
		t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
	}

	if !cfg.CollectCPU || !cfg.CollectMemory {
		t.Error("끄지 않은 지표는 켜진 채로 남아야 한다")
	}
	if cfg.CollectStorage || cfg.CollectDiskIO || cfg.CollectNetwork {
		t.Error("끈 지표가 켜져 있다")
	}
}

func TestParseSysMetricsConfig_전부끄면오류(t *testing.T) {
	// 아무것도 수집하지 않는 에이전트는 조용히 도는 대신 만들 때 걸러야 한다.
	_, err := parseSysMetricsConfig(agentCfg(map[string]any{
		"collect_cpu": false, "collect_memory": false, "collect_storage": false,
		"collect_disk_io": false, "collect_network": false,
	}))
	if err == nil {
		t.Fatal("오류가 없다, want 오류")
	}
}

func TestParseSysMetricsConfig_쉼표문자열목록(t *testing.T) {
	// 목록 필드가 자유 입력이던 시절에 저장된 값은 문자열로 남아 있다. 이걸 조용히
	// 무시하면 빈 목록 = "전체" 가 되어, 인터페이스를 골라 뒀는데도 모든 인터페이스
	// 데이터가 올라온다(추적하기 어려운 증상).
	cfg, err := parseSysMetricsConfig(agentCfg(map[string]any{
		"interfaces":  "en0, lo0",
		"mountpoints": "/",
	}))
	if err != nil {
		t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
	}

	if want := []string{"en0", "lo0"}; !equalStringSlice(cfg.Interfaces, want) {
		t.Errorf("Interfaces = %v, want %v", cfg.Interfaces, want)
	}
	if want := []string{"/"}; !equalStringSlice(cfg.Mountpoints, want) {
		t.Errorf("Mountpoints = %v, want %v", cfg.Mountpoints, want)
	}
}

func TestParseSysMetricsConfig_쉼표문자열이실제로필터링된다(t *testing.T) {
	stubCollectors(t)

	// 파싱만 보면 놓친다 — 실제 수집까지 내려가 필터가 걸리는지 확인한다.
	cfg, err := parseSysMetricsConfig(agentCfg(map[string]any{
		"interfaces": "en0",
	}))
	if err != nil {
		t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
	}

	sample, _ := collectSample(SysMetricsConfig{CollectNetwork: true, Interfaces: cfg.Interfaces}, 0)

	if len(sample.Network) != 1 || sample.Network[0].Interface != "en0" {
		var got []string
		for _, n := range sample.Network {
			got = append(got, n.Interface)
		}
		t.Errorf("수집된 인터페이스 = %v, want [en0]", got)
	}
}

func TestParseSysMetricsConfig_해석불가목록은거부한다(t *testing.T) {
	// 조용히 nil 로 떨어지면 필터가 통째로 풀린다. 만들 때 걸러야 한다.
	tests := []struct {
		name string
		raw  any
	}{
		{name: "숫자", raw: 42},
		{name: "객체", raw: map[string]any{"a": 1}},
		{name: "배열 안 비문자열", raw: []any{"en0", 7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseSysMetricsConfig(agentCfg(map[string]any{"interfaces": tt.raw})); err == nil {
				t.Fatal("오류가 없다, want 오류")
			}
		})
	}
}

// equalStringSlice 는 문자열 슬라이스 비교 헬퍼이다.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseSysMetricsConfig_대상목록(t *testing.T) {
	cfg, err := parseSysMetricsConfig(agentCfg(map[string]any{
		// JSON 경로는 []any 로, Go 설정 경로는 []string 으로 들어온다.
		"mountpoints": []any{"/", "", "/data"},
		"interfaces":  []string{"en0"},
	}))
	if err != nil {
		t.Fatalf("parseSysMetricsConfig() 오류 = %v", err)
	}

	if got := cfg.Mountpoints; len(got) != 2 || got[0] != "/" || got[1] != "/data" {
		t.Errorf("Mountpoints = %v, want [/ /data]", got)
	}
	if got := cfg.Interfaces; len(got) != 1 || got[0] != "en0" {
		t.Errorf("Interfaces = %v, want [en0]", got)
	}
	// 지정하지 않은 축은 nil 이어야 "전체" 로 읽힌다.
	if cfg.Devices != nil {
		t.Errorf("Devices = %v, want nil", cfg.Devices)
	}
}

// --- 표본 수집 ---

func TestCollectSample_켠지표만담는다(t *testing.T) {
	stubCollectors(t)

	cfg := SysMetricsConfig{CollectCPU: true, CollectMemory: true}
	sample, errs := collectSample(cfg, 1_700_000_000_000)

	if len(errs) != 0 {
		t.Fatalf("오류 = %v, want 없음", errs)
	}
	if sample.CPU == nil || sample.CPU.UsagePercent != 42.5 || sample.CPU.Cores != 8 {
		t.Errorf("CPU = %+v", sample.CPU)
	}
	if sample.Memory == nil || sample.Memory.UsagePercent != 50 {
		t.Errorf("Memory = %+v", sample.Memory)
	}
	if len(sample.Storage) != 0 || len(sample.DiskIO) != 0 || len(sample.Network) != 0 {
		t.Error("끈 지표가 담겼다")
	}
	if sample.Timestamp != 1_700_000_000_000 {
		t.Errorf("Timestamp = %d", sample.Timestamp)
	}
}

func TestCollectSample_이름순정렬(t *testing.T) {
	stubCollectors(t)

	// 수집 순서는 OS 마다 다르다. 정렬이 없으면 소비자의 시리즈 순서가 표본마다 뒤바뀐다.
	sample, _ := collectSample(SysMetricsConfig{
		CollectStorage: true, CollectDiskIO: true, CollectNetwork: true,
	}, 0)

	if len(sample.Storage) != 2 || sample.Storage[0].Mountpoint != "/" {
		t.Errorf("Storage 정렬 = %+v", sample.Storage)
	}
	if len(sample.DiskIO) != 2 || sample.DiskIO[0].Device != "disk0" {
		t.Errorf("DiskIO 정렬 = %+v", sample.DiskIO)
	}
	if len(sample.Network) != 2 || sample.Network[0].Interface != "en0" {
		t.Errorf("Network 정렬 = %+v", sample.Network)
	}
}

func TestCollectSample_지표하나실패해도나머지는담는다(t *testing.T) {
	stubCollectors(t)
	memVirtual = func() (*psmem.VirtualMemoryStat, error) {
		return nil, errors.New("권한 없음")
	}

	sample, errs := collectSample(SysMetricsConfig{CollectCPU: true, CollectMemory: true}, 0)

	if len(errs) != 1 {
		t.Fatalf("오류 개수 = %d, want 1", len(errs))
	}
	// 메모리 하나 때문에 CPU 관측까지 끊기면 관측 도구로서 쓸모가 없다.
	if sample.CPU == nil {
		t.Error("실패하지 않은 지표는 담겨야 한다")
	}
	if sample.Memory != nil {
		t.Error("실패한 지표가 담겼다")
	}
}

func TestCollectStorage_조회실패한마운트는건너뛴다(t *testing.T) {
	stubCollectors(t)
	diskUsage = func(path string) (*psdisk.UsageStat, error) {
		if path == "/data" {
			return nil, errors.New("권한 없음")
		}
		return &psdisk.UsageStat{Path: path, Total: 1, Used: 1}, nil
	}

	got, err := collectStorage(nil)
	if err != nil {
		t.Fatalf("collectStorage() 오류 = %v", err)
	}
	if len(got) != 1 || got[0].Mountpoint != "/" {
		t.Errorf("결과 = %+v, want [/] 하나", got)
	}
}

func TestCollectNetwork_지정한인터페이스만남긴다(t *testing.T) {
	stubCollectors(t)

	got, err := collectNetwork([]string{"en0"})
	if err != nil {
		t.Fatalf("collectNetwork() 오류 = %v", err)
	}
	if len(got) != 1 || got[0].Interface != "en0" {
		t.Errorf("결과 = %+v, want [en0]", got)
	}
}

// --- 에이전트 ---

func TestSysMetricsAgent_Stop후재시작(t *testing.T) {
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(map[string]any{"interval": "1s"}))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("첫 Start() 오류 = %v", err)
	}
	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop() 오류 = %v", err)
	}

	// Stop 이 닫아 둔 done 을 그대로 두고 다시 돌면 루프가 즉시 끝난다.
	if err := a.Start(ctx); err != nil {
		t.Fatalf("재 Start() 오류 = %v", err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	recvCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := a.ReceiveMessage(recvCtx); err != nil {
		t.Fatalf("재시작 후 ReceiveMessage() 오류 = %v", err)
	}
}

func TestSysMetricsAgent_잘못된설정은생성거부(t *testing.T) {
	_, err := NewSysMetricsAgent(agentCfg(map[string]any{"interval": "1ms"}))
	if err == nil {
		t.Fatal("오류가 없다, want 오류")
	}
}

func TestSysMetricsAgent_타입(t *testing.T) {
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(nil))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}
	if got := a.Type(); got != SysMetricsAgentType {
		t.Errorf("Type() = %q, want %q", got, SysMetricsAgentType)
	}
}

func TestRegisterSysMetricsTypes(t *testing.T) {
	mgr := agent.NewManager()

	if err := RegisterSysMetricsTypes(mgr); err != nil {
		t.Fatalf("RegisterSysMetricsTypes() 오류 = %v", err)
	}

	// 등록만 하고 main.go 에서 호출하지 않으면 타입이 살아나지 않는다.
	// 여기서는 등록 자체가 성공하는지만 잠근다.
	if err := RegisterSysMetricsTypes(mgr); err == nil {
		t.Error("중복 등록이 통과했다, want 오류")
	}
}

// --- 일괄 구성 ---

func TestBuildBatch_그룹별로중첩한다(t *testing.T) {
	sample := SysMetricsSample{
		Timestamp: 1_700_000_000_000,
		CPU:       &CPUSample{UsagePercent: 42.5, Cores: 8},
		Memory:    &MemorySample{UsedBytes: 8000, UsagePercent: 50},
	}

	batch := buildBatch(sample)

	if batch.Measurement != SysMetricsBatchMeasurement {
		t.Errorf("Measurement = %q, want %q", batch.Measurement, SysMetricsBatchMeasurement)
	}
	if batch.TimeMs != sample.Timestamp {
		t.Errorf("TimeMs = %d, want %d", batch.TimeMs, sample.Timestamp)
	}

	cpu, ok := batch.Fields[groupCPU].(MetricGroup)
	if !ok {
		t.Fatalf("cpu 그룹 타입 = %T", batch.Fields[groupCPU])
	}
	if cpu["usage_percent"] != 42.5 {
		t.Errorf("cpu.usage_percent = %v, want 42.5", cpu["usage_percent"])
	}

	mem, ok := batch.Fields[groupMemory].(MetricGroup)
	if !ok {
		t.Fatalf("memory 그룹 타입 = %T", batch.Fields[groupMemory])
	}
	if mem["used_bytes"] != 8000 || mem["usage_percent"] != 50 {
		t.Errorf("memory = %+v", mem)
	}
}

func TestBuildBatch_인스턴스축은한단계더중첩한다(t *testing.T) {
	sample := SysMetricsSample{
		Storage: []StorageSample{
			{Mountpoint: "/", UsedBytes: 10},
			{Mountpoint: "/data", UsedBytes: 25},
		},
		DiskIO:  []DiskIOSample{{Device: "disk0", ReadBytes: 30}},
		Network: []NetworkSample{{Interface: "en0", BytesRecv: 200}},
	}

	batch := buildBatch(sample)

	storage, ok := batch.Fields[groupStorage].(InstanceGroup)
	if !ok {
		t.Fatalf("storage 그룹 타입 = %T", batch.Fields[groupStorage])
	}
	if storage["/"]["used_bytes"] != 10 || storage["/data"]["used_bytes"] != 25 {
		t.Errorf("storage = %+v", storage)
	}

	disk, _ := batch.Fields[groupDiskIO].(InstanceGroup)
	if disk["disk0"]["read_bytes"] != 30 {
		t.Errorf("disk_io = %+v", disk)
	}

	net, _ := batch.Fields[groupNetwork].(InstanceGroup)
	if net["en0"]["bytes_recv"] != 200 {
		t.Errorf("network = %+v", net)
	}
}

func TestBuildBatch_인스턴스가여럿이어도덮어쓰지않는다(t *testing.T) {
	sample := SysMetricsSample{
		Network: []NetworkSample{
			{Interface: "en0", BytesRecv: 1},
			{Interface: "lo0", BytesRecv: 2},
		},
	}

	net, ok := buildBatch(sample).Fields[groupNetwork].(InstanceGroup)
	if !ok {
		t.Fatalf("network 그룹 타입 오류")
	}
	if len(net) != 2 {
		t.Fatalf("인터페이스 수 = %d, want 2", len(net))
	}
	if net["en0"]["bytes_recv"] != 1 || net["lo0"]["bytes_recv"] != 2 {
		t.Errorf("인스턴스별 값이 섞였다: %+v", net)
	}
}

func TestBuildBatch_정적정보는담지않는다(t *testing.T) {
	// 코어 수는 시계열이 아니라 메타데이터다. 표본마다 같은 값을 쌓으면 저장소만 먹는다.
	cpu, _ := buildBatch(SysMetricsSample{CPU: &CPUSample{UsagePercent: 1, Cores: 8}}).
		Fields[groupCPU].(MetricGroup)

	if _, ok := cpu["cores"]; ok {
		t.Errorf("정적 정보가 담겼다: %+v", cpu)
	}
}

func TestBuildBatch_값없는그룹은담지않는다(t *testing.T) {
	// 빈 오브젝트를 실어 보내면 하류가 빈 시리즈를 만든다.
	batch := buildBatch(SysMetricsSample{CPU: &CPUSample{UsagePercent: 1}})

	if len(batch.Fields) != 1 {
		t.Fatalf("그룹 수 = %d, want 1: %+v", len(batch.Fields), batch.Fields)
	}
	for _, group := range []string{groupMemory, groupStorage, groupDiskIO, groupNetwork} {
		if _, ok := batch.Fields[group]; ok {
			t.Errorf("빈 그룹이 담겼다: %s", group)
		}
	}
}

func TestBuildBatch_JSON중첩형상(t *testing.T) {
	// 노드와 하류가 참조하는 경로가 이 형상에 달려 있다.
	sample := SysMetricsSample{
		Timestamp: 1,
		CPU:       &CPUSample{UsagePercent: 42.5},
		Network:   []NetworkSample{{Interface: "en0", BytesRecv: 200}},
	}

	data, err := json.Marshal(buildBatch(sample))
	if err != nil {
		t.Fatalf("직렬화 실패: %v", err)
	}

	var got struct {
		Fields struct {
			CPU     map[string]float64            `json:"cpu"`
			Network map[string]map[string]float64 `json:"network"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("역직렬화 실패: %v (json=%s)", err, data)
	}
	if got.Fields.CPU["usage_percent"] != 42.5 {
		t.Errorf("cpu.usage_percent = %v", got.Fields.CPU["usage_percent"])
	}
	if got.Fields.Network["en0"]["bytes_recv"] != 200 {
		t.Errorf("network.en0.bytes_recv = %v", got.Fields.Network["en0"]["bytes_recv"])
	}
}

func TestSysMetricsAgent_표본당메시지하나(t *testing.T) {
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(map[string]any{"interval": "1s"}))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start() 오류 = %v", err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	recvCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := a.ReceiveMessage(recvCtx)
	if err != nil {
		t.Fatalf("ReceiveMessage() 오류 = %v", err)
	}

	var batch struct {
		Measurement string         `json:"measurement"`
		TimeMs      int64          `json:"time_ms"`
		Fields      map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(data, &batch); err != nil {
		t.Fatalf("레코드 역직렬화 실패: %v", err)
	}
	if batch.Measurement != SysMetricsBatchMeasurement {
		t.Errorf("Measurement = %q, want %q", batch.Measurement, SysMetricsBatchMeasurement)
	}
	// 스텁은 다섯 그룹을 모두 채운다 — 표본 하나가 메시지 하나에 담긴다.
	if len(batch.Fields) != 5 {
		t.Errorf("그룹 수 = %d, want 5: %v", len(batch.Fields), batch.Fields)
	}
	if batch.TimeMs < 1_600_000_000_000 {
		t.Errorf("TimeMs = %d, epoch ms 가 아니다", batch.TimeMs)
	}
}

// TestSysMetricsAgent_Configure는_Info에_반영된다 는 설정 변경이 조회 응답까지
// 도달하는지 잠근다.
//
// API 는 `Info().Config` 를 그대로 돌려주므로(`agentToHandlerInfo`), 여기에 새 설정이
// 실리지 않으면 저장은 되었는데 화면은 옛 값을 보여 준다 — 사용자에게는 "인터페이스를
// 골랐는데 적용이 안 됨" 으로 보인다. 수집 필터는 이미 새 값으로 도는 중이라 증상이
// 화면에만 나타나 추적하기 어렵다.
func TestSysMetricsAgent_Configure는_Info에_반영된다(t *testing.T) {
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(map[string]any{"interfaces": []any{"en0"}}))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}

	if err := a.Configure(agentCfg(map[string]any{"interfaces": []any{"en1"}})); err != nil {
		t.Fatalf("Configure() 오류 = %v", err)
	}

	got := a.Info().Config.Transport.Options["interfaces"]
	want := []any{"en1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Info().Config 의 interfaces = %v, want %v", got, want)
	}
}

// TestSysMetricsAgent_Configure는_수집필터에_반영된다 는 새 설정이 다음 표본부터
// 실제로 적용되는지 잠근다. Info 반영(위 테스트)과 별개의 축이다 — 한쪽만 되면
// "화면은 맞는데 데이터가 안 바뀐다" 또는 그 반대가 된다.
func TestSysMetricsAgent_Configure는_수집필터에_반영된다(t *testing.T) {
	stubCollectors(t)

	a, err := NewSysMetricsAgent(agentCfg(map[string]any{"interfaces": []any{"en0"}}))
	if err != nil {
		t.Fatalf("NewSysMetricsAgent() 오류 = %v", err)
	}

	if err := a.Configure(agentCfg(map[string]any{"interfaces": []any{"en1"}})); err != nil {
		t.Fatalf("Configure() 오류 = %v", err)
	}

	a.mu.RLock()
	got := a.cfg.Interfaces
	a.mu.RUnlock()
	if !reflect.DeepEqual(got, []string{"en1"}) {
		t.Errorf("수집 설정의 Interfaces = %v, want [en1]", got)
	}
}
