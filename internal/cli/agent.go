package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newAgentCmd 는 에이전트 관리 커맨드 그룹을 생성한다.
// 7개의 서브커맨드 (list, get, create, start, stop, restart, delete) 를 포함한다.
func newAgentCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "에이전트 관리",
		Long:  "에이전트의 조회, 생성, 시작, 중지, 재시작, 삭제를 수행합니다.",
	}

	// 서브커맨드 등록
	cmd.AddCommand(newAgentListCmd(client))
	cmd.AddCommand(newAgentGetCmd(client))
	cmd.AddCommand(newAgentCreateCmd(client))
	cmd.AddCommand(newAgentStartCmd(client))
	cmd.AddCommand(newAgentStopCmd(client))
	cmd.AddCommand(newAgentRestartCmd(client))
	cmd.AddCommand(newAgentDeleteCmd(client, confirmFn))
	cmd.AddCommand(newAgentExportCmd(client))
	cmd.AddCommand(newAgentImportCmd(client))

	return cmd
}

// newAgentListCmd 는 에이전트 목록 조회 커맨드를 생성한다.
// GET /api/v1/agents
// --name 플래그로 이름 부분 일치 필터링을 지원한다.
func newAgentListCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "에이전트 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			var agents []map[string]any
			if err := (*client).Get("/api/v1/agents", &agents); err != nil {
				return err
			}

			agents = filterByName(agents, name, "name")

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			return PrintResult(w, format, agents,
				[]string{"ID", "NAME", "TYPE", "STATUS", "CONNECTED"},
				func(item any) []string {
					m, ok := item.(map[string]any)
					if !ok {
						return []string{"", "", "", "", ""}
					}
					return []string{
						fmt.Sprintf("%v", m["id"]),
						fmt.Sprintf("%v", m["name"]),
						fmt.Sprintf("%v", m["type"]),
						fmt.Sprintf("%v", m["status"]),
						fmt.Sprintf("%v", m["connected"]),
					}
				},
			)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "이름으로 필터링 (부분 일치)")

	return cmd
}

// newAgentGetCmd 는 에이전트 상세 조회 커맨드를 생성한다.
// GET /api/v1/agents/:id
// positional 인자 또는 --name 플래그로 에이전트를 지정할 수 있다.
func newAgentGetCmd(client **Client) *cobra.Command {
	var name string
	var detail string

	cmd := &cobra.Command{
		Use:   "get [id|name]",
		Short: "에이전트 상세 조회",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			// 상세 수준을 쿼리 파라미터로 전달
			path := "/api/v1/agents/" + id + "?detail=" + detail

			var agent map[string]any
			if err := (*client).Get(path, &agent); err != nil {
				return err
			}

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			// table 포맷이면 DetailFormatter 를 사용하여 구조화된 출력 제공
			if format == "table" {
				df := NewDetailFormatter(
					[]string{"id", "name", "type", "status", "uptime", "connected", "started_at", "created_at"},
					map[string]string{
						"id":          "ID",
						"name":        "Name",
						"type":        "Type",
						"status":      "Status",
						"uptime":      "Uptime",
						"connected":   "Connected",
						"started_at":  "Started At",
						"created_at":  "Created At",
						"health":      "Health",
						"stats":       "Stats",
						"config":      "Config",
						"shared_info": "Shared Info",
						"state":       "State",
					},
					map[string]bool{
						"health":      true,
						"stats":       true,
						"config":      true,
						"shared_info": true,
						"state":       true,
					},
				)
				return df.Format(agent, w)
			}
			return PrintResult(w, format, agent, nil, nil)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 조회")
	cmd.Flags().StringVar(&detail, "detail", "summary", "상세 수준 (summary, full)")

	return cmd
}

// newAgentCreateCmd 는 파일 기반 에이전트 생성 커맨드를 생성한다.
// POST /api/v1/agents
func newAgentCreateCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "에이전트 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return ErrInvalidInput("파일 경로를 지정해주세요 (-f 플래그)")
			}

			// 파일 읽기
			data, err := readFile(filePath)
			if err != nil {
				return err
			}

			// 파일 형식에 따라 파싱
			var body map[string]any
			fileFormat := detectFileFormat(filePath)

			switch fileFormat {
			case "yaml":
				if err := yaml.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("YAML 파싱 실패: %v", err))
				}
			case "json":
				if err := json.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("JSON 파싱 실패: %v", err))
				}
			default:
				// 기본적으로 JSON 으로 시도
				if err := json.Unmarshal(data, &body); err != nil {
					return ErrInvalidInput(fmt.Sprintf("파일 형식을 인식할 수 없습니다: %s", filePath))
				}
			}

			// API 호출
			var result map[string]any
			if err := (*client).Post("/api/v1/agents", body, &result); err != nil {
				return err
			}

			format, _ := cmd.Flags().GetString("format")
			w := cmd.OutOrStdout()

			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "에이전트 정의 파일 경로 (JSON/YAML)")

	return cmd
}

