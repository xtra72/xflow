package message

import (
	"reflect"
	"testing"
)

// TestExpandGroupsByID 는 ExpandGroupsByID 가 슬림 그룹을 레지스트리 조회로
// 역-수화하며, 콜백 부재/실패 시 슬림 상태를 유지하고, 원본을 변형하지 않음을 검증한다.
func TestExpandGroupsByID(t *testing.T) {
	agentLookup := func(id string) (map[string]string, bool) {
		if id == "a-1" {
			return map[string]string{"type": "serial", "name": "reader"}, true
		}
		return nil, false
	}
	deviceLookup := func(id string) (map[string]string, bool) {
		if id == "d-1" {
			return map[string]string{"type": "HVACR.IDU", "name": "room1"}, true
		}
		return nil, false
	}

	tests := []struct {
		name string
		in   map[string]any
		exp  GroupExpander
		want map[string]any
	}{
		{
			name: "slim groups expanded from registry",
			in: map[string]any{
				"agent":  map[string]string{"id": "a-1"},
				"device": map[string]string{"id": "d-1"},
				"flat":   "v",
			},
			exp: GroupExpander{Agent: agentLookup, Device: deviceLookup},
			want: map[string]any{
				"agent":  map[string]string{"id": "a-1", "type": "serial", "name": "reader"},
				"device": map[string]string{"id": "d-1", "type": "HVACR.IDU", "name": "room1"},
				"flat":   "v",
			},
		},
		{
			name: "unknown id keeps slim group",
			in: map[string]any{
				"agent": map[string]string{"id": "unknown"},
			},
			exp: GroupExpander{Agent: agentLookup},
			want: map[string]any{
				"agent": map[string]string{"id": "unknown"},
			},
		},
		{
			name: "nil callbacks keep groups untouched",
			in: map[string]any{
				"agent":  map[string]string{"id": "a-1"},
				"device": map[string]string{"id": "d-1"},
			},
			exp: GroupExpander{},
			want: map[string]any{
				"agent":  map[string]string{"id": "a-1"},
				"device": map[string]string{"id": "d-1"},
			},
		},
		{
			name: "group without id is not expanded",
			in: map[string]any{
				"agent": map[string]string{"type": "x"},
			},
			exp: GroupExpander{Agent: agentLookup},
			want: map[string]any{
				"agent": map[string]string{"type": "x"},
			},
		},
		{
			name: "existing id is preserved even if lookup returns id",
			in: map[string]any{
				"agent": map[string]string{"id": "a-1"},
			},
			exp: GroupExpander{Agent: func(id string) (map[string]string, bool) {
				return map[string]string{"id": "WRONG", "type": "serial"}, true
			}},
			want: map[string]any{
				"agent": map[string]string{"id": "a-1", "type": "serial"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := deepCopyRaw(tt.in)

			got := ExpandGroupsByID(tt.in, tt.exp)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandGroupsByID() = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.in, snapshot) {
				t.Errorf("입력이 변형되었다: %#v, 기대 %#v", tt.in, snapshot)
			}
		})
	}
}

// TestExpandGroupsByID_Nil 는 nil 입력에 nil 을 반환함을 검증한다.
func TestExpandGroupsByID_Nil(t *testing.T) {
	if got := ExpandGroupsByID(nil, GroupExpander{}); got != nil {
		t.Errorf("ExpandGroupsByID(nil) = %#v, want nil", got)
	}
}

// TestExpandRoundTripWithSlim 는 id-only 그룹을 ExpandGroupsByID 로 레지스트리 조회하여
// type/name 을 복원함을 검증한다. (egress 슬림화 제거 후 SlimGroupsToID 는 축소하지
// 않으므로, id-only 입력을 직접 구성해 expand 경로만 독립적으로 검증한다.)
func TestExpandRoundTripWithSlim(t *testing.T) {
	full := map[string]any{
		"agent":  map[string]string{"id": "a-1", "type": "serial", "name": "reader"},
		"device": map[string]string{"id": "d-1", "type": "HVACR.IDU", "name": "room1"},
	}
	// id-only 그룹(과거 슬림 산출물과 동일 형태)을 직접 구성한다.
	slim := map[string]any{
		"agent":  map[string]string{"id": "a-1"},
		"device": map[string]string{"id": "d-1"},
	}

	expanded := ExpandGroupsByID(slim, GroupExpander{
		Agent: func(id string) (map[string]string, bool) {
			return map[string]string{"type": "serial", "name": "reader"}, true
		},
		Device: func(id string) (map[string]string, bool) {
			return map[string]string{"type": "HVACR.IDU", "name": "room1"}, true
		},
	})
	if !reflect.DeepEqual(expanded, full) {
		t.Errorf("expand(slim(full)) = %#v, want %#v", expanded, full)
	}
}
