package message

import (
	"reflect"
	"testing"
)

// TestSlimGroupsToID 는 SlimGroupsToID 가 agent/device 그룹을 id-only 로 축소하고,
// 그 외는 보존하며, 원본을 변형하지 않고, 멱등임을 검증한다(표 기반).
func TestSlimGroupsToID(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "full agent and device groups reduced to id-only",
			in: map[string]any{
				"agent":   map[string]string{"type": "serial", "id": "a-1", "name": "reader"},
				"device":  map[string]string{"type": "HVACR.IDU", "id": "d-1", "name": "room1"},
				"flatKey": "flatVal",
			},
			want: map[string]any{
				"agent":   map[string]string{"id": "a-1"},
				"device":  map[string]string{"id": "d-1"},
				"flatKey": "flatVal",
			},
		},
		{
			name: "agent group missing id is dropped (no empty group)",
			in: map[string]any{
				"agent":  map[string]string{"type": "serial", "name": "reader"},
				"device": map[string]string{"type": "HVACR.IDU", "id": "d-1"},
			},
			want: map[string]any{
				"device": map[string]string{"id": "d-1"},
			},
		},
		{
			name: "agent group empty id is dropped",
			in: map[string]any{
				"agent": map[string]string{"id": "", "type": "serial"},
			},
			want: map[string]any{},
		},
		{
			name: "no agent/device groups passes through unchanged",
			in: map[string]any{
				"k1":     "v1",
				"k2":     "v2",
				"custom": map[string]string{"foo": "bar"},
			},
			want: map[string]any{
				"k1":     "v1",
				"k2":     "v2",
				"custom": map[string]string{"foo": "bar"},
			},
		},
		{
			name: "already slim is idempotent",
			in: map[string]any{
				"agent":  map[string]string{"id": "a-1"},
				"device": map[string]string{"id": "d-1"},
			},
			want: map[string]any{
				"agent":  map[string]string{"id": "a-1"},
				"device": map[string]string{"id": "d-1"},
			},
		},
		{
			name: "agent key as flat string (not a group) is preserved as-is",
			in: map[string]any{
				"agent": "legacy-flat",
			},
			want: map[string]any{
				"agent": "legacy-flat",
			},
		},
		{
			name: "empty map yields empty map",
			in:   map[string]any{},
			want: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 원본 비변형 검증을 위해 입력의 깊은 스냅샷을 만든다.
			snapshot := deepCopyRaw(tt.in)

			got := SlimGroupsToID(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SlimGroupsToID() = %#v, want %#v", got, tt.want)
			}

			// 원본이 변형되지 않았는지 확인.
			if !reflect.DeepEqual(tt.in, snapshot) {
				t.Errorf("입력이 변형되었다: %#v, 기대(원본 보존) %#v", tt.in, snapshot)
			}

			// 멱등성: 결과에 다시 적용해도 동일.
			got2 := SlimGroupsToID(got)
			if !reflect.DeepEqual(got2, tt.want) {
				t.Errorf("멱등성 위반: 2회 적용 = %#v, want %#v", got2, tt.want)
			}
		})
	}
}

// TestSlimGroupsToID_Nil 는 nil 입력에 nil 을 반환함을 검증한다.
func TestSlimGroupsToID_Nil(t *testing.T) {
	if got := SlimGroupsToID(nil); got != nil {
		t.Errorf("SlimGroupsToID(nil) = %#v, want nil", got)
	}
}

// TestSlimGroupsToID_NewMapNotAliased 는 반환 맵과 내부 그룹 맵이 입력과
// 별개의 할당임을 검증한다(반환 맵을 변형해도 입력에 영향 없음).
func TestSlimGroupsToID_NewMapNotAliased(t *testing.T) {
	in := map[string]any{
		"agent": map[string]string{"type": "serial", "id": "a-1", "name": "reader"},
	}
	got := SlimGroupsToID(in)

	// 반환된 그룹 맵을 변형한다.
	if g, ok := got["agent"].(map[string]string); ok {
		g["id"] = "MUTATED"
	}
	got["newKey"] = "x"

	// 입력의 agent 그룹은 영향받지 않아야 한다.
	origGroup, _ := in["agent"].(map[string]string)
	if origGroup["id"] != "a-1" {
		t.Errorf("입력 그룹이 별칭으로 변형되었다: id=%q, want %q", origGroup["id"], "a-1")
	}
	if _, exists := in["newKey"]; exists {
		t.Errorf("입력 맵이 별칭으로 변형되었다: newKey 가 존재한다")
	}
}

// deepCopyRaw 는 테스트 비교용으로 raw 메타데이터 맵을 깊은 복사한다.
func deepCopyRaw(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if g, ok := v.(map[string]string); ok {
			gc := make(map[string]string, len(g))
			for gk, gv := range g {
				gc[gk] = gv
			}
			dst[k] = gc
			continue
		}
		dst[k] = v
	}
	return dst
}
