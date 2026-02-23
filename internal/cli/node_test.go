package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupNodeTest 는 노드 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 커맨드 루트, 출력 버퍼를 반환한다.
func setupNodeTest(t *testing.T, handler http.Handler) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// 클라이언트를 mock 서버로 직접 생성
	client := NewClient(srv.URL, "test-token", 5*time.Second, false)

	// 루트 커맨드 생성 (format 플래그 포함)
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	// 노드 커맨드 등록 (읽기 전용이므로 confirmFn 불필요)
	rootCmd.AddCommand(newNodeCmd(&client))

	// 출력 버퍼 설정
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// --- node type (목록 조회) 테스트 ---

// TestNodeType_List - 노드 타입 목록 조회 및 테이블 출력 검증
func TestNodeType_List(t *testing.T) {
	nodes := []map[string]any{
		{
			"type":        "http-trigger",
			"category":    "trigger",
			"description": "HTTP 요청을 트리거로 사용",
			"source":      "builtin",
		},
		{
			"type":        "json-transform",
			"category":    "processor",
			"description": "JSON 데이터 변환",
			"source":      "plugin",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/nodes", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type"})
	err := cmd.Execute()
	require.NoError(t, err, "node type 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 헤더 검증
	assert.Contains(t, output, "TYPE", "테이블에 TYPE 헤더가 있어야 합니다")
	assert.Contains(t, output, "CATEGORY", "테이블에 CATEGORY 헤더가 있어야 합니다")
	assert.Contains(t, output, "DESCRIPTION", "테이블에 DESCRIPTION 헤더가 있어야 합니다")
	assert.Contains(t, output, "SOURCE", "테이블에 SOURCE 헤더가 있어야 합니다")
	// 데이터 검증
	assert.Contains(t, output, "http-trigger", "출력에 http-trigger 가 포함되어야 합니다")
	assert.Contains(t, output, "json-transform", "출력에 json-transform 이 포함되어야 합니다")
	assert.Contains(t, output, "builtin", "출력에 builtin 소스가 포함되어야 합니다")
	assert.Contains(t, output, "plugin", "출력에 plugin 소스가 포함되어야 합니다")
}

// TestNodeType_List_JSONFormat - JSON 출력 형식 검증
func TestNodeType_List_JSONFormat(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node type --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱 가능 여부 검증
	var result []map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Len(t, result, 1, "JSON 배열에 1개 항목이 있어야 합니다")
}

// TestNodeType_List_Empty - 빈 노드 목록 출력 검증
func TestNodeType_List_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type"})
	err := cmd.Execute()
	require.NoError(t, err, "빈 목록 조회 에러가 없어야 합니다")

	output := buf.String()
	// 헤더는 있어야 함
	assert.Contains(t, output, "TYPE", "빈 목록에도 테이블 헤더가 있어야 합니다")
}

// TestNodeType_List_AllFormats - 4가지 출력 형식 검증
func TestNodeType_List_AllFormats(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	formats := []string{"json", "yaml", "table", "text"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(apiEnvelope(nodes))
			})

			_, cmd, buf := setupNodeTest(t, handler)

			cmd.SetArgs([]string{"node", "type", "--format", format})
			err := cmd.Execute()
			require.NoError(t, err,
				"node type --format %s 실행 에러가 없어야 합니다", format)

			output := buf.String()
			assert.NotEmpty(t, output,
				"format=%s 에서 출력이 비어있으면 안됩니다", format)
		})
	}
}

// TestNodeType_List_APIError - API 에러가 사용자에게 올바르게 전파되는지 검증
func TestNodeType_List_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "INTERNAL_ERROR",
				"message": "내부 서버 오류",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, _ := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type"})
	err := cmd.Execute()
	require.Error(t, err, "API 에러가 전파되어야 합니다")
}

// --- node type --name 필터 테스트 ---

