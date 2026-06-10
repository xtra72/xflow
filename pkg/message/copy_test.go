package message

import (
	"reflect"
	"testing"
)

// TestCopyMetadata_CopiesStringsAndGroups 는 CopyMetadata 가 string 값과 group 값을
// 모두 dst 로 복사하는지 검증한다 (P2: 메타데이터 전파 시 group 보존).
func TestCopyMetadata_CopiesStringsAndGroups(t *testing.T) {
	src := NewMetadata()
	src.Set("flatKey", "flatVal")
	src.SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})
	src.SetGroup("device", map[string]string{"name": "thermostat"})

	dst := NewMetadata()
	CopyMetadata(dst, src)

	// string 값 복사 검증
	if v, ok := dst.Get("flatKey"); !ok || v != "flatVal" {
		t.Errorf("CopyMetadata: flat string = (%q, %v), 기대값 (\"flatVal\", true)", v, ok)
	}

	// group 값 복사 검증
	agent, ok := dst.GetGroup("agent")
	if !ok {
		t.Fatal("CopyMetadata: agent group 이 복사되지 않았다")
	}
	if !reflect.DeepEqual(agent, map[string]string{"type": "serial", "id": "node-1"}) {
		t.Errorf("CopyMetadata: agent group = %v", agent)
	}

	device, ok := dst.GetGroup("device")
	if !ok || device["name"] != "thermostat" {
		t.Errorf("CopyMetadata: device group = %v, ok=%v", device, ok)
	}
}

// TestCopyMetadata_GroupIsIndependentCopy 는 CopyMetadata 후 src group 을 변경해도
// dst group 에 영향을 주지 않는지(깊은 복사) 검증한다.
func TestCopyMetadata_GroupIsIndependentCopy(t *testing.T) {
	src := NewMetadata()
	src.SetGroup("agent", map[string]string{"type": "serial"})

	dst := NewMetadata()
	CopyMetadata(dst, src)

	// src group 을 덮어쓴다.
	src.SetGroup("agent", map[string]string{"type": "modbus"})

	got, ok := dst.GetGroup("agent")
	if !ok || got["type"] != "serial" {
		t.Errorf("CopyMetadata: dst group 이 src 변경에 영향받았다: %v", got)
	}
}

// TestCopyMetadataGroups_OnlyCopiesGroups 는 CopyMetadataGroups 가 group 만 복사하고
// string 값은 건드리지 않는지 검증한다 (필터링이 이미 string 에 적용된 노드용).
func TestCopyMetadataGroups_OnlyCopiesGroups(t *testing.T) {
	src := NewMetadata()
	src.Set("srcOnlyFlat", "should-not-copy")
	src.SetGroup("agent", map[string]string{"type": "serial"})

	dst := NewMetadata()
	dst.Set("existingFlat", "keep-me")

	CopyMetadataGroups(dst, src)

	// src 의 flat 값은 복사되지 않아야 한다.
	if _, ok := dst.Get("srcOnlyFlat"); ok {
		t.Error("CopyMetadataGroups: src flat string 이 복사되었다 (group-only 위반)")
	}

	// dst 의 기존 flat 값은 유지되어야 한다.
	if v, ok := dst.Get("existingFlat"); !ok || v != "keep-me" {
		t.Errorf("CopyMetadataGroups: dst 기존 flat = (%q, %v)", v, ok)
	}

	// group 은 복사되어야 한다.
	agent, ok := dst.GetGroup("agent")
	if !ok || agent["type"] != "serial" {
		t.Errorf("CopyMetadataGroups: agent group = %v, ok=%v", agent, ok)
	}
}
