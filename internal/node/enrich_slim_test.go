package node

import (
	"context"
	"testing"

	"github.com/xtra/xflow/pkg/message"
)

// enrich_slim_test.go (message-slim-metadata 버그 수정) 는 enrich 노드가 to_metadata 로
// 재수화한 그룹이 egress 슬림 경계(slimEgressMetadata)를 통과해도 type/name 을 유지함을
// 검증한다(버그: 슬림이 enrich 를 무조건 되돌려 지웠다).

// TestEnrichThenSlim_KeepsEnrichedGroup 는 버그 재현/수정 검증이다:
// device 를 enrich(to_metadata) 한 뒤 debug/output egress 경로(slimEgressMetadata)를
// 통과시키면 device 의 type/name 이 살아있어야 한다.
//
// 수정 전: slimEgressMetadata → SlimGroupsToID 가 device 를 id-only 로 되돌림 → 실패.
func TestEnrichThenSlim_KeepsEnrichedGroup(t *testing.T) {
	// device={id:d-1} 슬림 메시지에 enrich(to_metadata) 적용 → device={type,id,name}.
	n := newEnrichNode(t, map[string]any{"source": "device", "to_metadata": true})
	msg := deviceGroupMsg("d-1")
	out, err := n.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("enrich Process 에러: %v", err)
	}
	enriched := out[0]

	// enrich 직후 내부 메시지에는 full 그룹 + 보존 마커가 있어야 한다.
	dg, _ := enriched.Metadata().GetGroup("device")
	if dg["type"] != "HVACR.IDU" || dg["name"] != "room1" {
		t.Fatalf("enrich 후 device 그룹이 full 이어야 한다: %#v", dg)
	}
	if keep, _ := enriched.Metadata().Get(message.MetaKeySlimKeep); keep == "" {
		t.Fatalf("enrich to_metadata 는 _slimKeep 마커를 설정해야 한다")
	}

	// egress 슬림(debug/output 노드 경로) — 보존 그룹은 full 유지, 마커는 제거.
	md := slimEgressMetadata(enriched)
	device, ok := md["device"].(map[string]string)
	if !ok {
		t.Fatalf("egress device 그룹이 없다: %#v", md["device"])
	}
	if device["type"] != "HVACR.IDU" || device["name"] != "room1" || device["id"] != "d-1" {
		t.Errorf("egress 후에도 enrich 된 device 는 full 이어야 한다: %#v", device)
	}
	if _, leaked := md[message.MetaKeySlimKeep]; leaked {
		t.Errorf("_slimKeep 마커가 egress 에 누출되었다: %#v", md)
	}
}

// TestEnrichThenTapEgress_KeepsEnrichedGroup 는 WS tap egress(EgressMetadata, expander nil)
// 경로에서도 enrich 된 그룹이 보존됨을 검증한다.
func TestEnrichThenTapEgress_KeepsEnrichedGroup(t *testing.T) {
	n := newEnrichNode(t, map[string]any{"source": "agent", "to_metadata": true})
	out, err := n.Process(context.Background(), agentGroupMsg("a-1"))
	if err != nil {
		t.Fatalf("enrich Process 에러: %v", err)
	}
	// tap egress 는 SlimGroupsToID 를 쓰는 EgressMetadata(expander=nil)와 동등하다.
	md := message.SlimGroupsToID(out[0].Metadata().Raw())
	agent, ok := md["agent"].(map[string]string)
	if !ok {
		t.Fatalf("agent 그룹이 없다: %#v", md["agent"])
	}
	if agent["type"] != "serial" || agent["name"] != "reader" {
		t.Errorf("tap egress 에서 enrich 된 agent 는 full 이어야 한다: %#v", agent)
	}
}