// TestNodeType_List_NameFilter - --name 플래그로 노드 타입 필터링 검증
func TestNodeType_List_NameFilter(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 요청을 트리거로 사용", "source": "builtin"},
		{"type": "json-transform", "category": "processor", "description": "JSON 데이터 변환", "source": "plugin"},
		{"type": "mqtt-bridge", "category": "connector", "description": "MQTT 브릿지 커넥터", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "--name", "http"})
	err := cmd.Execute()
	require.NoError(t, err, "node type --name 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "http-trigger 가 타입명 매칭으로 포함되어야 합니다")
	assert.NotContains(t, output, "mqtt-bridge", "mqtt-bridge 는 필터링되어야 합니다")
}

// TestNodeType_List_NameFilter_ByDescription - --name 필터가 설명 필드도 검색하는지 검증
func TestNodeType_List_NameFilter_ByDescription(t *testing.T) {
	nodes := []map[string]any{
		{"type": "custom-node", "category": "processor", "description": "MQTT 메시지를 처리합니다", "source": "plugin"},
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "--name", "mqtt"})
	err := cmd.Execute()
	require.NoError(t, err, "node type --name (설명 매칭) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "custom-node",
		"설명에 MQTT 가 포함된 custom-node 가 매칭되어야 합니다")
	assert.NotContains(t, output, "http-trigger",
		"http-trigger 는 필터링되어야 합니다")
}

// TestNodeType_List_NameFilter_NoMatch - --name 필터 매칭 없음
func TestNodeType_List_NameFilter_NoMatch(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "--name", "nonexistent"})
	err := cmd.Execute()
	require.NoError(t, err, "매칭 없어도 에러가 없어야 합니다")

	output := buf.String()
	assert.NotContains(t, output, "http-trigger",
		"필터링 후 http-trigger 가 없어야 합니다")
}

// --- node type <type> (상세 조회) 테스트 ---

// TestNodeType_Info - 단일 노드 타입 상세 조회 검증
func TestNodeType_Info(t *testing.T) {
	nodeInfo := map[string]any{
		"type":        "http-trigger",
		"category":    "trigger",
		"description": "HTTP 요청을 수신하여 플로우를 트리거합니다.",
		"source":      "builtin",
		"inputs":      []any{},
		"outputs": []map[string]any{
			{"name": "body", "type": "object"},
			{"name": "headers", "type": "object"},
		},
		"config_schema": map[string]any{
			"port":   "number",
			"method": "string",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/nodes/http-trigger", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodeInfo))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "http-trigger", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node type <type> 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "출력에 카테고리가 포함되어야 합니다")
	assert.Contains(t, output, "body", "출력에 출력 포트 이름이 포함되어야 합니다")
	assert.Contains(t, output, "config_schema", "출력에 설정 스키마가 포함되어야 합니다")
}

// TestNodeType_Info_TextFormat - text 출력 형식 검증
func TestNodeType_Info_TextFormat(t *testing.T) {
	nodeInfo := map[string]any{
		"type":        "json-transform",
		"category":    "processor",
		"description": "JSON 데이터를 변환합니다.",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodeInfo))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "json-transform", "--format", "text"})
	err := cmd.Execute()
	require.NoError(t, err, "node type <type> --format text 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "json-transform", "text 형식에 노드 타입이 포함되어야 합니다")
}

// TestNodeType_Info_TableFormatFallback - 단일 객체에서 table 형식이 text 로 전환되는지 검증
func TestNodeType_Info_TableFormatFallback(t *testing.T) {
	nodeInfo := map[string]any{
		"type":        "http-trigger",
		"category":    "trigger",
		"description": "HTTP 트리거",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodeInfo))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	// 기본 형식이 table 이지만 단일 객체이므로 text 로 전환되어야 함
	cmd.SetArgs([]string{"node", "type", "http-trigger"})
	err := cmd.Execute()
	require.NoError(t, err, "node type <type> (table->text 폴백) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "출력에 카테고리가 포함되어야 합니다")
}

