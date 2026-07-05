package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupRemoteTest 는 mock 서버와 remote 커맨드 루트를 세팅하는 헬퍼이다.
// confirmFn 은 기본적으로 true(승인)를 반환한다(device_test.go 하니스와 동일).
func setupRemoteTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()
	return setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
}

// setupRemoteTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupRemoteTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newRemoteCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// remoteEnvelope 은 원격 API 응답 엔벨로프를 생성한다.
func remoteEnvelope(data any) []byte {
	resp := map[string]any{"success": true, "data": data}
	b, _ := json.Marshal(resp)
	return b
}

const (
	testInstanceID  = "node-01"
	testInstanceID2 = "node-02"
)

// sampleNodes 는 테스트용 노드 목록을 반환한다.
func sampleNodes() []map[string]any {
	return []map[string]any{
		{
			"instance_id": testInstanceID,
			"hostname":    "edge-a",
			"version":     "v0.18.6",
			"status":      "approved",
			"online":      true,
			"group_name":  "1f",
			"last_seen":   float64(1700000000000),
			"outdated":    false,
		},
		{
			"instance_id": testInstanceID2,
			"hostname":    "edge-b",
			"version":     "v0.18.0",
			"status":      "pending",
			"online":      false,
			"group_name":  "2f",
			"last_seen":   float64(1700000001000),
			"outdated":    true,
		},
	}
}

// --- remote node list ---

func TestRemoteNodeList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/nodes", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleNodes()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "list"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "INSTANCE_ID")
	assert.Contains(t, out, "HOSTNAME")
	assert.Contains(t, out, "VERSION")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "ONLINE")
	assert.Contains(t, out, "GROUP")
	assert.Contains(t, out, "LAST_SEEN")
	assert.Contains(t, out, "OUTDATED")
	assert.Contains(t, out, "edge-a")
	assert.Contains(t, out, "edge-b")
	// online bool 은 yes/no 로 렌더링된다.
	assert.Contains(t, out, "yes")
	assert.Contains(t, out, "no")
	// last_seen 은 epoch ms 정수 문자열로 표시된다(과학표기 금지).
	assert.Contains(t, out, "1700000000000")
}

func TestRemoteNodeList_Pending(t *testing.T) {
	var gotPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]map[string]any{sampleNodes()[1]}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "list", "--pending"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/remote/nodes/pending", gotPath)
	assert.Contains(t, buf.String(), "edge-b")
}

func TestRemoteNodeList_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/nodes", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleNodes()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "node", "list"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 2)
	assert.Equal(t, testInstanceID, parsed[0]["instance_id"])
}

// --- remote node get ---

// sampleNodeDetail 은 테스트용 단일 노드 상세(NodeDetailDTO)를 반환한다.
// 운영 요약(summary)과 시스템 정보(os/arch), uptime 을 포함한다.
func sampleNodeDetail() map[string]any {
	return map[string]any{
		"instance_id":             testInstanceID,
		"hostname":                "edge-a",
		"version":                 "v0.18.6",
		"status":                  "approved",
		"online":                  true,
		"group_name":              "1f",
		"os":                      "linux",
		"arch":                    "arm64",
		"started_at":              float64(1700000000000),
		"uptime":                  float64(3600000),
		"display_width":           float64(1920),
		"display_height":          float64(1080),
		"display_override_width":  float64(0),
		"display_override_height": float64(0),
		"display_reported_width":  float64(1920),
		"display_reported_height": float64(1080),
		"last_seen":               float64(1700000003600),
		"summary": map[string]any{
			"flows":   map[string]any{"total": float64(3), "running": float64(2), "stopped": float64(1)},
			"agents":  map[string]any{"total": float64(2), "connected": float64(2)},
			"devices": map[string]any{"total": float64(5), "online": float64(4)},
		},
	}
}

func TestRemoteNodeGet(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleNodeDetail()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "get", testInstanceID})
	require.NoError(t, cmd.Execute())

	// get 은 단일 노드 GET 엔드포인트를 호출한다(목록+필터링 아님).
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID, gotPath)

	out := buf.String()
	// 스칼라 필드(시스템 정보 포함)가 표시되어야 한다.
	assert.Contains(t, out, testInstanceID)
	assert.Contains(t, out, "edge-a")
	assert.Contains(t, out, "linux")
	assert.Contains(t, out, "arm64")
	// epoch-ms 필드는 정수 문자열로 표시된다(과학표기 금지).
	assert.Contains(t, out, "1700000000000")
	assert.Contains(t, out, "1700000003600")
	// uptime 도 정수 문자열로 표시된다.
	assert.Contains(t, out, "3600000")
	// summary 중첩 객체는 별도 섹션으로 렌더링된다.
	assert.Contains(t, out, "Summary")
	assert.Contains(t, out, "flows")
	assert.Contains(t, out, "agents")
	assert.Contains(t, out, "devices")
}

