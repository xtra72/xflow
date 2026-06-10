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

// TestInfluxDBWriteNode_DefaultTags_FlattensGroups 는 tag_mappings 미지정(기본) 시
// nested group metadata 가 "{group}.{field}" 평면 태그로 펼쳐지는지 검증한다
// (P2 Class C: influxdb_write.go — Flux 태그는 평면 string 이므로 flatten).
func TestInfluxDBWriteNode_DefaultTags_FlattensGroups(t *testing.T) {
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
	// group 은 "{group}.{field}" 태그로 flatten
	assert.Equal(t, "serial", captured.Tags["agent.type"], "agent group 의 type 이 agent.type 태그로 flatten 되어야 한다")
	assert.Equal(t, "node-1", captured.Tags["agent.id"], "agent group 의 id 가 agent.id 태그로 flatten 되어야 한다")
}
