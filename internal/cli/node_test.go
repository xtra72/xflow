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

// --- node list 테스트 ---

// TestNodeList - 노드 타입 목록 조회 및 테이블 출력 검증
func TestNodeList(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "node list 실행 에러가 없어야 합니다")

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

// TestNodeList_JSONFormat - JSON 출력 형식 검증
func TestNodeList_JSONFormat(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱 가능 여부 검증
	var result []map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Len(t, result, 1, "JSON 배열에 1개 항목이 있어야 합니다")
}

// TestNodeList_Empty - 빈 노드 목록 출력 검증
func TestNodeList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "빈 목록 조회 에러가 없어야 합니다")

	output := buf.String()
	// 헤더는 있어야 함
	assert.Contains(t, output, "TYPE", "빈 목록에도 테이블 헤더가 있어야 합니다")
}

// TestNodeList_AllFormats - list 의 4가지 출력 형식 검증
func TestNodeList_AllFormats(t *testing.T) {
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

			cmd.SetArgs([]string{"node", "list", "--format", format})
			err := cmd.Execute()
			require.NoError(t, err,
				"node list --format %s 실행 에러가 없어야 합니다", format)

			output := buf.String()
			assert.NotEmpty(t, output,
				"format=%s 에서 출력이 비어있으면 안됩니다", format)
		})
	}
}

// TestNodeList_APIError - API 에러가 사용자에게 올바르게 전파되는지 검증
func TestNodeList_APIError(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "list"})
	err := cmd.Execute()
	require.Error(t, err, "API 에러가 전파되어야 합니다")
}

// --- node list --name 필터 테스트 ---

// TestNodeList_NameFilter - --name 플래그로 노드 타입 필터링 검증
func TestNodeList_NameFilter(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "list", "--name", "http"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --name 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "http-trigger 가 타입명 매칭으로 포함되어야 합니다")
	assert.NotContains(t, output, "mqtt-bridge", "mqtt-bridge 는 필터링되어야 합니다")
}

// TestNodeList_NameFilter_ByDescription - --name 필터가 설명 필드도 검색하는지 검증
func TestNodeList_NameFilter_ByDescription(t *testing.T) {
	nodes := []map[string]any{
		{"type": "custom-node", "category": "processor", "description": "MQTT 메시지를 처리합니다", "source": "plugin"},
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "--name", "mqtt"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --name (설명 매칭) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "custom-node",
		"설명에 MQTT 가 포함된 custom-node 가 매칭되어야 합니다")
	assert.NotContains(t, output, "http-trigger",
		"http-trigger 는 필터링되어야 합니다")
}

// TestNodeList_NameFilter_NoMatch - --name 필터 매칭 없음
func TestNodeList_NameFilter_NoMatch(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "--name", "nonexistent"})
	err := cmd.Execute()
	require.NoError(t, err, "매칭 없어도 에러가 없어야 합니다")

	output := buf.String()
	assert.NotContains(t, output, "http-trigger",
		"필터링 후 http-trigger 가 없어야 합니다")
}

// --- node info 테스트 ---

// TestNodeInfo - 단일 노드 타입 상세 조회 검증
func TestNodeInfo(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "info", "http-trigger", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node info 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "출력에 카테고리가 포함되어야 합니다")
	assert.Contains(t, output, "body", "출력에 출력 포트 이름이 포함되어야 합니다")
	assert.Contains(t, output, "config_schema", "출력에 설정 스키마가 포함되어야 합니다")
}

// TestNodeInfo_TextFormat - text 출력 형식 검증
func TestNodeInfo_TextFormat(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "info", "json-transform", "--format", "text"})
	err := cmd.Execute()
	require.NoError(t, err, "node info --format text 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "json-transform", "text 형식에 노드 타입이 포함되어야 합니다")
}

// TestNodeInfo_TableFormatFallback - 단일 객체에서 table 형식이 text 로 전환되는지 검증
func TestNodeInfo_TableFormatFallback(t *testing.T) {
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
	cmd.SetArgs([]string{"node", "info", "http-trigger"})
	err := cmd.Execute()
	require.NoError(t, err, "node info (table->text 폴백) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "출력에 카테고리가 포함되어야 합니다")
}

// TestNodeInfo_MissingArg - 타입 인자 누락 시 에러 검증
func TestNodeInfo_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, _ := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "info"})
	err := cmd.Execute()
	require.Error(t, err, "타입 인자 누락 시 에러를 반환해야 합니다")
}

// TestNodeInfo_APIError - API 에러 전파 검증
func TestNodeInfo_APIError(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "info", "nonexistent-node"})
	err := cmd.Execute()
	require.Error(t, err, "존재하지 않는 노드 타입은 에러를 반환해야 합니다")
}

// --- node 서브커맨드 등록 검증 ---

// TestNodeSubcommands - 서브커맨드 등록 여부 검증
func TestNodeSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)

	nodeCmd := newNodeCmd(&client)

	expectedSubcommands := []string{"list", "info"}

	subNames := make(map[string]bool)
	for _, sub := range nodeCmd.Commands() {
		// Use 필드에 인자 정보가 포함될 수 있으므로 첫 번째 단어만 검사
		parts := strings.Fields(sub.Use)
		if len(parts) > 0 {
			subNames[parts[0]] = true
		}
	}

	for _, expected := range expectedSubcommands {
		assert.True(t, subNames[expected],
			"node 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// TestNewNodeCmd_Signature - newNodeCmd 함수 시그니처 검증
func TestNewNodeCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)

	nodeCmd := newNodeCmd(&client)
	require.NotNil(t, nodeCmd, "newNodeCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "node", nodeCmd.Use, "node 커맨드의 Use 가 'node' 여야 합니다")
}

// TestNodeInfo_YAMLFormat - YAML 출력 형식 검증
func TestNodeInfo_YAMLFormat(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "info", "http-trigger", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "node info --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "http-trigger", "YAML 출력에 노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "trigger", "YAML 출력에 카테고리가 포함되어야 합니다")
}

// TestNodeList_YAMLFormat - node list YAML 출력 형식 검증
func TestNodeList_YAMLFormat(t *testing.T) {
	nodes := []map[string]any{
		{"type": "http-trigger", "category": "trigger", "description": "HTTP 트리거", "source": "builtin"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(nodes))
	})

	_, cmd, buf := setupNodeTest(t, handler)

	cmd.SetArgs([]string{"node", "list", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "node list --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "YAML 출력이 비어있으면 안됩니다")
	assert.Contains(t, output, "http-trigger", "YAML 출력에 노드 타입이 포함되어야 합니다")
}

// TestNodeInfo_WithPorts - 입출력 포트 정보가 올바르게 표시되는지 검증
func TestNodeInfo_WithPorts(t *testing.T) {
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

	cmd.SetArgs([]string{"node", "info", "email-sender", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "node info (포트 포함) 실행 에러가 없어야 합니다")

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
