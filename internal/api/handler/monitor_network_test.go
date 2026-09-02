package handler

import (
	"errors"
	"testing"

	psdisk "github.com/shirou/gopsutil/v4/disk"
	psnet "github.com/shirou/gopsutil/v4/net"
)

// stubCounters 는 테스트 동안 netIOCounters 를 교체하고 복구 함수를 돌려준다.
func stubCounters(t *testing.T, fn func(bool) ([]psnet.IOCountersStat, error)) {
	t.Helper()
	prev := netIOCounters
	netIOCounters = fn
	t.Cleanup(func() { netIOCounters = prev })
}

func TestCollectNetworkStats_합산(t *testing.T) {
	stubCounters(t, func(perNic bool) ([]psnet.IOCountersStat, error) {
		if !perNic {
			t.Fatalf("인터페이스별 수집이어야 한다 (perNic=true), got false")
		}
		return []psnet.IOCountersStat{
			{Name: "en0", BytesSent: 100, BytesRecv: 200, PacketsSent: 1, PacketsRecv: 2, Errin: 1, Dropin: 3},
			{Name: "lo0", BytesSent: 10, BytesRecv: 20, PacketsSent: 3, PacketsRecv: 4, Errout: 2, Dropout: 4},
		}, nil
	})

	got, err := collectNetworkStats()
	if err != nil {
		t.Fatalf("collectNetworkStats() 오류 = %v", err)
	}

	if len(got.Interfaces) != 2 {
		t.Fatalf("인터페이스 수 = %d, want 2", len(got.Interfaces))
	}

	want := NetworkInterfaceStat{
		Name: "total", BytesSent: 110, BytesRecv: 220,
		PacketsSent: 4, PacketsRecv: 6,
		ErrIn: 1, ErrOut: 2, DropIn: 3, DropOut: 4,
	}
	if got.Total != want {
		t.Errorf("합산 = %+v, want %+v", got.Total, want)
	}
}

func TestCollectNetworkStats_이름순정렬(t *testing.T) {
	// 수집 순서는 OS 마다 다르므로 이름순으로 고정되어야 한다.
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return []psnet.IOCountersStat{
			{Name: "utun0"}, {Name: "en0"}, {Name: "lo0"},
		}, nil
	})

	got, err := collectNetworkStats()
	if err != nil {
		t.Fatalf("collectNetworkStats() 오류 = %v", err)
	}

	want := []string{"en0", "lo0", "utun0"}
	for i, name := range want {
		if got.Interfaces[i].Name != name {
			t.Errorf("Interfaces[%d].Name = %q, want %q", i, got.Interfaces[i].Name, name)
		}
	}
}

func TestCollectNetworkStats_인터페이스없음(t *testing.T) {
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return nil, nil
	})

	got, err := collectNetworkStats()
	if err != nil {
		t.Fatalf("collectNetworkStats() 오류 = %v", err)
	}
	if len(got.Interfaces) != 0 {
		t.Errorf("인터페이스 수 = %d, want 0", len(got.Interfaces))
	}
	// 빈 목록이어도 합산 항목은 이름을 갖는다 — 클라이언트가 "전체" 선택지를 그대로 쓴다.
	if got.Total.Name != totalInterfaceName {
		t.Errorf("합산 이름 = %q, want %q", got.Total.Name, totalInterfaceName)
	}
}

func TestCollectNetworkStats_수집실패(t *testing.T) {
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return nil, errors.New("수집 실패")
	})

	if _, err := collectNetworkStats(); err == nil {
		t.Fatal("collectNetworkStats() 오류가 없다, want 오류")
	}
}

// --- 관측 대상 목록 ---

// stubSysResources 는 목록 수집 함수를 결정적 값으로 바꾸고 복구를 예약한다.
func stubSysResources(
	t *testing.T,
	parts func(bool) ([]psdisk.PartitionStat, error),
	io func(...string) (map[string]psdisk.IOCountersStat, error),
) {
	t.Helper()
	prevParts, prevIO := sysDiskPartitions, sysDiskIOCounters
	sysDiskPartitions, sysDiskIOCounters = parts, io
	t.Cleanup(func() { sysDiskPartitions, sysDiskIOCounters = prevParts, prevIO })
}

