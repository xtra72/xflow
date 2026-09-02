package system

import (
	"fmt"
	"sort"
	"time"

	pscpu "github.com/shirou/gopsutil/v4/cpu"
	psdisk "github.com/shirou/gopsutil/v4/disk"
	psmem "github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
)

// 시스템 리소스 표본 수집 (순수 로직 + 주입 가능한 수집 함수).
//
// 수집 함수를 패키지 변수로 두어 테스트가 호스트 상태에 묶이지 않게 했다. 에이전트
// 자체는 이 파일의 함수만 호출하며, gopsutil 호출은 전부 여기에 모여 있다.

// SysMetricsSample 은 한 시점의 시스템 리소스 표본이다.
//
// 값은 두 갈래다:
//   - 비율/용량(cpu·memory·storage): 그 시점의 상태값. 그대로 쓰면 된다.
//   - 누적 카운터(network·diskIO): 부팅 이후 누적. 초당 증가량은 소비자가 두 표본의
//     차이로 계산한다 — 에이전트가 상태를 들고 있지 않아도 되고, 표본 주기를
//     소비자가 자유롭게 정할 수 있다.
type SysMetricsSample struct {
	// Timestamp 는 표본 시각(epoch ms)이다. 프로젝트 전역 규약을 따른다.
	Timestamp int64 `json:"timestamp"`
	// CPU 는 CPU 사용률(%)이다. 수집하지 않으면 nil.
	CPU *CPUSample `json:"cpu,omitempty"`
	// Memory 는 호스트 메모리 사용량이다. 수집하지 않으면 nil.
	Memory *MemorySample `json:"memory,omitempty"`
	// Storage 는 마운트별 디스크 사용량이다. 수집하지 않으면 비어 있다.
	Storage []StorageSample `json:"storage,omitempty"`
	// DiskIO 는 장치별 디스크 I/O 누적 카운터이다. 수집하지 않으면 비어 있다.
	DiskIO []DiskIOSample `json:"disk_io,omitempty"`
	// Network 는 인터페이스별 네트워크 누적 카운터이다. 수집하지 않으면 비어 있다.
	Network []NetworkSample `json:"network,omitempty"`
}

// CPUSample 은 CPU 사용률 표본이다.
type CPUSample struct {
	// UsagePercent 는 전체 CPU 사용률(%)이다.
	UsagePercent float64 `json:"usage_percent"`
	// Cores 는 논리 코어 수이다.
	Cores int `json:"cores"`
}

// MemorySample 은 호스트 메모리 표본이다.
//
// Go 런타임의 힙 통계(`/monitor/metrics` 의 memory_usage_percent)와는 다른 값이다.
// 이쪽이 사람이 "메모리 사용률" 이라 부를 때 기대하는 호스트 전체 수치다.
type MemorySample struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// StorageSample 은 마운트 하나의 디스크 사용량이다.
type StorageSample struct {
	Mountpoint   string  `json:"mountpoint"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// DiskIOSample 은 장치 하나의 디스크 I/O 누적 카운터이다.
type DiskIOSample struct {
	Device     string `json:"device"`
	ReadBytes  uint64 `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
	ReadCount  uint64 `json:"read_count"`
	WriteCount uint64 `json:"write_count"`
}

