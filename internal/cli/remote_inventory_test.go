package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// remote command <instance_id>
// =============================================================================

func TestRemoteCommand(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"domain":      "flow",
			"action":      "restart",
			"result":      map[string]any{"ok": true},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "command", testInstanceID,
		"--domain", "flow", "--action", "restart",
		"--args", `{"flow_id":"f1"}`,
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/command", gotPath)
	assert.Equal(t, "flow", gotBody["domain"])
	assert.Equal(t, "restart", gotBody["action"])
	args, ok := gotBody["args"].(map[string]any)
	require.True(t, ok, "args 는 JSON 객체로 전달되어야 합니다")
	assert.Equal(t, "f1", args["flow_id"])
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteCommand_NoArgsOmitsField(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"domain":      "system",
			"action":      "ping",
			"result":      "pong",
		}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "command", testInstanceID,
		"--domain", "system", "--action", "ping",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "system", gotBody["domain"])
	_, hasArgs := gotBody["args"]
	assert.False(t, hasArgs, "--args 미지정 시 args 필드는 생략되어야 합니다")
}

func TestRemoteCommand_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"domain":      "flow",
			"action":      "restart",
			"result":      map[string]any{"ok": true},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"--format", "json", "remote", "command", testInstanceID,
		"--domain", "flow", "--action", "restart",
	})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "flow", parsed["domain"])
	assert.Equal(t, "restart", parsed["action"])
	_, hasResult := parsed["result"]
	assert.True(t, hasResult, "json passthrough 는 result 를 포함해야 합니다")
}

func TestRemoteCommand_EscapesID(t *testing.T) {
	var gotRawPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"result": "ok"}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "command", "node 01",
		"--domain", "system", "--action", "ping",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/remote/nodes/node%2001/command", gotRawPath)
}

func TestRemoteCommand_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "command", "  ", "--domain", "system", "--action", "ping"})
	require.Error(t, cmd.Execute())
}

func TestRemoteCommand_MissingDomain(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("domain 누락 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "command", testInstanceID, "--action", "ping"})
	require.Error(t, cmd.Execute())
}

func TestRemoteCommand_MissingAction(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("action 누락 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "command", testInstanceID, "--domain", "flow"})
	require.Error(t, cmd.Execute())
}

func TestRemoteCommand_BadArgsJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 args JSON 이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "command", testInstanceID,
		"--domain", "flow", "--action", "restart", "--args", "{bad json",
	})
	require.Error(t, cmd.Execute())
}

// =============================================================================
// remote audit
// =============================================================================

// sampleAuditEntries 는 테스트용 감사 로그 항목을 반환한다.
func sampleAuditEntries() []map[string]any {
	return []map[string]any{
		{
			"id":             float64(2),
			"instance_id":    testInstanceID,
			"actor":          "admin",
			"action":         "command",
			"domain":         "flow",
			"command_action": "restart",
			"result":         "ok",
			"reason":         "",
			"timestamp":      float64(1700000002000),
		},
		{
			"id":          float64(1),
			"instance_id": testInstanceID2,
			"actor":       "admin",
			"action":      "revoke",
			"result":      "ok",
			"timestamp":   float64(1700000001000),
		},
	}
}

func TestRemoteAudit(t *testing.T) {
	var gotPath, gotMethod, gotQuery string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleAuditEntries()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "audit"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/api/v1/remote/audit", gotPath)
	// 플래그 미지정 시 쿼리 파라미터가 없어야 한다.
	assert.Empty(t, gotQuery)

	out := buf.String()
	for _, h := range []string{"ID", "NODE", "ACTOR", "ACTION", "RESULT", "TIMESTAMP"} {
		assert.Contains(t, out, h)
	}
	assert.Contains(t, out, testInstanceID)
	assert.Contains(t, out, "admin")
	assert.Contains(t, out, "command")
	assert.Contains(t, out, "revoke")
	// timestamp 는 epoch ms 정수 문자열로 표시되어야 한다(과학표기 금지).
	assert.Contains(t, out, "1700000002000")
}

func TestRemoteAudit_WithFilters(t *testing.T) {
	var gotValues url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValues = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleAuditEntries()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "audit",
		"--node", testInstanceID, "--limit", "50", "--offset", "10",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, testInstanceID, gotValues.Get("instance_id"))
	assert.Equal(t, "50", gotValues.Get("limit"))
	assert.Equal(t, "10", gotValues.Get("offset"))
}

func TestRemoteAudit_OffsetZeroIncluded(t *testing.T) {
	var gotValues url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValues = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleAuditEntries()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	// offset=0 은 명시적으로 설정되면 포함되어야 한다(>=0 허용).
	cmd.SetArgs([]string{"remote", "audit", "--offset", "0"})
	require.NoError(t, cmd.Execute())

	_, hasOffset := gotValues["offset"]
	assert.True(t, hasOffset, "명시적 --offset 0 은 쿼리에 포함되어야 합니다")
	assert.Equal(t, "0", gotValues.Get("offset"))
	// limit 은 미지정이므로 생략되어야 한다.
	_, hasLimit := gotValues["limit"]
	assert.False(t, hasLimit, "--limit 미지정 시 limit 쿼리는 생략되어야 합니다")
}

func TestRemoteAudit_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleAuditEntries()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "audit"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 2)
	assert.Equal(t, "command", parsed[0]["action"])
}

