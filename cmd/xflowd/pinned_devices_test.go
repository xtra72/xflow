package main

import (
	"testing"

	"github.com/xtra/xflow/internal/device"
)

func pinned() *bool   { v := true; return &v }
func unpinned() *bool { v := false; return &v }

func TestCollectPinnedDevices_UsesStoredOwnership(t *testing.T) {
	meta := map[string]device.DeviceMetadata{
		"uuid-1": {Pinned: pinned(), AgentName: "cs", LocalID: "24e124725d081175"},
	}
	got := collectPinnedDevices([]string{"cs"}, meta, nil, nil)

	if len(got.Entries) != 1 || got.Entries[0].Address != "24e124725d081175" {
		t.Fatalf("소유 정보가 있으면 살아 있지 않아도 복원해야 한다: %+v", got.Entries)
	}
	if len(got.Backfill) != 0 {
		t.Fatalf("이미 기록된 소유 정보를 다시 쓸 이유가 없다: %+v", got.Backfill)
	}
}

// 소유 정보 필드가 생기기 전에 고정해 둔 기록 — 사용자가 다시 고정하지 않아도
// device_ids 역인덱스로 살아나야 한다. 이것이 "첫 데이터 전에는 보이지 않음"의 원인이었다.
func TestCollectPinnedDevices_RecoversLegacyRecordFromDeviceIDIndex(t *testing.T) {
	meta := map[string]device.DeviceMetadata{"uuid-1": {Pinned: pinned()}}
	deviceIDs := map[string]string{"cs:24e124725d081175": "uuid-1"}

	got := collectPinnedDevices([]string{"cs"}, meta, deviceIDs, nil)

	if len(got.Entries) != 1 || got.Entries[0].Address != "24e124725d081175" {
		t.Fatalf("역인덱스로 복원해야 한다: %+v", got.Entries)
	}
	patched, ok := got.Backfill["uuid-1"]
	if !ok {
		t.Fatal("소유 정보를 채워 되써야 다음 부팅이 참조만으로 끝난다")
	}
	if patched.AgentName != "cs" || patched.LocalID != "24e124725d081175" {
		t.Fatalf("채운 소유 정보가 어긋난다: %+v", patched)
	}
	if patched.Pinned == nil || !*patched.Pinned {
		t.Fatal("되쓰기가 고정 표시를 지우면 안 된다")
	}
}

func TestCollectPinnedDevices_FallsBackToLiveDevices(t *testing.T) {
	meta := map[string]device.DeviceMetadata{"uuid-1": {Pinned: pinned()}}
	live := map[string]string{"uuid-1": "aabbccdd"}

	got := collectPinnedDevices([]string{"cs"}, meta, nil, live)

	if len(got.Entries) != 1 || got.Entries[0].Address != "aabbccdd" {
		t.Fatalf("살아 있는 디바이스로도 복원해야 한다: %+v", got.Entries)
	}
	if _, ok := got.Backfill["uuid-1"]; !ok {
		t.Fatal("이 경로에서도 소유 정보를 채워 둬야 한다")
	}
}

func TestCollectPinnedDevices_SkipsOtherAgents(t *testing.T) {
	meta := map[string]device.DeviceMetadata{
		"uuid-1": {Pinned: pinned(), AgentName: "other", LocalID: "11"},
	}
	// 역인덱스에도 다른 에이전트 것만 있다.
	deviceIDs := map[string]string{"other:11": "uuid-1"}

	got := collectPinnedDevices([]string{"cs"}, meta, deviceIDs, nil)

	if len(got.Entries) != 0 {
		t.Fatalf("다른 에이전트 디바이스를 가져오면 안 된다: %+v", got.Entries)
	}
}

func TestCollectPinnedDevices_SkipsUnpinned(t *testing.T) {
	meta := map[string]device.DeviceMetadata{
		"uuid-1": {Pinned: unpinned(), AgentName: "cs", LocalID: "11"},
		"uuid-2": {AgentName: "cs", LocalID: "22"}, // 미설정
	}
	got := collectPinnedDevices([]string{"cs"}, meta, nil, nil)
	if len(got.Entries) != 0 {
		t.Fatalf("고정하지 않은 디바이스는 대상이 아니다: %+v", got.Entries)
	}
}

func TestCollectPinnedDevices_NoLocalIDAnywhere(t *testing.T) {
	meta := map[string]device.DeviceMetadata{"uuid-1": {Pinned: pinned()}}
	got := collectPinnedDevices([]string{"cs"}, meta, nil, nil)
	if len(got.Entries) != 0 {
		t.Fatalf("주소를 알 수 없으면 지어내지 않는다: %+v", got.Entries)
	}
}

func TestOwnedLocalIDs_PrefixMatchNotColonSplit(t *testing.T) {
	// 에이전트 이름과 로컬 ID 양쪽에 콜론이 있어도 접두사 매칭은 흔들리지 않는다.
	deviceIDs := map[string]string{
		"cs:main:aa:bb:cc": "uuid-1",
		"other:aa:bb:cc":   "uuid-2",
	}
	got := ownedLocalIDs([]string{"cs:main"}, deviceIDs)

	if got["uuid-1"] != "aa:bb:cc" {
		t.Fatalf("접두사 뒤 전부가 로컬 ID 다: %+v", got)
	}
	if _, ok := got["uuid-2"]; ok {
		t.Fatalf("다른 에이전트 항목이 섞였다: %+v", got)
	}
}

func TestOwnedLocalIDs_EmptyAgentName(t *testing.T) {
	if got := ownedLocalIDs(nil, map[string]string{"cs:1": "uuid-1"}); len(got) != 0 {
		t.Fatalf("이름이 없으면 어느 것도 이 에이전트 소유라고 말할 수 없다: %+v", got)
	}
}

// device_ids 키는 정본 에이전트 ID 로 정규화되어 발급된다. 표시 이름만 보고 찾으면
// 지금 쓰이는 파일을 통째로 놓친다 — 이 회귀가 실제 원인이었다.
func TestCollectPinnedDevices_MatchesNormalizedAgentID(t *testing.T) {
	meta := map[string]device.DeviceMetadata{"uuid-1": {Pinned: pinned()}}
	deviceIDs := map[string]string{"agent-uuid-42:24e124725d081175": "uuid-1"}

	got := collectPinnedDevices([]string{"ChirpStack-Client", "agent-uuid-42"}, meta, deviceIDs, nil)

	if len(got.Entries) != 1 || got.Entries[0].Address != "24e124725d081175" {
		t.Fatalf("정본 ID 키로도 복원해야 한다: %+v", got.Entries)
	}
	// 되쓰는 소유 이름은 표시 이름(정본 메타데이터 표기)이어야 한다.
	if got.Backfill["uuid-1"].AgentName != "ChirpStack-Client" {
		t.Fatalf("소유 이름은 표시 이름으로 적어야 한다: %+v", got.Backfill["uuid-1"])
	}
}

// 메타데이터에 ID 표기로 적힌 소유 정보도 이 에이전트 것으로 인정해야 한다.
func TestCollectPinnedDevices_OwnershipRecordedAsID(t *testing.T) {
	meta := map[string]device.DeviceMetadata{
		"uuid-1": {Pinned: pinned(), AgentName: "agent-uuid-42", LocalID: "aabb"},
	}
	got := collectPinnedDevices([]string{"ChirpStack-Client", "agent-uuid-42"}, meta, nil, nil)
	if len(got.Entries) != 1 {
		t.Fatalf("ID 표기 소유 정보를 남의 것으로 보면 안 된다: %+v", got.Entries)
	}
}