// TestRemoteNodeGet_EscapesID 는 instance_id 가 URL 경로로 이스케이프되는지 검증한다.
func TestRemoteNodeGet_EscapesID(t *testing.T) {
	var gotRawPath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// EscapedPath 로 원본 인코딩을 확인한다.
		gotRawPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleNodeDetail()))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "get", "node 01"})
	require.NoError(t, cmd.Execute())

	// 공백이 %20 으로 이스케이프되어야 한다.
	assert.Equal(t, "/api/v1/remote/nodes/node%2001", gotRawPath)
}

func TestRemoteNodeGet_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 서버는 미존재 노드에 404(not-found)를 반환한다(NodeDetail → ErrManagedNodeNotFound).
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"success":false,"error":{"code":"not_found","message":"원격 노드를 찾을 수 없습니다"}}`))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "get", "no-such-node"})
	err := cmd.Execute()
	require.Error(t, err)
	// MapAPIError 가 서버의 not-found 메시지를 surface 한다(클라이언트 필터링 없음).
	assert.Contains(t, err.Error(), "찾을 수 없습니다")
}

func TestRemoteNodeGet_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "get", "   "})
	require.Error(t, cmd.Execute())
}

func TestRemoteNodeGet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleNodeDetail()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "node", "get", testInstanceID})
	require.NoError(t, cmd.Execute())

	// json 포맷은 전체 객체를 그대로 통과시킨다(summary 중첩 객체 포함).
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, testInstanceID, parsed["instance_id"])
	assert.Equal(t, "linux", parsed["os"])
	_, hasSummary := parsed["summary"]
	assert.True(t, hasSummary, "json passthrough 는 summary 를 포함해야 합니다")
}

// --- remote node approve ---

func TestRemoteNodeApprove(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "approved",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "approve", testInstanceID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/approve", gotPath)
	assert.Contains(t, buf.String(), "승인")
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteNodeApprove_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "approved",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "node", "approve", testInstanceID})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "approved", parsed["status"])
}

func TestRemoteNodeApprove_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "approve", "  "})
	require.Error(t, cmd.Execute())
}

// --- remote node reject ---

func TestRemoteNodeReject_WithReason(t *testing.T) {
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
			"status":      "rejected",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "reject", testInstanceID, "--reason", "버전 미달"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/reject", gotPath)
	assert.Equal(t, "버전 미달", gotBody["reason"])
	assert.Contains(t, buf.String(), "거부")
}

func TestRemoteNodeReject_NoReasonOmitsField(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "rejected",
		}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "reject", testInstanceID})
	require.NoError(t, cmd.Execute())

	_, hasReason := gotBody["reason"]
	assert.False(t, hasReason, "--reason 미지정 시 reason 필드는 생략되어야 합니다")
}

// --- remote node revoke ---

func TestRemoteNodeRevoke_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "revoked",
		}))
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "revoke", testInstanceID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes/"+testInstanceID+"/revoke", gotPath)
	assert.Contains(t, buf.String(), "폐기")
}

func TestRemoteNodeRevoke_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "revoke", testInstanceID})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteNodeRevoke_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "revoked",
		}))
	})

	// confirmFn 이 false 여도 --yes 면 폐기가 진행되어야 한다.
	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "revoke", testInstanceID, "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 폐기를 진행해야 합니다")
}

// --- remote node pre-register ---

func TestRemoteNodePreRegister(t *testing.T) {
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
			"hostname":    "edge-a",
			"status":      "approved",
			"online":      false,
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "pre-register", testInstanceID, "--name", "edge-a"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/nodes", gotPath)
	assert.Equal(t, testInstanceID, gotBody["instance_id"])
	assert.Equal(t, "edge-a", gotBody["name"])
	assert.Contains(t, buf.String(), testInstanceID)
}

func TestRemoteNodePreRegister_NoNameOmitsField(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"instance_id": testInstanceID,
			"status":      "approved",
		}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "pre-register", testInstanceID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, testInstanceID, gotBody["instance_id"])
	_, hasName := gotBody["name"]
	assert.False(t, hasName, "--name 미지정 시 name 필드는 생략되어야 합니다")
}

func TestRemoteNodePreRegister_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 instance_id 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "node", "pre-register", "   "})
	require.Error(t, cmd.Execute())
}
