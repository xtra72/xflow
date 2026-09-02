package handler

import (
	"net/http"
	"sort"

	psdisk "github.com/shirou/gopsutil/v4/disk"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// 호스트가 실제로 가진 관측 대상 목록.
//
// sysmetrics 에이전트 설정에서 마운트·디스크 장치·네트워크 인터페이스를 손으로
// 적는 대신 골라 쓰게 하려면, 지금 이 호스트에 무엇이 있는지 알아야 한다.
// 값이 아니라 "이름 목록"만 돌려주는 가벼운 조회다.

// SysResourcesResponse 는 선택 가능한 관측 대상 목록이다.
//
// 세 목록 모두 이름 사전순이다. 수집 순서는 OS 마다 다르고 호출마다 흔들릴 수 있어,
// 정렬하지 않으면 설정 화면의 체크박스 순서가 새로고침마다 뒤바뀐다.
type SysResourcesResponse struct {
	// Mountpoints 는 물리 파티션의 마운트 지점 목록이다.
	Mountpoints []string `json:"mountpoints"`
	// Devices 는 I/O 통계를 낼 수 있는 디스크 장치 이름 목록이다.
	Devices []string `json:"devices"`
	// Interfaces 는 네트워크 인터페이스 이름 목록이다.
	Interfaces []string `json:"interfaces"`
}

// 수집 함수. 테스트가 호스트 상태에 묶이지 않도록 변수로 둔다
// (netIOCounters 는 monitor_network.go 가 이미 선언한 것을 그대로 쓴다).
var (
	sysDiskPartitions = psdisk.Partitions
	sysDiskIOCounters = psdisk.IOCounters
)

// collectSysResources 는 선택 가능한 관측 대상 목록을 모은다.
//
// 세 축은 서로 독립이므로 하나가 실패해도 나머지는 돌려준다 — 디스크 권한 문제
// 하나로 인터페이스 목록까지 못 고르게 되면 설정 화면이 통째로 막힌다.
func collectSysResources() *SysResourcesResponse {
	resp := &SysResourcesResponse{
		Mountpoints: []string{},
		Devices:     []string{},
		Interfaces:  []string{},
	}

	if parts, err := sysDiskPartitions(false); err == nil {
		for _, p := range parts {
			if p.Mountpoint != "" {
				resp.Mountpoints = append(resp.Mountpoints, p.Mountpoint)
			}
		}
		sort.Strings(resp.Mountpoints)
	}

	if counters, err := sysDiskIOCounters(); err == nil {
		for name := range counters {
			resp.Devices = append(resp.Devices, name)
		}
		sort.Strings(resp.Devices)
	}

	if counters, err := netIOCounters(true); err == nil {
		for _, c := range counters {
			if c.Name != "" {
				resp.Interfaces = append(resp.Interfaces, c.Name)
			}
		}
		sort.Strings(resp.Interfaces)
	}

	return resp
}

// SysResources 는 선택 가능한 관측 대상 목록을 반환한다.
// GET /monitor/sysresources
func (h *MonitorHandler) SysResources(ctx api.Context) error {
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(collectSysResources()))
}