// newAgentStartCmd 는 에이전트 시작 커맨드를 생성한다.
// POST /api/v1/agents/:id/start
func newAgentStartCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "start", "에이전트 시작")
}

// newAgentStopCmd 는 에이전트 중지 커맨드를 생성한다.
// POST /api/v1/agents/:id/stop
func newAgentStopCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "stop", "에이전트 중지")
}

// newAgentRestartCmd 는 에이전트 재시작 커맨드를 생성한다.
// POST /api/v1/agents/:id/restart
func newAgentRestartCmd(client **Client) *cobra.Command {
	return newAgentLifecycleCmd(client, "restart", "에이전트 재시작")
}

// newAgentLifecycleCmd 는 에이전트 라이프사이클 (start/stop/restart) 커맨드의 공통 팩토리이다.
// POST /api/v1/agents/:id/{action}
// positional 인자 또는 --name 플래그로 에이전트를 지정할 수 있다.
func newAgentLifecycleCmd(client **Client, action, short string) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   action + " [id|name]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/%s", id, action)
			if err := (*client).Post(path, nil, &result); err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "에이전트 %s: %s 완료\n", id, short)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")

	return cmd
}

// newAgentDeleteCmd 는 에이전트 삭제 커맨드를 생성한다.
// DELETE /api/v1/agents/:id
// --yes 플래그로 확인 프롬프트를 건너뛸 수 있다.
// positional 인자 또는 --name 플래그로 에이전트를 지정할 수 있다.
func newAgentDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool
	var name string

	cmd := &cobra.Command{
		Use:   "delete [id|name]",
		Short: "에이전트 삭제",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()

			// --yes 플래그가 없으면 확인 요청
			if !yes {
				prompt := fmt.Sprintf("에이전트 '%s' 를 삭제하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "삭제가 취소되었습니다.")
					return nil
				}
			}

			// DELETE 요청
			var result map[string]any
			if err := (*client).Delete("/api/v1/agents/"+id, &result); err != nil {
				return err
			}

			fmt.Fprintf(w, "에이전트 '%s' 가 삭제되었습니다.\n", id)

			// 경고 메시지가 있으면 출력
			if result != nil {
				if warning, ok := result["warning"]; ok && warning != nil {
					fmt.Fprintf(w, "경고: %v\n", warning)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")
	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")

	return cmd
}

// agentRuntimeFields 는 내보내기 시 제거할 런타임 전용 필드 목록이다.
var agentRuntimeFields = []string{"id", "status", "connected", "uptime", "messages_in", "messages_out", "error_count"}

// stripRuntimeFields 는 에이전트 데이터에서 런타임 전용 필드를 제거한다.
func stripRuntimeFields(agent map[string]any) map[string]any {
	result := make(map[string]any, len(agent))
	for k, v := range agent {
		result[k] = v
	}
	for _, field := range agentRuntimeFields {
		delete(result, field)
	}
	return result
}

// sanitizeFileName 은 파일명으로 사용할 수 없는 문자를 하이픈으로 치환한다.
func sanitizeFileName(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	return re.ReplaceAllString(name, "-")
}

// newAgentExportCmd 는 에이전트를 파일로 내보내는 커맨드를 생성한다.
// GET /api/v1/agents/{id} 로 단일 에이전트를 조회하여 파일로 저장하거나,
// --all 플래그로 모든 에이전트를 일괄 내보낸다.
func newAgentExportCmd(client **Client) *cobra.Command {
	var outputPath string
	var all bool
	var exportFormat string
	var name string

	cmd := &cobra.Command{
		Use:   "export [id|name]",
		Short: "에이전트를 파일로 내보내기",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()

			// --all 플래그: 모든 에이전트 일괄 내보내기
			if all {
				if outputPath == "" {
					return fmt.Errorf("출력 디렉터리 경로(-o)를 지정해야 합니다")
				}

				// 출력 디렉터리 생성
				if err := os.MkdirAll(outputPath, 0755); err != nil {
					return fmt.Errorf("디렉터리 생성 실패: %w", err)
				}

				// 모든 에이전트 조회
				var agents []map[string]any
				if err := (*client).Get("/api/v1/agents", &agents); err != nil {
					return err
				}

				// 기본 형식: yaml
				if exportFormat == "" {
					exportFormat = "yaml"
				}

				for _, agent := range agents {
					cleaned := stripRuntimeFields(agent)
					name, _ := agent["name"].(string)
					if name == "" {
						name = fmt.Sprintf("%v", agent["id"])
					}

					// 파일명 생성
					ext := "." + exportFormat
					if exportFormat == "yaml" {
						ext = ".yaml"
					}
					fileName := sanitizeFileName(name) + ext
					filePath := filepath.Join(outputPath, fileName)

					// 직렬화
					var data []byte
					var err error
					switch exportFormat {
					case "json":
						data, err = json.MarshalIndent(cleaned, "", "  ")
						if err == nil {
							data = append(data, '\n')
						}
					default:
						data, err = yaml.Marshal(cleaned)
					}
					if err != nil {
						return fmt.Errorf("직렬화 실패 (%s): %w", name, err)
					}

					if err := os.WriteFile(filePath, data, 0644); err != nil {
						return fmt.Errorf("파일 쓰기 실패 (%s): %w", filePath, err)
					}
				}

				fmt.Fprintf(w, "%d개 에이전트를 %s 로 내보냈습니다.\n", len(agents), outputPath)
				return nil
			}

			// 단일 에이전트 내보내기
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return fmt.Errorf("에이전트 ID 또는 --name을 지정하거나 --all 플래그를 사용하세요")
			}

			if outputPath == "" {
				return fmt.Errorf("출력 파일 경로(-o)를 지정해야 합니다")
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}
			var agent map[string]any
			if err := (*client).Get("/api/v1/agents/"+id, &agent); err != nil {
				return err
			}

			// 런타임 필드 제거
			cleaned := stripRuntimeFields(agent)

			// 파일 확장자로 형식 자동 감지
			fileFormat := detectFileFormat(outputPath)
			var data []byte

			switch fileFormat {
			case "yaml":
				data, err = yaml.Marshal(cleaned)
			default:
				// 기본값: JSON
				data, err = json.MarshalIndent(cleaned, "", "  ")
				if err == nil {
					data = append(data, '\n')
				}
			}
			if err != nil {
				return fmt.Errorf("직렬화 실패: %w", err)
			}

			if err := os.WriteFile(outputPath, data, 0644); err != nil {
				return fmt.Errorf("파일 쓰기 실패: %w", err)
			}

			fmt.Fprintf(w, "에이전트 '%s' 를 %s 로 내보냈습니다.\n", id, outputPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "출력 파일/디렉터리 경로")
	cmd.Flags().BoolVar(&all, "all", false, "모든 에이전트 일괄 내보내기")
	cmd.Flags().StringVar(&exportFormat, "export-format", "yaml", "일괄 내보내기 형식 (json/yaml)")
	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")

	return cmd
}

// newAgentImportCmd 는 파일에서 에이전트를 가져오는 커맨드를 생성한다.
// 파일을 읽어 POST /api/v1/agents 로 에이전트를 생성한다.
// 디렉터리를 지정하면 내부의 모든 JSON/YAML 파일을 일괄 가져온다.
func newAgentImportCmd(client **Client) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "파일에서 에이전트 가져오기",
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("가져올 파일 경로(-f)를 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			// 경로가 디렉터리인지 확인
			info, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("경로 확인 실패: %w", err)
			}

			// 디렉터리인 경우: 일괄 가져오기
			if info.IsDir() {
				return importAgentsFromDir(client, filePath, cmd, w)
			}

			// 단일 파일 가져오기
			body, err := parseAgentFile(filePath)
			if err != nil {
				return err
			}

			request := buildAgentCreateRequest(body)

			var result map[string]any
			if err := (*client).Post("/api/v1/agents", request, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "가져올 에이전트 파일/디렉터리 경로 (JSON/YAML)")

	return cmd
}

// parseAgentFile 은 에이전트 파일을 읽고 파싱한다.
func parseAgentFile(path string) (map[string]any, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	var body map[string]any
	fileFormat := detectFileFormat(path)

	switch fileFormat {
	case "yaml":
		if err := yaml.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("YAML 파싱 실패: %w", err)
		}
	case "json":
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("JSON 파싱 실패: %w", err)
		}
	default:
		// 기본적으로 JSON 으로 시도
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("파일 형식을 인식할 수 없습니다: %s", path)
		}
	}

	return body, nil
}