// TestEnrichPayloadOnly_DoesNotPreserve 는 to_payload 만(to_metadata 없이) 사용하는
// enrich 는 _slimKeep 마커를 설정하지 않으며, (원래 그룹이 있었다면) egress 에서 여전히
// 슬림됨을 검증한다.
func TestEnrichPayloadOnly_DoesNotPreserve(t *testing.T) {
	n := newEnrichNode(t, map[string]any{"source": "device", "to_payload": "device_info"})
	// device={id:d-1} 그룹을 가진 메시지에 payload-only enrich.
	out, err := n.Process(context.Background(), deviceGroupMsg("d-1"))
	if err != nil {
		t.Fatalf("enrich Process 에러: %v", err)
	}
	enriched := out[0]

	// 마커가 설정되지 않아야 한다.
	if keep, ok := enriched.Metadata().Get(message.MetaKeySlimKeep); ok && keep != "" {
		t.Errorf("payload-only enrich 는 _slimKeep 를 설정하면 안 된다: %q", keep)
	}

	// payload 에는 {type,id,name} 이 기록되지만, metadata device 그룹은 egress 에서 슬림.
	md := slimEgressMetadata(enriched)
	device, ok := md["device"].(map[string]string)
	if !ok {
		t.Fatalf("device 그룹이 없다: %#v", md["device"])
	}
	if _, hasType := device["type"]; hasType {
		t.Errorf("payload-only enrich 의 device 그룹은 egress 에서 여전히 슬림되어야 한다: %#v", device)
	}
}

// TestEnrichLookupFails_ButPreservesExistingGroup 은 버그 재현/수정 검증이다:
// enrich(to_metadata) 를 적용하되 lookup 이 실패(not-found)한 경우, 메시지가 원래
// 가지고 있던 device 그룹의 type/name 을 egress 슬림 후에도 보존해야 한다.
//
// 수정 전: lookup 실패 → marker 미설정 → slimEgressMetadata 에서 type/name 제거 → 실패.
func TestEnrichLookupFails_ButPreservesExistingGroup(t *testing.T) {
	// 내부 메시지에 full device={type,id,name} 를 갖고 시작 (예: 에미터의 mergeDeviceGroup 에서).
	msg := message.New(message.WithID("m1"))
	msg.Metadata().SetGroup("device", map[string]string{
		"type": "HVACR.IDU",
		"id":   "unknown-device-xyz", // 레지스트리에 없는 id
		"name": "internal-room",
	})

	// enrich(device, to_metadata) 적용 — lookup 은 실패할 것.
	n := newEnrichNode(t, map[string]any{"source": "device", "to_metadata": true})
	out, err := n.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("enrich Process 에러: %v", err)
	}
	enriched := out[0]

	// lookup 실패했더라도 to_metadata 가 활성이면 marker 가 설정되어야 한다.
	keep, _ := enriched.Metadata().Get(message.MetaKeySlimKeep)
	if keep != "device" {
		t.Fatalf("lookup 실패해도 to_metadata 는 _slimKeep 마커를 설정해야 한다: %q", keep)
	}

	// egress 슬림 후: marker 덕분에 device 그룹은 full 로 유지되어야 한다.
	md := slimEgressMetadata(enriched)
	device, ok := md["device"].(map[string]string)
	if !ok {
		t.Fatalf("device 그룹이 없다: %#v", md["device"])
	}
	if device["type"] != "HVACR.IDU" || device["name"] != "internal-room" {
		t.Errorf("lookup 실패해도 egress 후 type/name 은 보존되어야 한다: %#v", device)
	}
	if device["id"] != "unknown-device-xyz" {
		t.Errorf("id 는 항상 보존: %#v", device)
	}
}

// TestNonEnriched_StillSlimmed 는 enrich 를 거치지 않은 메시지는 종전처럼 슬림됨을 검증한다.
func TestNonEnriched_StillSlimmed(t *testing.T) {
	msg := message.New(message.WithID("m1"))
	msg.Metadata().SetGroup("device", map[string]string{"type": "HVACR.IDU", "id": "d-1", "name": "room1"})
	md := slimEgressMetadata(msg)
	device := md["device"].(map[string]string)
	if _, hasType := device["type"]; hasType {
		t.Errorf("비-enrich 메시지의 device 는 id-only 로 슬림되어야 한다: %#v", device)
	}
	if device["id"] != "d-1" {
		t.Errorf("id 는 보존: %#v", device)
	}
}