func TestCollectSysResources_이름순정렬(t *testing.T) {
	// 수집 순서는 OS 마다 다르다. 정렬이 없으면 설정 화면의 체크박스 순서가
	// 새로고침마다 뒤바뀐다.
	stubSysResources(t,
		func(bool) ([]psdisk.PartitionStat, error) {
			return []psdisk.PartitionStat{{Mountpoint: "/data"}, {Mountpoint: "/"}}, nil
		},
		func(...string) (map[string]psdisk.IOCountersStat, error) {
			return map[string]psdisk.IOCountersStat{"disk1": {}, "disk0": {}}, nil
		},
	)
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return []psnet.IOCountersStat{{Name: "lo0"}, {Name: "en0"}}, nil
	})

	got := collectSysResources()

	if want := []string{"/", "/data"}; !equalStrings(got.Mountpoints, want) {
		t.Errorf("Mountpoints = %v, want %v", got.Mountpoints, want)
	}
	if want := []string{"disk0", "disk1"}; !equalStrings(got.Devices, want) {
		t.Errorf("Devices = %v, want %v", got.Devices, want)
	}
	if want := []string{"en0", "lo0"}; !equalStrings(got.Interfaces, want) {
		t.Errorf("Interfaces = %v, want %v", got.Interfaces, want)
	}
}

func TestCollectSysResources_한축이실패해도나머지는돌려준다(t *testing.T) {
	// 디스크 권한 문제 하나로 인터페이스 목록까지 못 고르면 설정 화면이 통째로 막힌다.
	stubSysResources(t,
		func(bool) ([]psdisk.PartitionStat, error) { return nil, errors.New("권한 없음") },
		func(...string) (map[string]psdisk.IOCountersStat, error) { return nil, errors.New("실패") },
	)
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return []psnet.IOCountersStat{{Name: "en0"}}, nil
	})

	got := collectSysResources()

	if len(got.Mountpoints) != 0 || len(got.Devices) != 0 {
		t.Errorf("실패한 축이 비어 있지 않다: %+v", got)
	}
	if want := []string{"en0"}; !equalStrings(got.Interfaces, want) {
		t.Errorf("Interfaces = %v, want %v", got.Interfaces, want)
	}
}

func TestCollectSysResources_빈이름은버린다(t *testing.T) {
	stubSysResources(t,
		func(bool) ([]psdisk.PartitionStat, error) {
			return []psdisk.PartitionStat{{Mountpoint: ""}, {Mountpoint: "/"}}, nil
		},
		func(...string) (map[string]psdisk.IOCountersStat, error) { return nil, nil },
	)
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) {
		return []psnet.IOCountersStat{{Name: ""}, {Name: "en0"}}, nil
	})

	got := collectSysResources()

	if want := []string{"/"}; !equalStrings(got.Mountpoints, want) {
		t.Errorf("Mountpoints = %v, want %v", got.Mountpoints, want)
	}
	if want := []string{"en0"}; !equalStrings(got.Interfaces, want) {
		t.Errorf("Interfaces = %v, want %v", got.Interfaces, want)
	}
}

func TestCollectSysResources_빈목록도nil이아니다(t *testing.T) {
	// nil 이면 JSON 이 null 로 나가 프런트가 배열 메서드를 부르다 터진다.
	stubSysResources(t,
		func(bool) ([]psdisk.PartitionStat, error) { return nil, errors.New("x") },
		func(...string) (map[string]psdisk.IOCountersStat, error) { return nil, errors.New("x") },
	)
	stubCounters(t, func(bool) ([]psnet.IOCountersStat, error) { return nil, errors.New("x") })

	got := collectSysResources()

	if got.Mountpoints == nil || got.Devices == nil || got.Interfaces == nil {
		t.Errorf("빈 목록이 nil 이다: %+v", got)
	}
}

// equalStrings 는 문자열 슬라이스 비교 헬퍼이다.
func equalStrings(a, b []string) bool {
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