// TestNodeType_Info_APIError - API 에러 전파 검증
func TestNodeType_Info_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "NOT_FOUND",
				"message": "노드 타입을 찾을 수 없습니다",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, _ := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "nonexistent-node"})
	err := cmd.Execute()
	require.Error(t, err, "존재하지 않는 노드 타입은 에러를 반환해야 합니다")
}

// --- node 서브커맨드 등록 검증 ---

// TestNodeSubcommands - 서브커맨드 등록 여부 검증
func TestNodeSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)

	nodeCmd := newNodeCmd(&client)

	subNames := make(map[string]bool)
	for _, sub := range nodeCmd.Commands() {
		parts := strings.Fields(sub.Use)
		if len(parts) > 0 {
			subNames[parts[0]] = true
		}
	}

	assert.True(t, subNames["type"],
		"node 에 'type' 서브커맨드가 등록되어 있어야 합니다")
	assert.True(t, subNames["list"],
		"node 에 'list' 서브커맨드가 등록되어 있어야 합니다")
}

// TestNewNodeCmd_Signature - newNodeCmd 함수 시그니처 검증
func TestNewNodeCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)

	nodeCmd := newNodeCmd(&client)
	require.NotNil(t, nodeCmd, "newNodeCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "node", nodeCmd.Use, "node 커맨드의 Use 가 'node' 여야 합니다")
}

// TestNodeType_Info_YAMLFormat - YAML 출력 형식 검증
func TestNodeType_Info_YAMLFormat(t *testing.T) {
	nodeInfo := map[string]any{
		"type":        "http-trigger",
		"category":    "trigger",
		"description": "HTTP 트리거",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodeInfo))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "http-trigger", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "node type <type> --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "YAML 출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "YAML 출력에 카테고리가 포함되어야 합니다")
}

// TestNodeType_List_YAMLFormat - node type YAML 출력 형식 검증
func TestNodeType_List_YAMLFormat(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "node type --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "YAML 출력이 비어있으면 안됩니다")
	assert.Contains(t, output, "http-trigger", "YAML 출력에 노드 타입이 포함되어야 합니다")
}

