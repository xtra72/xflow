package system

// 표본 → 일괄 레코드 구성.
//
// 표본 하나를 측정값마다 쪼개 내보내면(fan-out) 메시지가 폭증한다 — 마운트 2개 +
// 장치 2개 + 인터페이스 3개면 5초마다 40여 건이다. 대신 표본 하나를 메시지 하나로
// 묶고, 값들을 payload 에 그룹별로 중첩해 담는다.
//
// 중첩 형태:
//
//	cpu      → { usage_percent: 42.5 }                      (인스턴스 축 없음)
//	memory   → { used_bytes: 8000, usage_percent: 50 }      (인스턴스 축 없음)
//	storage  → { "/": {...}, "/data": {...} }               (마운트별)
//	disk_io  → { "disk0": {...} }                           (장치별)
//	network  → { "en0": {...} }                             (인터페이스별)

// SysMetricsBatchMeasurement 은 일괄 레코드의 measurement 이름이다.
//
// 값이 아니라 이름이므로 바꾸면 이미 쌓인 시계열과 끊긴다.
const SysMetricsBatchMeasurement = "sysmetrics"

// SysMetricsBatch 는 표본 하나를 담은 일괄 레코드이다.
type SysMetricsBatch struct {
	// Measurement 는 항상 `sysmetrics` 이다. 노드가 metadata.measurement 로 옮긴다.
	Measurement string `json:"measurement"`
	// TimeMs 는 표본 시각(epoch ms)이다.
	TimeMs int64 `json:"time_ms"`
	// Fields 는 그룹별로 중첩된 측정값이다. 노드가 payload 로 그대로 옮긴다.
	//
	// 값 타입이 그룹마다 다르므로(2단계 / 3단계) any 로 둔다. 구체 타입은
	// `MetricGroup` / `InstanceGroup` 이다.
	Fields map[string]any `json:"fields"`
}

// MetricGroup 은 인스턴스 축이 없는 그룹이다 (필드 → 값).
type MetricGroup map[string]float64

// InstanceGroup 은 인스턴스 축이 있는 그룹이다 (인스턴스 → 필드 → 값).
type InstanceGroup map[string]MetricGroup

// 그룹 이름. 소비자가 그룹을 구분하는 기준이므로 값을 바꾸면 이미 쌓인 시계열과 끊긴다.
const (
	groupCPU     = "cpu"
	groupMemory  = "memory"
	groupStorage = "storage"
	groupDiskIO  = "disk_io"
	groupNetwork = "network"
)

// buildBatch 는 표본 하나를 일괄 레코드로 묶는다.
//
// 값이 없는 그룹은 담지 않는다 — 빈 오브젝트를 실어 보내면 하류가 빈 시리즈를 만든다.
// 정적 정보(코어 수 등)도 담지 않는다: 시계열이 아니라 메타데이터이고, 표본마다
// 같은 값을 쌓으면 저장소만 먹는다.
func buildBatch(sample SysMetricsSample) SysMetricsBatch {
	fields := make(map[string]any, 5)

	if c := sample.CPU; c != nil {
		fields[groupCPU] = MetricGroup{"usage_percent": c.UsagePercent}
	}

	if m := sample.Memory; m != nil {
		fields[groupMemory] = MetricGroup{
			"total_bytes":     float64(m.TotalBytes),
			"used_bytes":      float64(m.UsedBytes),
			"available_bytes": float64(m.AvailableBytes),
			"usage_percent":   m.UsagePercent,
		}
	}

	if len(sample.Storage) > 0 {
		group := make(InstanceGroup, len(sample.Storage))
		for _, s := range sample.Storage {
			group[s.Mountpoint] = MetricGroup{
				"total_bytes":   float64(s.TotalBytes),
				"used_bytes":    float64(s.UsedBytes),
				"free_bytes":    float64(s.FreeBytes),
				"usage_percent": s.UsagePercent,
			}
		}
		fields[groupStorage] = group
	}

	if len(sample.DiskIO) > 0 {
		group := make(InstanceGroup, len(sample.DiskIO))
		for _, d := range sample.DiskIO {
			group[d.Device] = MetricGroup{
				"read_bytes":  float64(d.ReadBytes),
				"write_bytes": float64(d.WriteBytes),
				"read_count":  float64(d.ReadCount),
				"write_count": float64(d.WriteCount),
			}
		}
		fields[groupDiskIO] = group
	}

	if len(sample.Network) > 0 {
		group := make(InstanceGroup, len(sample.Network))
		for _, n := range sample.Network {
			group[n.Interface] = MetricGroup{
				"bytes_sent":   float64(n.BytesSent),
				"bytes_recv":   float64(n.BytesRecv),
				"packets_sent": float64(n.PacketsSent),
				"packets_recv": float64(n.PacketsRecv),
				"err_in":       float64(n.ErrIn),
				"err_out":      float64(n.ErrOut),
				"drop_in":      float64(n.DropIn),
				"drop_out":     float64(n.DropOut),
			}
		}
		fields[groupNetwork] = group
	}

	return SysMetricsBatch{
		Measurement: SysMetricsBatchMeasurement,
		TimeMs:      sample.Timestamp,
		Fields:      fields,
	}
}
