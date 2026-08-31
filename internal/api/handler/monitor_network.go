package handler

import (
	"net/http"
	"sort"

	psnet "github.com/shirou/gopsutil/v4/net"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// NetworkInterfaceStat 은 네트워크 인터페이스 하나의 누적 카운터이다.
//
// 값은 모두 부팅 이후 누적치다. 초당 전송량 같은 비율은 클라이언트가 두 시점의
// 차이로 계산한다 — 서버가 상태를 들고 있지 않아도 되고, 폴링 주기를 클라이언트가
// 자유롭게 정할 수 있다.
type NetworkInterfaceStat struct {
	// Name 은 인터페이스 이름이다(en0, eth0, lo0 ...). 합산 항목은 "total" 이다.
	Name string `json:"name"`
	// BytesSent 는 송신 누적 바이트 수이다.
	BytesSent uint64 `json:"bytes_sent"`
	// BytesRecv 는 수신 누적 바이트 수이다.
	BytesRecv uint64 `json:"bytes_recv"`
	// PacketsSent 는 송신 누적 패킷 수이다.
	PacketsSent uint64 `json:"packets_sent"`
	// PacketsRecv 는 수신 누적 패킷 수이다.
	PacketsRecv uint64 `json:"packets_recv"`
	// ErrIn 은 수신 오류 누적 수이다.
	ErrIn uint64 `json:"err_in"`
	// ErrOut 은 송신 오류 누적 수이다.
	ErrOut uint64 `json:"err_out"`
	// DropIn 은 수신 드롭 누적 수이다.
	DropIn uint64 `json:"drop_in"`
	// DropOut 은 송신 드롭 누적 수이다.
	DropOut uint64 `json:"drop_out"`
}

// NetworkStatsResponse 는 네트워크 인터페이스 통계 응답이다.
type NetworkStatsResponse struct {
	// Total 은 모든 인터페이스의 합산이다.
	Total NetworkInterfaceStat `json:"total"`
	// Interfaces 는 인터페이스별 통계로, 이름 사전순이다.
	Interfaces []NetworkInterfaceStat `json:"interfaces"`
}

// netIOCounters 는 인터페이스별 카운터 수집 함수이다.
// 테스트가 호스트 상태에 묶이지 않도록 변수로 두어 교체할 수 있게 했다.
var netIOCounters = psnet.IOCounters

// totalInterfaceName 은 합산 항목의 이름이다.
const totalInterfaceName = "total"

// collectNetworkStats 는 인터페이스별 카운터를 모아 합산과 함께 반환한다.
func collectNetworkStats() (*NetworkStatsResponse, error) {
	counters, err := netIOCounters(true)
	if err != nil {
		return nil, err
	}

	resp := &NetworkStatsResponse{
		Total:      NetworkInterfaceStat{Name: totalInterfaceName},
		Interfaces: make([]NetworkInterfaceStat, 0, len(counters)),
	}

	for _, c := range counters {
		stat := NetworkInterfaceStat{
			Name:        c.Name,
			BytesSent:   c.BytesSent,
			BytesRecv:   c.BytesRecv,
			PacketsSent: c.PacketsSent,
			PacketsRecv: c.PacketsRecv,
			ErrIn:       c.Errin,
			ErrOut:      c.Errout,
			DropIn:      c.Dropin,
			DropOut:     c.Dropout,
		}
		resp.Interfaces = append(resp.Interfaces, stat)

		resp.Total.BytesSent += stat.BytesSent
		resp.Total.BytesRecv += stat.BytesRecv
		resp.Total.PacketsSent += stat.PacketsSent
		resp.Total.PacketsRecv += stat.PacketsRecv
		resp.Total.ErrIn += stat.ErrIn
		resp.Total.ErrOut += stat.ErrOut
		resp.Total.DropIn += stat.DropIn
		resp.Total.DropOut += stat.DropOut
	}

	// 이름 순서를 고정한다. 수집 순서는 OS 마다 다르고 호출마다 흔들릴 수 있어,
	// 그대로 두면 클라이언트의 인터페이스 선택 목록이 매 폴링마다 뒤바뀐다.
	sort.Slice(resp.Interfaces, func(i, j int) bool {
		return resp.Interfaces[i].Name < resp.Interfaces[j].Name
	})

	return resp, nil
}

// NetworkStats 는 네트워크 인터페이스별 누적 통계를 반환한다.
// GET /monitor/network
func (h *MonitorHandler) NetworkStats(ctx api.Context) error {
	stats, err := collectNetworkStats()
	if err != nil {
		h.logger.Error("네트워크 통계 조회 실패", "error", err)
		return ctx.JSON(http.StatusInternalServerError,
			dto.NewErrorResponse("NETWORK_STATS_ERROR", "네트워크 통계 조회에 실패했습니다"))
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(stats))
}
