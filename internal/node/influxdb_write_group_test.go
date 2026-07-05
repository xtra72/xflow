package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// TestInfluxDBWriteNode_DefaultTags_SlimsGroupsToID 는 tag_mappings 미지정(기본) 시
// nested group metadata 가 외부 스토리지 egress 정책에 따라 id-only 로 슬림화된 뒤
// "{group}.id" 평면 태그로 펼쳐지는지 검증한다 (message-slim-metadata).
// type/name 은 레지스트리의 정규 데이터이므로 스토리지에 중복 기록하지 않는다.
func TestInfluxDBWriteNode_DefaultTags_SlimsGroupsToID(t *testing.T) {
	var captured influxdbWriteData
	mock := &mockInfluxDBAgent{
		processFunc: func(data []byte) ([]byte, error) {
			return nil, json.Unmarshal(data, &captured)
		},
	}

	def := flow.NodeDef{ID: "iw-grp", Type: "influxdb-write"}
	n, err := NewInfluxDBWriteNode(def)
	require.NoError(t, err)

	// tag_mappings 미지정 → 모든 metadata 를 tags 로 (group flatten 대상).
	require.NoError(t, n.Configure(map[string]any{
		"_influxdb_agent": mock,
		"measurement":     "metrics",
	}))
	require.NoError(t, n.Init(context.Background()))

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"v": 1})))
	msg.Metadata().Set("host", "server-01")
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	// flat 키는 그대로
	assert.Equal(t, "server-01", captured.Tags["host"])
	// group 은 id-only 슬림 후 "{group}.id" 태그로만 flatten
	assert.Equal(t, "node-1", captured.Tags["agent.id"], "agent group 의 id 가 agent.id 태그로 flatten 되어야 한다")
	// type/name 은 스토리지 egress 에서 제거되어 태그로 기록되지 않는다.
	_, hasType := captured.Tags["agent.type"]
	assert.False(t, hasType, "스토리지 egress 슬림 후 agent.type 태그는 기록되지 않아야 한다")
}