// buildAgentCreateRequest 는 파싱된 데이터를 AgentCreateRequest 형식으로 래핑한다.
// "config" 키가 있으면 {name, type, config} 그대로 사용하고,
// 없으면 "name"과 "type"을 추출하고 나머지를 "config"에 넣는다.
func buildAgentCreateRequest(body map[string]any) map[string]any {
	if _, hasConfig := body["config"]; hasConfig {
		return map[string]any{
			"name":   body["name"],
			"type":   body["type"],
			"config": body["config"],
		}
	}

	// "name"과 "type"을 추출하고 나머지를 config 로 구성
	name := body["name"]
	typ := body["type"]
	config := make(map[string]any, len(body))
	for k, v := range body {
		if k != "name" && k != "type" {
			config[k] = v
		}
	}

	request := map[string]any{
		"name": name,
		"type": typ,
	}
	if len(config) > 0 {
		request["config"] = config
	}
	return request
}

// importAgentsFromDir 은 디렉터리 내의 JSON/YAML 파일을 순회하며 에이전트를 일괄 가져온다.
func importAgentsFromDir(client **Client, dirPath string, cmd *cobra.Command, w io.Writer) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("디렉터리 읽기 실패: %w", err)
	}

	var total, success, failure int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// JSON/YAML 파일만 처리
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			continue
		}

		total++
		entryPath := filepath.Join(dirPath, entry.Name())

		body, err := parseAgentFile(entryPath)
		if err != nil {
			fmt.Fprintf(w, "실패: %s - %v\n", entry.Name(), err)
			failure++
			continue
		}

		request := buildAgentCreateRequest(body)

		var result map[string]any
		if err := (*client).Post("/api/v1/agents", request, &result); err != nil {
			fmt.Fprintf(w, "실패: %s - %v\n", entry.Name(), err)
			failure++
			continue
		}

		success++
	}

	fmt.Fprintf(w, "%d개 에이전트 가져오기 완료 (성공: %d, 실패: %d)\n", total, success, failure)
	return nil
}