// TestNodeType_Info_WithPorts - 입출력 포트 정보가 올바르게 표시되는지 검증
func TestNodeType_Info_WithPorts(t *testing.T) {
	nodeInfo := map[string]any{
		"type":        "email-sender",
		"category":    "action",
		"description": "이메일을 발송합니다.",
		"source":      "plugin",
		"inputs": []map[string]any{
			{"name": "to", "type": "string"},
			{"name": "subject", "type": "string"},
			{"name": "body", "type": "string"},
		},
		"outputs": []map[string]any{
			{"name": "success", "type": "boolean"},
			{"name": "message_id", "type": "string"},
		},
		"config_schema": map[string]any{
			"smtp_host": "string",
			"smtp_port": "number",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/nodes/email-sender", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodeInfo))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "type", "email-sender", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node type <type> (포트 포함) 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱으로 상세 검증
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "email-sender", result["type"], "타입이 일치해야 합니다")
	assert.Equal(t, "action", result["category"], "카테고리가 일치해야 합니다")

	// 입력 포트 검증
	inputs, ok := result["inputs"].([]any)
	require.True(t, ok, "inputs 가 배열이어야 합니다")
	assert.Len(t, inputs, 3, "입력 포트가 3개여야 합니다")

	// 출력 포트 검증
	outputs, ok := result["outputs"].([]any)
	require.True(t, ok, "outputs 가 배열이어야 합니다")
	assert.Len(t, outputs, 2, "출력 포트가 2개여야 합니다")
}

// TestNodeRowFunc_InvalidType - nodeRowFunc 에 map[string]any 가 아닌 값이 전달될 때 빈 행 반환 검증
func TestNodeRowFunc_InvalidType(t *testing.T) {
	// 문자열 전달 시 빈 행이 반환되어야 함
	row := nodeRowFunc("invalid-item")
	assert.Equal(t, []string{"", "", "", ""}, row, "map[string]any 가 아닌 값은 빈 행을 반환해야 합니다")

	// nil 전달 시에도 빈 행이 반환되어야 함
	row = nodeRowFunc(nil)
	assert.Equal(t, []string{"", "", "", ""}, row, "nil 은 빈 행을 반환해야 합니다")
}

// --- node list (런타임 노드 인스턴스) 테스트 ---

// TestNodeList_ForFlow - 특정 플로우의 노드 인스턴스 목록 조회
func TestNodeList_ForFlow(t *testing.T) {
	flowID := "f1234567-1234-1234-1234-123456789012"
	flowNodes := []map[string]any{
		{"node_id": "n1", "name": "sensor", "type": "mqtt-subscriber", "state": "running"},
		{"node_id": "n2", "name": "transform", "type": "json-transform", "state": "running"},
	}
	flowInfo := map[string]any{"id": flowID, "name": "my-flow"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/flows/"+flowID+"/nodes":
			w.Write(apiEnvelope(flowNodes))
		case r.URL.Path == "/api/v1/flows/"+flowID:
			w.Write(apiEnvelope(flowInfo))
		case r.URL.Path == "/api/v1/flows":
			// resolveFlowID 에서 이름 검색 시 사용
			w.Write(apiEnvelope([]map[string]any{flowInfo}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "my-flow"})
	err := cmd.Execute()
	require.NoError(t, err, "node list <flow> 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "FLOW", "테이블에 FLOW 헤더가 있어야 합니다")
	assert.Contains(t, output, "NODE_ID", "테이블에 NODE_ID 헤더가 있어야 합니다")
	assert.Contains(t, output, "sensor", "출력에 sensor 노드가 포함되어야 합니다")
	assert.Contains(t, output, "transform", "출력에 transform 노드가 포함되어야 합니다")
	assert.Contains(t, output, "my-flow", "출력에 플로우 이름이 포함되어야 합니다")
}

// TestNodeList_ForFlow_NameFilter - 플로우 내 노드를 이름으로 필터링
func TestNodeList_ForFlow_NameFilter(t *testing.T) {
	flowID := "f1234567-1234-1234-1234-123456789012"
	flowNodes := []map[string]any{
		{"node_id": "n1", "name": "sensor", "type": "mqtt-subscriber", "state": "running"},
		{"node_id": "n2", "name": "transform", "type": "json-transform", "state": "running"},
	}
	flowInfo := map[string]any{"id": flowID, "name": "my-flow"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/flows/"+flowID+"/nodes":
			w.Write(apiEnvelope(flowNodes))
		case r.URL.Path == "/api/v1/flows/"+flowID:
			w.Write(apiEnvelope(flowInfo))
		case r.URL.Path == "/api/v1/flows":
			w.Write(apiEnvelope([]map[string]any{flowInfo}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "my-flow", "--name", "sensor"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --name 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "sensor", "sensor 가 포함되어야 합니다")
	assert.NotContains(t, output, "transform", "transform 은 필터링되어야 합니다")
}

// TestNodeList_All - 모든 플로우의 노드 인스턴스 집계 조회
func TestNodeList_All(t *testing.T) {
	flow1ID := "f1111111-1111-1111-1111-111111111111"
	flow2ID := "f2222222-2222-2222-2222-222222222222"

	flows := []map[string]any{
		{"id": flow1ID, "name": "flow-a"},
		{"id": flow2ID, "name": "flow-b"},
	}

	flow1Nodes := []map[string]any{
		{"node_id": "n1", "name": "reader", "type": "file-reader", "state": "running"},
	}
	flow2Nodes := []map[string]any{
		{"node_id": "n2", "name": "writer", "type": "file-writer", "state": "stopped"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/flows":
			w.Write(apiEnvelope(flows))
		case "/api/v1/flows/" + flow1ID + "/nodes":
			w.Write(apiEnvelope(flow1Nodes))
		case "/api/v1/flows/" + flow2ID + "/nodes":
			w.Write(apiEnvelope(flow2Nodes))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "node list (전체) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-a", "출력에 flow-a 가 포함되어야 합니다")
	assert.Contains(t, output, "flow-b", "출력에 flow-b 가 포함되어야 합니다")
	assert.Contains(t, output, "reader", "출력에 reader 노드가 포함되어야 합니다")
	assert.Contains(t, output, "writer", "출력에 writer 노드가 포함되어야 합니다")
}

// TestNodeList_All_Empty - 플로우가 없는 경우
func TestNodeList_All_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "빈 목록 조회 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "FLOW", "빈 목록에도 테이블 헤더가 있어야 합니다")
}

// TestNodeList_All_SkipUndeployedFlow - 미배포 플로우는 건너뛰기
func TestNodeList_All_SkipUndeployedFlow(t *testing.T) {
	flow1ID := "f1111111-1111-1111-1111-111111111111"
	flow2ID := "f2222222-2222-2222-2222-222222222222"

	flows := []map[string]any{
		{"id": flow1ID, "name": "deployed"},
		{"id": flow2ID, "name": "undeployed"},
	}

	flow1Nodes := []map[string]any{
		{"node_id": "n1", "name": "active-node", "type": "processor", "state": "running"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/flows":
			w.Write(apiEnvelope(flows))
		case "/api/v1/flows/" + flow1ID + "/nodes":
			w.Write(apiEnvelope(flow1Nodes))
		case "/api/v1/flows/" + flow2ID + "/nodes":
			// 미배포 → 에러 반환
			w.WriteHeader(http.StatusNotFound)
			resp := map[string]any{"success": false, "error": map[string]any{"code": "NOT_FOUND", "message": "flow not deployed"}}
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "미배포 플로우 스킵 후 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "active-node", "배포된 플로우의 노드는 포함되어야 합니다")
}

// TestNodeList_JSONFormat - node list JSON 출력 형식 검증
func TestNodeList_JSONFormat(t *testing.T) {
	flow1ID := "f1111111-1111-1111-1111-111111111111"
	flows := []map[string]any{{"id": flow1ID, "name": "test-flow"}}
	nodes := []map[string]any{
		{"node_id": "n1", "name": "sensor", "type": "mqtt", "state": "running"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/flows":
			w.Write(apiEnvelope(flows))
		case "/api/v1/flows/" + flow1ID + "/nodes":
			w.Write(apiEnvelope(nodes))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --format json 에러가 없어야 합니다")

	var result []map[string]any
	err = json.Unmarshal([]byte(buf.String()), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Len(t, result, 1, "JSON 배열에 1개 항목이 있어야 합니다")
	assert.Equal(t, "test-flow", result[0]["flow"], "flow 필드가 포함되어야 합니다")
}

// TestNodeInstanceRowFunc_InvalidType - nodeInstanceRowFunc 에 잘못된 타입 전달 시 빈 행 반환
func TestNodeInstanceRowFunc_InvalidType(t *testing.T) {
	row := nodeInstanceRowFunc("invalid")
	assert.Equal(t, []string{"", "", "", "", ""}, row, "잘못된 타입은 빈 행을 반환해야 합니다")
}

// TestNodeCmd_NoSubcommand - 서브커맨드 없이 node 커맨드만 실행했을 때 도움말 검증
func TestNodeCmd_NoSubcommand(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("서브커맨드 없이 서버 요청이 없어야 합니다")
	})

	_, cmd, _ := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node"})
	// 서브커맨드 없이 실행하면 에러 없이 도움말이 표시됨
	err := cmd.Execute()
	require.NoError(t, err, "서브커맨드 없이 node 실행은 에러가 아닙니다")
}
