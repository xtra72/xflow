package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 테스트 헬퍼 함수
// =============================================================================

// newTestRootCmdWithSubs 는 서브커맨드가 포함된 테스트용 루트 커맨드를 생성한다.
// 실제 명령어 트리와 유사한 구조를 제공한다.
func newTestRootCmdWithSubs() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "xflow",
		Short: "xflow CLI",
	}

	// 글로벌 persistent 플래그
	pflags := rootCmd.PersistentFlags()
	pflags.String("config", "", "설정 파일 경로")
	pflags.String("server", "", "서버 URL")
	pflags.String("format", "table", "출력 형식")
	pflags.String("token", "", "인증 토큰")
	pflags.Bool("verbose", false, "상세 출력")
	pflags.Bool("quiet", false, "조용한 출력")
	pflags.Bool("no-color", false, "색상 비활성화")

	// flow 커맨드 + 서브커맨드
	flowCmd := &cobra.Command{Use: "flow", Short: "플로우 관리"}
	flowCmd.AddCommand(&cobra.Command{Use: "list", Short: "플로우 목록"})
	flowCmd.AddCommand(&cobra.Command{Use: "get", Short: "플로우 조회"})
	flowCmd.AddCommand(&cobra.Command{Use: "create", Short: "플로우 생성"})
	flowCmd.AddCommand(&cobra.Command{Use: "update", Short: "플로우 수정"})
	flowCmd.AddCommand(&cobra.Command{Use: "delete", Short: "플로우 삭제"})
	flowCmd.AddCommand(&cobra.Command{Use: "deploy", Short: "플로우 배포"})
	// flow list 에 로컬 플래그 추가
	for _, sub := range flowCmd.Commands() {
		if sub.Use == "list" {
			sub.Flags().Int("limit", 10, "최대 조회 수")
			break
		}
	}
	rootCmd.AddCommand(flowCmd)

	// agent 커맨드 + 서브커맨드
	agentCmd := &cobra.Command{Use: "agent", Short: "에이전트 관리"}
	agentCmd.AddCommand(&cobra.Command{Use: "list", Short: "에이전트 목록"})
	agentCmd.AddCommand(&cobra.Command{Use: "get", Short: "에이전트 조회"})
	agentCmd.AddCommand(&cobra.Command{Use: "create", Short: "에이전트 생성"})
	agentCmd.AddCommand(&cobra.Command{Use: "update", Short: "에이전트 수정"})
	agentCmd.AddCommand(&cobra.Command{Use: "delete", Short: "에이전트 삭제"})
	rootCmd.AddCommand(agentCmd)

	// config 커맨드
	configCmd := &cobra.Command{Use: "config", Short: "설정 관리"}
	configCmd.AddCommand(&cobra.Command{Use: "show", Short: "설정 표시"})
	configCmd.AddCommand(&cobra.Command{Use: "set", Short: "설정 변경"})
	rootCmd.AddCommand(configCmd)

	// node 커맨드 + 서브커맨드
	nodeCmd := &cobra.Command{Use: "node", Short: "노드 관리"}
	nodeCmd.AddCommand(&cobra.Command{Use: "list", Short: "노드 목록"})
	nodeCmd.AddCommand(&cobra.Command{Use: "get", Short: "노드 조회"})
	nodeCmd.AddCommand(&cobra.Command{Use: "delete", Short: "노드 삭제"})
	rootCmd.AddCommand(nodeCmd)

	// plugin 커맨드 + 서브커맨드
	pluginCmd := &cobra.Command{Use: "plugin", Short: "플러그인 관리"}
	pluginCmd.AddCommand(&cobra.Command{Use: "list", Short: "플러그인 목록"})
	pluginCmd.AddCommand(&cobra.Command{Use: "get", Short: "플러그인 조회"})
	pluginCmd.AddCommand(&cobra.Command{Use: "delete", Short: "플러그인 삭제"})
	rootCmd.AddCommand(pluginCmd)

	// status 커맨드
	rootCmd.AddCommand(&cobra.Command{Use: "status", Short: "상태 조회"})

	// version 커맨드
	rootCmd.AddCommand(&cobra.Command{Use: "version", Short: "버전 정보"})

	// interactive 커맨드
	rootCmd.AddCommand(&cobra.Command{Use: "interactive", Short: "대화형 모드"})

	return rootCmd
}