// NetworkSample 은 인터페이스 하나의 네트워크 누적 카운터이다.
type NetworkSample struct {
	Interface   string `json:"interface"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
	ErrIn       uint64 `json:"err_in"`
	ErrOut      uint64 `json:"err_out"`
	DropIn      uint64 `json:"drop_in"`
	DropOut     uint64 `json:"drop_out"`
}

// --- 주입 가능한 수집 함수 (테스트가 교체한다) ---

var (
	// cpuPercent 는 구간 평균 CPU 사용률을 반환한다.
	cpuPercent = pscpu.Percent
	// cpuCounts 는 논리 코어 수를 반환한다.
	cpuCounts = pscpu.Counts
	// memVirtual 은 호스트 메모리 상태를 반환한다.
	memVirtual = psmem.VirtualMemory
	// diskPartitions 는 마운트 목록을 반환한다.
	diskPartitions = psdisk.Partitions
	// diskUsage 는 마운트 하나의 사용량을 반환한다.
	diskUsage = psdisk.Usage
	// diskIOCounters 는 장치별 I/O 누적 카운터를 반환한다.
	diskIOCounters = psdisk.IOCounters
	// sysNetIOCounters 는 인터페이스별 누적 카운터를 반환한다.
	sysNetIOCounters = psnet.IOCounters
)

// collectCPU 는 CPU 사용률을 수집한다.
//
// `interval` 이 0 이면 직전 호출 이후의 평균을 즉시 돌려준다(논블로킹). 첫 호출에서는
// 기준점이 없어 0 이 나올 수 있으므로, 에이전트는 시작 시 한 번 예열한다.
func collectCPU(interval time.Duration) (*CPUSample, error) {
	percents, err := cpuPercent(interval, false)
	if err != nil {
		return nil, fmt.Errorf("CPU 사용률 수집 실패: %w", err)
	}
	if len(percents) == 0 {
		return nil, fmt.Errorf("CPU 사용률 수집 실패: 값이 비어 있음")
	}

	cores, err := cpuCounts(true)
	if err != nil {
		// 코어 수는 부가 정보다 — 실패해도 사용률은 살린다.
		cores = 0
	}

	return &CPUSample{UsagePercent: percents[0], Cores: cores}, nil
}

// collectMemory 는 호스트 메모리 사용량을 수집한다.
func collectMemory() (*MemorySample, error) {
	vm, err := memVirtual()
	if err != nil {
		return nil, fmt.Errorf("메모리 수집 실패: %w", err)
	}
	return &MemorySample{
		TotalBytes:     vm.Total,
		UsedBytes:      vm.Used,
		AvailableBytes: vm.Available,
		UsagePercent:   vm.UsedPercent,
	}, nil
}

// collectStorage 는 마운트별 사용량을 수집한다.
//
// `mounts` 가 비어 있으면 물리 파티션 전체를 훑는다. 개별 마운트 조회 실패는
// 건너뛴다 — 권한이 없거나 사라진 마운트 하나 때문에 표본 전체를 잃지 않기 위함이다.
func collectStorage(mounts []string) ([]StorageSample, error) {
	targets := mounts
	if len(targets) == 0 {
		parts, err := diskPartitions(false)
		if err != nil {
			return nil, fmt.Errorf("파티션 목록 조회 실패: %w", err)
		}
		for _, p := range parts {
			targets = append(targets, p.Mountpoint)
		}
	}

	out := make([]StorageSample, 0, len(targets))
	for _, mp := range targets {
		usage, err := diskUsage(mp)
		if err != nil || usage == nil {
			continue
		}
		out = append(out, StorageSample{
			Mountpoint:   mp,
			TotalBytes:   usage.Total,
			UsedBytes:    usage.Used,
			FreeBytes:    usage.Free,
			UsagePercent: usage.UsedPercent,
		})
	}

	// 수집 순서는 OS 마다 다르고 호출마다 흔들릴 수 있다. 이름순으로 고정해야
	// 소비자의 시리즈 순서가 매 표본마다 뒤바뀌지 않는다.
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out, nil
}

// collectDiskIO 는 장치별 I/O 누적 카운터를 수집한다.
func collectDiskIO(devices []string) ([]DiskIOSample, error) {
	counters, err := diskIOCounters(devices...)
	if err != nil {
		return nil, fmt.Errorf("디스크 I/O 수집 실패: %w", err)
	}

	out := make([]DiskIOSample, 0, len(counters))
	for name, c := range counters {
		out = append(out, DiskIOSample{
			Device:     name,
			ReadBytes:  c.ReadBytes,
			WriteBytes: c.WriteBytes,
			ReadCount:  c.ReadCount,
			WriteCount: c.WriteCount,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Device < out[j].Device })
	return out, nil
}

// collectNetwork 는 인터페이스별 누적 카운터를 수집한다.
//
// `interfaces` 가 비어 있지 않으면 그 이름만 남긴다.
func collectNetwork(interfaces []string) ([]NetworkSample, error) {
	counters, err := sysNetIOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("네트워크 수집 실패: %w", err)
	}

	wanted := make(map[string]bool, len(interfaces))
	for _, name := range interfaces {
		wanted[name] = true
	}

	out := make([]NetworkSample, 0, len(counters))
	for _, c := range counters {
		if len(wanted) > 0 && !wanted[c.Name] {
			continue
		}
		out = append(out, NetworkSample{
			Interface:   c.Name,
			BytesSent:   c.BytesSent,
			BytesRecv:   c.BytesRecv,
			PacketsSent: c.PacketsSent,
			PacketsRecv: c.PacketsRecv,
			ErrIn:       c.Errin,
			ErrOut:      c.Errout,
			DropIn:      c.Dropin,
			DropOut:     c.Dropout,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out, nil
}

// collectSample 은 설정이 켠 지표만 모아 표본 하나를 만든다.
//
// 개별 지표 수집 실패는 표본 전체를 버리지 않는다 — 디스크 권한 문제 하나로 CPU·메모리
// 관측이 함께 끊기면 관측 도구로서 쓸모가 없다. 실패한 지표는 표본에서 빠지고, 사유는
// 반환된 오류 목록으로 호출부가 로깅한다.
func collectSample(cfg SysMetricsConfig, nowMs int64) (SysMetricsSample, []error) {
	sample := SysMetricsSample{Timestamp: nowMs}
	var errs []error

	if cfg.CollectCPU {
		// 인터벌 0 = 직전 호출 이후 평균(논블로킹). 표본 주기가 곧 평균 구간이 된다.
		cpu, err := collectCPU(0)
		if err != nil {
			errs = append(errs, err)
		} else {
			sample.CPU = cpu
		}
	}
	if cfg.CollectMemory {
		mem, err := collectMemory()
		if err != nil {
			errs = append(errs, err)
		} else {
			sample.Memory = mem
		}
	}
	if cfg.CollectStorage {
		storage, err := collectStorage(cfg.Mountpoints)
		if err != nil {
			errs = append(errs, err)
		} else {
			sample.Storage = storage
		}
	}
	if cfg.CollectDiskIO {
		io, err := collectDiskIO(cfg.Devices)
		if err != nil {
			errs = append(errs, err)
		} else {
			sample.DiskIO = io
		}
	}
	if cfg.CollectNetwork {
		net, err := collectNetwork(cfg.Interfaces)
		if err != nil {
			errs = append(errs, err)
		} else {
			sample.Network = net
		}
	}

	return sample, errs
}