func TestRemoteAudit_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]map[string]any{}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "audit"})
	require.NoError(t, cmd.Execute())
	// 빈 목록에서도 헤더는 표시되어야 한다.
	assert.Contains(t, buf.String(), "ID")
}

// =============================================================================
// remote inventory {flows|agents|devices}
// =============================================================================

// sampleMirroredResources 는 테스트용 미러 자원 목록을 반환한다.
func sampleMirroredResources() []map[string]any {
	return []map[string]any{
		{
			"id":                 "f1",
			"source_instance_id": testInstanceID,
			"name":               "flow-a",
			"kind":               "flow",
			"status":             "running",
			"updated_at":         float64(1700000000000),
			"online":             true,
		},
		{
			"id":                 "f2",
			"source_instance_id": testInstanceID2,
			"name":               "flow-b",
			"kind":               "flow",
			"status":             "stopped",
			"updated_at":         float64(1700000001000),
			"online":             false,
		},
	}
}

// inventoryResources 는 테스트가 순회할 인벤토리 자원 이름이다.
var inventoryResources = []string{"flows", "agents", "devices"}

func TestRemoteInventory_AggregateMirror(t *testing.T) {
	for _, resource := range inventoryResources {
		t.Run(resource, func(t *testing.T) {
			var gotPath, gotMethod string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMethod = r.Method
				w.Header().Set("Content-Type", "application/json")
				w.Write(remoteEnvelope(sampleMirroredResources()))
			})

			buf, cmd, cleanup := setupRemoteTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"remote", "inventory", resource})
			require.NoError(t, cmd.Execute())

			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, "/api/v1/remote/"+resource, gotPath)

			out := buf.String()
			for _, h := range []string{"ID", "SOURCE_NODE", "NAME", "KIND", "STATUS", "ONLINE", "UPDATED_AT"} {
				assert.Contains(t, out, h)
			}
			assert.Contains(t, out, "flow-a")
			assert.Contains(t, out, "flow-b")
			assert.Contains(t, out, testInstanceID)
			// online bool 은 yes/no 로 렌더링된다.
			assert.Contains(t, out, "yes")
			assert.Contains(t, out, "no")
			// updated_at 은 epoch ms 정수 문자열로 표시된다(과학표기 금지).
			assert.Contains(t, out, "1700000000000")
		})
	}
}

func TestRemoteInventory_PerNodeMirror(t *testing.T) {
	for _, resource := range inventoryResources {
		t.Run(resource, func(t *testing.T) {
			var gotPath, gotMethod string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMethod = r.Method
				w.Header().Set("Content-Type", "application/json")
				w.Write(remoteEnvelope(sampleMirroredResources()))
			})

			buf, cmd, cleanup := setupRemoteTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"remote", "inventory", resource, testInstanceID})
			require.NoError(t, cmd.Execute())

			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/"+resource, gotPath)
			assert.Contains(t, buf.String(), "SOURCE_NODE")
			assert.Contains(t, buf.String(), "flow-a")
		})
	}
}

func TestRemoteInventory_LivePerNode(t *testing.T) {
	for _, resource := range inventoryResources {
		t.Run(resource, func(t *testing.T) {
			var gotPath, gotMethod string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotMethod = r.Method
				w.Header().Set("Content-Type", "application/json")
				// 라이브 데이터는 노드의 원본 배열(자원별 형태 상이).
				w.Write(remoteEnvelope([]any{
					map[string]any{"id": "x1", "running": true, "uptime": float64(1000)},
					map[string]any{"id": "x2", "running": false, "uptime": float64(2000)},
				}))
			})

			buf, cmd, cleanup := setupRemoteTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"remote", "inventory", resource, testInstanceID, "--live"})
			require.NoError(t, cmd.Execute())

			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/"+resource+"/live", gotPath)
			// 라이브 데이터의 식별자가 출력에 포함되어야 한다.
			assert.Contains(t, buf.String(), "x1")
		})
	}
}

func TestRemoteInventory_LiveJSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/flows/live", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]any{
			map[string]any{"id": "x1", "status": "running"},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "inventory", "flows", testInstanceID, "--live"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 1)
	assert.Equal(t, "x1", parsed[0]["id"])
}

func TestRemoteInventory_MirrorJSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/flows", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleMirroredResources()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "inventory", "flows"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 2)
	assert.Equal(t, "f1", parsed[0]["id"])
}

func TestRemoteInventory_LiveWithoutIDFails(t *testing.T) {
	for _, resource := range inventoryResources {
		t.Run(resource, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("--live 에 인스턴스 ID 가 없으면 서버를 호출하면 안 됩니다")
			})

			_, cmd, cleanup := setupRemoteTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"remote", "inventory", resource, "--live"})
			require.Error(t, cmd.Execute())
		})
	}
}

func TestRemoteInventory_EscapesID(t *testing.T) {
	var gotRawPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleMirroredResources()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "inventory", "agents", "node 01"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/remote/nodes/node%2001/agents", gotRawPath)
}

func TestRemoteInventory_LiveEscapesID(t *testing.T) {
	var gotRawPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]any{}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "inventory", "devices", "node 01", "--live"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/remote/nodes/node%2001/devices/live", gotRawPath)
}

func TestRemoteInventory_LiveEmptyArrayDoesNotCrash(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]any{}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "inventory", "flows", testInstanceID, "--live"})
	require.NoError(t, cmd.Execute())
}