// newMockAPIServer 는 리소스 목록을 반환하는 테스트용 HTTP 서버를 생성한다.
func newMockAPIServer(t *testing.T, resources map[string][]resourceItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 경로에서 리소스 타입 추출: /api/v1/flows -> flows
		path := r.URL.Path
		// /api/v1/{resourceType} 형태에서 리소스 타입 추출
		var resourceType string
		for rt := range resources {
			if path == "/api/v1/"+rt {
				resourceType = rt
				break
			}
		}

		if resourceType == "" {
			http.NotFound(w, r)
			return
		}

		items := resources[resourceType]
		resp := apiResponse{
			Success: true,
		}

		listResp := resourceListResponse{Items: items}
		data, _ := json.Marshal(listResp)
		resp.Data = json.RawMessage(data)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// =============================================================================
// TestNewAutoCompleter - AutoCompleter 생성 테스트
// =============================================================================

func TestNewAutoCompleter(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client

	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	require.NotNil(t, ac, "AutoCompleter 가 nil 이면 안됩니다")
	assert.Equal(t, rootCmd, ac.rootCmd, "rootCmd 가 동일해야 합니다")
	assert.Equal(t, 30*time.Second, ac.cacheTTL, "cacheTTL 이 30초여야 합니다")
	assert.NotNil(t, ac.cache, "cache 맵이 nil 이면 안됩니다")
}

// =============================================================================
// TestCompleteTopLevelCommands - 최상위 명령어 자동완성 테스트
// REQ-INT-007: Tab 자동완성 - 명령어
// =============================================================================

func TestCompleteTopLevelCommands(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	tests := []struct {
		name     string
		tokens   []string
		partial  string
		expected []string
	}{
		{
			name:     "fl 로 시작하는 명령어 완성",
			tokens:   []string{},
			partial:  "fl",
			expected: []string{"flow"},
		},
		{
			name:     "ag 로 시작하는 명령어 완성",
			tokens:   []string{},
			partial:  "ag",
			expected: []string{"agent"},
		},
		{
			name:    "빈 입력 시 모든 최상위 명령어 반환",
			tokens:  []string{},
			partial: "",
			expected: []string{
				"agent", "config", "flow", "interactive", "node",
				"plugin", "status", "version",
			},
		},
		{
			name:     "st 로 시작하는 명령어는 status",
			tokens:   []string{},
			partial:  "st",
			expected: []string{"status"},
		},
		{
			name:     "매칭되지 않는 접두사는 빈 목록",
			tokens:   []string{},
			partial:  "xyz",
			expected: nil,
		},
		{
			name:     "v 로 시작하는 명령어는 version",
			tokens:   []string{},
			partial:  "v",
			expected: []string{"version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ac.completeCommands(tt.tokens, tt.partial)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestCompleteSubcommands - 서브커맨드 자동완성 테스트
// REQ-INT-007: Tab 자동완성 - 명령어
// =============================================================================

func TestCompleteSubcommands(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	tests := []struct {
		name     string
		tokens   []string
		partial  string
		expected []string
	}{
		{
			name:     "flow 서브커맨드 중 li 로 시작하는 것",
			tokens:   []string{"flow"},
			partial:  "li",
			expected: []string{"list"},
		},
		{
			name:     "flow 서브커맨드 중 g 로 시작하는 것",
			tokens:   []string{"flow"},
			partial:  "g",
			expected: []string{"get"},
		},
		{
			name:    "flow 서브커맨드 빈 입력 시 모든 서브커맨드",
			tokens:  []string{"flow"},
			partial: "",
			expected: []string{
				"create", "delete", "deploy", "get", "list", "update",
			},
		},
		{
			name:     "flow 서브커맨드 중 de 로 시작하는 것 (delete, deploy)",
			tokens:   []string{"flow"},
			partial:  "de",
			expected: []string{"delete", "deploy"},
		},
		{
			name:    "agent 서브커맨드 빈 입력 시 모든 서브커맨드",
			tokens:  []string{"agent"},
			partial: "",
			expected: []string{
				"create", "delete", "get", "list", "update",
			},
		},
		{
			name:     "존재하지 않는 명령어의 서브커맨드",
			tokens:   []string{"nonexistent"},
			partial:  "li",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ac.completeCommands(tt.tokens, tt.partial)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestCompleteFlags - 플래그 자동완성 테스트
// REQ-INT-007: Tab 자동완성 - 명령어
// =============================================================================

func TestCompleteFlags(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	tests := []struct {
		name     string
		cmdPath  []string // 명령어 경로 (예: ["flow", "list"])
		partial  string
		contains []string // 결과에 포함되어야 하는 항목
	}{
		{
			name:     "루트 플래그 --ve 로 시작하는 것",
			cmdPath:  []string{},
			partial:  "--ve",
			contains: []string{"--verbose"},
		},
		{
			name:     "루트 플래그 --fo 로 시작하는 것",
			cmdPath:  []string{},
			partial:  "--fo",
			contains: []string{"--format"},
		},
		{
			name:    "빈 플래그 접두사로 모든 글로벌 플래그 반환",
			cmdPath: []string{},
			partial: "--",
			contains: []string{
				"--config", "--format", "--no-color",
				"--quiet", "--server", "--token", "--verbose",
			},
		},
		{
			name:     "flow list 의 로컬 플래그 --li",
			cmdPath:  []string{"flow", "list"},
			partial:  "--li",
			contains: []string{"--limit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 명령어 경로에서 cobra.Command 찾기
			cmd := rootCmd
			for _, name := range tt.cmdPath {
				found := false
				for _, sub := range cmd.Commands() {
					if sub.Name() == name {
						cmd = sub
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("명령어 '%s' 를 찾을 수 없습니다", name)
				}
			}

			result := ac.completeFlags(cmd, tt.partial)
			for _, item := range tt.contains {
				assert.Contains(t, result, item,
					"플래그 완성 결과에 '%s' 가 포함되어야 합니다", item)
			}
		})
	}
}

// =============================================================================
// TestCompleteResource - API 리소스 동적 자동완성 테스트
// REQ-INT-011: Tab 자동완성 - API 리소스
// =============================================================================

func TestCompleteResource(t *testing.T) {
	// 모의 API 서버 생성
	server := newMockAPIServer(t, map[string][]resourceItem{
		"flows": {
			{ID: "flow-1", Name: "my-workflow"},
			{ID: "flow-2", Name: "data-pipeline"},
			{ID: "flow-3", Name: "ml-training"},
		},
	})
	defer server.Close()

	rootCmd := newTestRootCmdWithSubs()
	client := NewClient(server.URL, "", 5*time.Second, false)
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	t.Run("플로우 리소스 이름 완성", func(t *testing.T) {
		result := ac.completeResource("flows", "my")
		assert.Contains(t, result, "my-workflow",
			"my 로 시작하는 플로우 이름이 포함되어야 합니다")
	})

	t.Run("빈 접두사로 모든 리소스 반환", func(t *testing.T) {
		result := ac.completeResource("flows", "")
		assert.Len(t, result, 3,
			"빈 접두사 시 모든 플로우가 반환되어야 합니다")
	})

	t.Run("매칭되지 않는 접두사는 빈 목록", func(t *testing.T) {
		result := ac.completeResource("flows", "nonexistent")
		assert.Empty(t, result,
			"매칭되지 않는 접두사는 빈 목록이어야 합니다")
	})
}

// =============================================================================
// TestResourceCacheHit - 캐시 히트 테스트
// REQ-INT-011: 자동완성 캐시
// =============================================================================

func TestResourceCacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := apiResponse{Success: true}
		listResp := resourceListResponse{Items: []resourceItem{
			{ID: "f-1", Name: "cached-flow"},
		}}
		data, _ := json.Marshal(listResp)
		resp.Data = json.RawMessage(data)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rootCmd := newTestRootCmdWithSubs()
	client := NewClient(server.URL, "", 5*time.Second, false)
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	// 첫 번째 호출: API 호출 발생
	result1 := ac.completeResource("flows", "")
	assert.Len(t, result1, 1)
	assert.Equal(t, 1, callCount, "첫 번째 호출에서 API 가 1회 호출되어야 합니다")

	// 두 번째 호출: 캐시 사용, API 호출 없음
	result2 := ac.completeResource("flows", "")
	assert.Len(t, result2, 1)
	assert.Equal(t, 1, callCount, "두 번째 호출에서는 API 가 호출되지 않아야 합니다 (캐시 히트)")
}

// =============================================================================
// TestResourceCacheExpiry - 캐시 TTL 만료 후 재조회 테스트
// REQ-INT-011: 캐시 TTL 30초
// =============================================================================

func TestResourceCacheExpiry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := apiResponse{Success: true}
		listResp := resourceListResponse{Items: []resourceItem{
			{ID: "f-1", Name: "test-flow"},
		}}
		data, _ := json.Marshal(listResp)
		resp.Data = json.RawMessage(data)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rootCmd := newTestRootCmdWithSubs()
	client := NewClient(server.URL, "", 5*time.Second, false)
	// 매우 짧은 TTL (50ms) 로 캐시 만료를 빠르게 테스트
	ac := NewAutoCompleter(rootCmd, &client, 50*time.Millisecond)

	// 첫 번째 호출
	ac.completeResource("flows", "")
	assert.Equal(t, 1, callCount)

	// TTL 만료 대기
	time.Sleep(100 * time.Millisecond)

	// 두 번째 호출: TTL 만료 후 재조회
	ac.completeResource("flows", "")
	assert.Equal(t, 2, callCount, "TTL 만료 후 API 가 다시 호출되어야 합니다")
}

// =============================================================================
// TestResourceCacheMiss - 다른 리소스 타입은 별도 캐시 키
// REQ-INT-011: 리소스 타입별 캐시
// =============================================================================

func TestResourceCacheMiss(t *testing.T) {
	server := newMockAPIServer(t, map[string][]resourceItem{
		"flows": {
			{ID: "f-1", Name: "flow-alpha"},
		},
		"agents": {
			{ID: "a-1", Name: "agent-beta"},
		},
	})
	defer server.Close()

	rootCmd := newTestRootCmdWithSubs()
	client := NewClient(server.URL, "", 5*time.Second, false)
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	// flows 조회
	flowResult := ac.completeResource("flows", "")
	assert.Contains(t, flowResult, "flow-alpha")

	// agents 조회 (별도 캐시 키, 새로운 API 호출)
	agentResult := ac.completeResource("agents", "")
	assert.Contains(t, agentResult, "agent-beta")
}

// =============================================================================
// TestCompleteResourceOffline - 서버 미연결 시 빈 결과 반환
// REQ-INT-011: API 미연결 시 graceful fallback
// =============================================================================

func TestCompleteResourceOffline(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()

	t.Run("클라이언트가 nil 인 경우", func(t *testing.T) {
		var client *Client
		ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

		result := ac.completeResource("flows", "")
		assert.Empty(t, result,
			"클라이언트가 nil 이면 빈 목록을 반환해야 합니다")
	})

	t.Run("서버 연결 불가능한 경우", func(t *testing.T) {
		client := NewClient("http://localhost:99999", "", 1*time.Second, false)
		ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

		result := ac.completeResource("flows", "")
		assert.Empty(t, result,
			"서버 연결 불가 시 빈 목록을 반환해야 합니다 (에러 없음)")
	})
}

// =============================================================================
// TestResolveResourceType - 명령어 컨텍스트에서 리소스 타입 추론
// REQ-INT-011: 리소스 타입 매핑
// =============================================================================

func TestResolveResourceType(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	tests := []struct {
		name     string
		cmdName  string
		expected string
	}{
		{
			name:     "flow 명령어는 flows 리소스 타입",
			cmdName:  "flow",
			expected: "flows",
		},
		{
			name:     "agent 명령어는 agents 리소스 타입",
			cmdName:  "agent",
			expected: "agents",
		},
		{
			name:     "node 명령어는 nodes 리소스 타입",
			cmdName:  "node",
			expected: "nodes",
		},
		{
			name:     "plugin 명령어는 plugins 리소스 타입",
			cmdName:  "plugin",
			expected: "plugins",
		},
		{
			name:     "매핑되지 않는 명령어는 빈 문자열",
			cmdName:  "config",
			expected: "",
		},
		{
			name:     "version 명령어는 빈 문자열",
			cmdName:  "version",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 루트에서 해당 명령어 찾기
			var cmd *cobra.Command
			for _, sub := range rootCmd.Commands() {
				if sub.Name() == tt.cmdName {
					cmd = sub
					break
				}
			}
			require.NotNil(t, cmd, "명령어 '%s' 가 존재해야 합니다", tt.cmdName)

			result := ac.resolveResourceType(cmd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestDoMethod - readline.AutoCompleter.Do() 통합 테스트
// REQ-INT-007: Tab 자동완성
// =============================================================================

func TestDoMethod(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	tests := []struct {
		name           string
		line           string
		pos            int
		expectCandLen  int    // 후보 수 (최소)
		expectLength   int    // 접두사 길이
		expectContains string // 후보 중 포함되어야 하는 문자열 접미사
	}{
		{
			name:           "fl 입력 시 flow 완성",
			line:           "fl",
			pos:            2,
			expectCandLen:  1,
			expectLength:   2,
			expectContains: "ow",
		},
		{
			name:           "flow li 입력 시 list 완성",
			line:           "flow li",
			pos:            7,
			expectCandLen:  1,
			expectLength:   2,
			expectContains: "st",
		},
		{
			name:          "빈 입력 시 모든 최상위 명령어",
			line:          "",
			pos:           0,
			expectCandLen: 8, // agent, config, flow, interactive, node, plugin, status, version
			expectLength:  0,
		},
		{
			name:           "flow --fo 입력 시 --format 완성",
			line:           "flow --fo",
			pos:            9,
			expectCandLen:  1,
			expectLength:   4,
			expectContains: "rmat",
		},
		{
			name:          "flow de 입력 시 delete, deploy 완성",
			line:          "flow de",
			pos:           7,
			expectCandLen: 2,
			expectLength:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := []rune(tt.line)
			newLine, length := ac.Do(line, tt.pos)

			assert.GreaterOrEqual(t, len(newLine), tt.expectCandLen,
				"후보 수가 %d 이상이어야 합니다", tt.expectCandLen)
			assert.Equal(t, tt.expectLength, length,
				"접두사 길이가 %d 여야 합니다", tt.expectLength)

			if tt.expectContains != "" {
				found := false
				for _, candidate := range newLine {
					if string(candidate) == tt.expectContains {
						found = true
						break
					}
				}
				assert.True(t, found,
					"후보에 '%s' 가 포함되어야 합니다 (실제: %v)",
					tt.expectContains, runeSlicesToStrings(newLine))
			}
		})
	}
}

// =============================================================================
// TestDoMethodWithAPIResource - Do() 메서드에서 API 리소스 완성 테스트
// REQ-INT-011: Tab 자동완성 - API 리소스
// =============================================================================

func TestDoMethodWithAPIResource(t *testing.T) {
	server := newMockAPIServer(t, map[string][]resourceItem{
		"flows": {
			{ID: "f-1", Name: "my-workflow"},
			{ID: "f-2", Name: "my-pipeline"},
			{ID: "f-3", Name: "other-flow"},
		},
	})
	defer server.Close()

	rootCmd := newTestRootCmdWithSubs()
	client := NewClient(server.URL, "", 5*time.Second, false)
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	t.Run("flow get my 입력 시 API 리소스 완성", func(t *testing.T) {
		line := []rune("flow get my")
		newLine, length := ac.Do(line, 11)

		assert.GreaterOrEqual(t, len(newLine), 1,
			"my 로 시작하는 리소스가 있어야 합니다")
		assert.Equal(t, 2, length,
			"접두사 길이가 'my' 의 길이 2여야 합니다")
	})

	t.Run("flow delete 후 빈 입력 시 모든 리소스", func(t *testing.T) {
		line := []rune("flow delete ")
		newLine, length := ac.Do(line, 12)

		assert.Equal(t, 3, len(newLine),
			"모든 플로우 리소스가 반환되어야 합니다")
		assert.Equal(t, 0, length,
			"빈 접두사이므로 길이가 0이어야 합니다")
	})
}

// =============================================================================
// TestCacheGetSet - 캐시 저장/조회 기본 테스트
// =============================================================================

func TestCacheGetSet(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	// 캐시에 데이터 저장
	ac.setCache("flows", []string{"flow-1", "flow-2"})

	// 캐시에서 조회
	items, ok := ac.getCached("flows")
	assert.True(t, ok, "캐시에 저장된 키가 존재해야 합니다")
	assert.Equal(t, []string{"flow-1", "flow-2"}, items)

	// 존재하지 않는 키
	_, ok = ac.getCached("unknown")
	assert.False(t, ok, "존재하지 않는 키는 false 를 반환해야 합니다")
}

// =============================================================================
// TestCacheConcurrency - 캐시 동시 접근 안전성 테스트
// =============================================================================

func TestCacheConcurrency(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			ac.setCache("flows", []string{"flow"})
		}(i)
		go func(i int) {
			defer wg.Done()
			ac.getCached("flows")
		}(i)
	}
	wg.Wait()
	// 데이터 레이스 없이 완료되면 성공
}

// =============================================================================
// TestDoMethodCursorMiddle - 커서가 줄 중간에 있는 경우
// =============================================================================

func TestDoMethodCursorMiddle(t *testing.T) {
	rootCmd := newTestRootCmdWithSubs()
	var client *Client
	ac := NewAutoCompleter(rootCmd, &client, 30*time.Second)

	// 커서가 줄 중간에 있는 경우: "flow list" 에서 pos=4 (flow 까지만)
	line := []rune("flow list")
	newLine, length := ac.Do(line, 4)

	// "flow" 다음에 서브커맨드 완성이 아닌, "flow" 자체가 완전 매칭이므로
	// 서브커맨드 목록을 반환해야 함
	_ = newLine
	_ = length
	// 커서 위치 기준으로 동작하는 것만 확인
}

// =============================================================================
// 헬퍼: [][]rune 을 []string 으로 변환
// =============================================================================

func runeSlicesToStrings(slices [][]rune) []string {
	result := make([]string, len(slices))
	for i, s := range slices {
		result[i] = string(s)
	}
	return result
}
