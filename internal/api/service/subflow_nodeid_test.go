package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseSubflowNodeID 는 네임스페이스 노드 ID 의 한 겹 분해 규약을 검증한다.
// (WEB 팀 공유 규약: subflow_<flowNodeID>_<originalID>)
func TestParseSubflowNodeID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantFlow string
		wantOrig string
		wantOK   bool
	}{
		{
			name:     "단일 네임스페이스",
			id:       "subflow_F_inner",
			wantFlow: "F",
			wantOrig: "inner",
			wantOK:   true,
		},
		{
			name:     "중첩 네임스페이스는 한 겹만 제거",
			id:       "subflow_F_subflow_G_inner",
			wantFlow: "F",
			wantOrig: "subflow_G_inner",
			wantOK:   true,
		},
		{
			name:     "네임스페이스 아님",
			id:       "inner",
			wantFlow: "",
			wantOrig: "",
			wantOK:   false,
		},
		{
			name:     "두 번째 구분자 없음",
			id:       "subflow_inner",
			wantFlow: "",
			wantOrig: "",
			wantOK:   false,
		},
		{
			name:     "flowNodeID 비어 있음",
			id:       "subflow__inner",
			wantFlow: "",
			wantOrig: "",
			wantOK:   false,
		},
		{
			name:     "originalID 비어 있음",
			id:       "subflow_F_",
			wantFlow: "",
			wantOrig: "",
			wantOK:   false,
		},
		{
			name:     "빈 문자열",
			id:       "",
			wantFlow: "",
			wantOrig: "",
			wantOK:   false,
		},
		{
			name:     "UUID 형식 flowNodeID/originalID",
			id:       "subflow_3f2a1b4c-0000-4000-8000-000000000001_a1b2c3d4-0000-4000-8000-000000000002",
			wantFlow: "3f2a1b4c-0000-4000-8000-000000000001",
			wantOrig: "a1b2c3d4-0000-4000-8000-000000000002",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFlow, gotOrig, gotOK := ParseSubflowNodeID(tt.id)
			assert.Equal(t, tt.wantOK, gotOK, "ok")
			assert.Equal(t, tt.wantFlow, gotFlow, "flowNodeID")
			assert.Equal(t, tt.wantOrig, gotOrig, "originalID")
		})
	}
}

// TestSubflowNodeIDPrefix 는 접두사 생성기가 규약 문자열을 정확히 만들고
// ParseSubflowNodeID 의 역연산임을 검증한다.
func TestSubflowNodeIDPrefix(t *testing.T) {
	assert.Equal(t, "subflow_F_", SubflowNodeIDPrefix("F"))
	assert.Equal(t, "subflow_abc-123_", SubflowNodeIDPrefix("abc-123"))

	// 라운드트립: prefix + originalID → ParseSubflowNodeID 가 원복.
	id := SubflowNodeIDPrefix("F") + "inner"
	gotFlow, gotOrig, ok := ParseSubflowNodeID(id)
	assert.True(t, ok)
	assert.Equal(t, "F", gotFlow)
	assert.Equal(t, "inner", gotOrig)
}
