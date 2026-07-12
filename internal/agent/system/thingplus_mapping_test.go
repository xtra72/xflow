package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// === nameIDMap 양방향 매핑 테스트 ===

func TestNameIDMap_PutAndLookup(t *testing.T) {
	m := newNameIDMap()

	m.put("Device A", "uuid-a")

	// 정방향 조회 (NAME → device_id)
	id, ok := m.deviceID("Device A")
	require.True(t, ok)
	assert.Equal(t, "uuid-a", id)

	// 역방향 조회 (device_id → NAME)
	name, ok := m.name("uuid-a")
	require.True(t, ok)
	assert.Equal(t, "Device A", name)
}

func TestNameIDMap_LookupMissing(t *testing.T) {
	m := newNameIDMap()

	_, ok := m.deviceID("Unknown")
	assert.False(t, ok)

	_, ok = m.name("no-such-id")
	assert.False(t, ok)
}

func TestNameIDMap_DuplicatePutConsistency(t *testing.T) {
	m := newNameIDMap()

	// 동일 NAME 을 동일 device_id 로 반복 등록해도 일관성이 유지된다.
	m.put("Device A", "uuid-a")
	m.put("Device A", "uuid-a")
	m.put("Device A", "uuid-a")

	id, ok := m.deviceID("Device A")
	require.True(t, ok)
	assert.Equal(t, "uuid-a", id)

	name, ok := m.name("uuid-a")
	require.True(t, ok)
	assert.Equal(t, "Device A", name)
}

func TestNameIDMap_RemapUpdatesReverse(t *testing.T) {
	m := newNameIDMap()

	// NAME 이 새로운 device_id 로 재매핑되면 역방향 맵의 이전 항목이 정리된다.
	m.put("Device A", "uuid-old")
	m.put("Device A", "uuid-new")

	id, ok := m.deviceID("Device A")
	require.True(t, ok)
	assert.Equal(t, "uuid-new", id)

	// 이전 device_id 는 더 이상 역매핑되지 않는다.
	_, ok = m.name("uuid-old")
	assert.False(t, ok)

	name, ok := m.name("uuid-new")
	require.True(t, ok)
	assert.Equal(t, "Device A", name)
}

// === extractDeviceName (JSONPath 추출) 테스트 ===

func TestExtractDeviceName(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]any
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "기본 경로 $.device",
			data: map[string]any{"device": "Device A", "temperature": 42},
			path: "$.device",
			want: "Device A",
		},
		{
			name: "중첩 경로 $.metadata.device_id",
			data: map[string]any{"metadata": map[string]any{"device_id": "Device B"}},
			path: "$.metadata.device_id",
			want: "Device B",
		},
		{
			name:    "경로에 값 없음",
			data:    map[string]any{"other": "x"},
			path:    "$.device",
			wantErr: true,
		},
		{
			name:    "문자열이 아닌 값",
			data:    map[string]any{"device": 123},
			path:    "$.device",
			wantErr: true,
		},
		{
			name:    "잘못된 경로 (접두사 없음)",
			data:    map[string]any{"device": "Device A"},
			path:    "device",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := message.NewPayload(tt.data)
			got, err := extractDeviceName(p, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// === resolveDeviceID (ResolveDeviceID 통합 + fallback) 테스트 ===

func TestResolveDeviceID_NilRepoFallback(t *testing.T) {
	// device_id_repo 가 미설정(nil)이면 NAME 자체를 device_id 로 사용한다 (REQ-map-fallback).
	m := newNameIDMap()

	id := m.resolveDeviceID(context.Background(), "gateway-agent", "Device A")

	assert.Equal(t, "Device A", id, "nil repo 상황에서는 NAME 이 device_id 로 사용되어야 한다")

	// 매핑이 채워졌는지 확인한다.
	got, ok := m.deviceID("Device A")
	require.True(t, ok)
	assert.Equal(t, "Device A", got)

	name, ok := m.name("Device A")
	require.True(t, ok)
	assert.Equal(t, "Device A", name)
}

func TestResolveDeviceID_RepeatedNameConsistency(t *testing.T) {
	// 동일 NAME 이 여러 번 추출되어도 일관된 device_id 로 매핑된다 (AC-07).
	m := newNameIDMap()

	id1 := m.resolveDeviceID(context.Background(), "gateway-agent", "Device A")
	id2 := m.resolveDeviceID(context.Background(), "gateway-agent", "Device A")

	assert.Equal(t, id1, id2)
}
