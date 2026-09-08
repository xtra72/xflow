package main

import (
	"strings"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
)

// pinnedRestore 는 부팅 시 고정 설치 디바이스를 되살리기 위해 필요한 것 두 가지다.
type pinnedRestore struct {
	// 에이전트 로스터에 미리 등록할 디바이스.
	Entries []agent.DeviceEntry
	// 소유 정보가 비어 있던 기록에 채워 넣을 메타데이터(UUID → 메타데이터).
	// 저장해 두면 다음 부팅부터는 참조만으로 복원된다.
	Backfill map[string]device.DeviceMetadata
}

// ownedLocalIDs 는 device_ids 저장소의 (에이전트참조:로컬ID → UUID) 매핑을 뒤집어,
// **이 에이전트가 소유한** UUID → 로컬 ID 매핑을 만든다.
//
// 키를 콜론으로 쪼개지 않고 `<참조> + ":"` 접두사로 자르는 이유는, 로컬 ID 에도
// 에이전트 참조에도 콜론이 들어갈 수 있어 어느 쪽 콜론인지 문자열만 봐서는 가릴 수
// 없기 때문이다. 참조를 알고 있으니 접두사 매칭이 유일하게 모호하지 않다.
//
// 참조를 여러 개 받는 이유: 저장소 키는 정본 **에이전트 ID** 로 정규화되어 발급되지만
// (agent.ResolveDeviceID), 리졸버가 없던 시점에 쓰인 파일에는 **이름** 이 그대로 들어
// 있다. 한쪽만 보면 그 시절 기록을 통째로 놓친다.
func ownedLocalIDs(agentRefs []string, deviceIDs map[string]string) map[string]string {
	out := make(map[string]string, len(deviceIDs))
	for _, ref := range agentRefs {
		if ref == "" {
			continue
		}
		prefix := ref + ":"
		for key, uid := range deviceIDs {
			if uid == "" || !strings.HasPrefix(key, prefix) {
				continue
			}
			if localID := key[len(prefix):]; localID != "" {
				// 먼저 찾은 참조가 이긴다 — 호출부가 확실한 것부터 넘긴다.
				if _, seen := out[uid]; !seen {
					out[uid] = localID
				}
			}
		}
	}
	return out
}

// collectPinnedDevices 는 이 에이전트에 다시 등록할 고정 설치 디바이스를 고른다.
//
// 로컬 ID 를 찾는 순서는 확실한 것부터다:
//
//  1. 메타데이터의 소유 정보 — 사용자가 메타데이터를 고칠 때 기록된 정본.
//  2. device_ids 역인덱스 — **한 번이라도 발견된 적 있는** 디바이스면 남아 있다.
//     소유 정보 필드가 생기기 전에 고정해 둔 기록이 여기서 살아난다.
//  3. 지금 살아 있는 디바이스 열거 — 위 둘이 모두 비었을 때의 마지막 수단.
//
// 2번이 없으면 옛 기록은 "이미 발견된 디바이스만 복원"하는 순환에 갇힌다. 업링크가
// 와야 발견되는 LoRaWAN 센서에서는 그 순환이 곧 "첫 데이터가 오기 전에는 보이지
// 않음"이다 — 고정 설치가 약속한 것과 정반대다.
func collectPinnedDevices(
	agentRefs []string,
	allMeta map[string]device.DeviceMetadata,
	deviceIDs map[string]string,
	liveLocalIDs map[string]string,
) pinnedRestore {
	indexed := ownedLocalIDs(agentRefs, deviceIDs)
	// 메타데이터에 적힌 소유 에이전트는 표시 이름이지만, 저장소 키는 ID 일 수 있다.
	// 어느 표기로 적혔든 이 에이전트를 가리키면 내 것이다.
	owned := make(map[string]bool, len(agentRefs))
	for _, ref := range agentRefs {
		if ref != "" {
			owned[ref] = true
		}
	}
	agentName := ""
	if len(agentRefs) > 0 {
		agentName = agentRefs[0]
	}

	out := pinnedRestore{Backfill: make(map[string]device.DeviceMetadata)}
	for uid, meta := range allMeta {
		if meta.Pinned == nil || !*meta.Pinned {
			continue
		}
		// 다른 에이전트 소유임이 기록돼 있으면 건너뛴다.
		if meta.AgentName != "" && !owned[meta.AgentName] {
			continue
		}

		localID := meta.LocalID
		backfill := false
		if localID == "" {
			localID = indexed[uid]
			backfill = localID != ""
		}
		if localID == "" {
			localID = liveLocalIDs[uid]
			backfill = localID != ""
		}
		if localID == "" {
			continue
		}

		out.Entries = append(out.Entries, agent.DeviceEntry{
			Address: localID,
			Name:    "", // 라벨은 에이전트가 정한다(업링크가 알려 줄 때까지 로컬 ID).
		})
		if backfill || meta.AgentName == "" {
			// 소유 정보를 채워 되쓴다 — 사용자가 고정을 다시 켜지 않아도 다음
			// 부팅부터는 1번 경로로 복원된다.
			patched := meta
			patched.AgentName = agentName
			patched.LocalID = localID
			out.Backfill[uid] = patched
		}
	}
	return out
}

// deviceLocalIDOf 는 에이전트 내부 식별자를 얻는다.
//
// LocalID() 는 선택적 확장 인터페이스다. 없으면 Name() 으로 떨어진다 — 대부분의
// 어댑터에서 둘이 같은 값이다.
func deviceLocalIDOf(dev device.Device) string {
	type localIDProvider interface{ LocalID() string }
	if lp, ok := dev.(localIDProvider); ok {
		if id := lp.LocalID(); id != "" {
			return id
		}
	}
	return dev.Name()
}
