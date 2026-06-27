package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoteGroupCmd 는 remote group 서브커맨드 그룹을 생성한다(SPEC-CLI-004 P3b).
// 원격 노드 그룹 관리 API(/api/v1/remote/groups, /api/v1/remote/nodes/{id}/group)를
// CLI 로 노출한다.
//
// 하위 커맨드:
//
//	list     그룹 목록 + 노드 수 조회
//	set      노드를 그룹에 배정/변경
//	clear    노드의 그룹 해제("전체" 환원)
//	rename   그룹 일괄 이름변경
//	delete   그룹 삭제(멤버 "전체" 이동, 파괴적)
//	update   그룹 단위 원격 업데이트(fleet update, 파괴적)
//	command  그룹 단위 임의 명령 디스패치
//
// confirmFn 은 delete/update 같은 파괴적 작업의 확인 프롬프트에 사용된다
// (remote node revoke 패턴 참고).
func newRemoteGroupCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "원격 노드 그룹 관리",
		Long:  "원격 노드의 그룹 배정/해제, 그룹 이름변경/삭제, 그룹 단위 업데이트/명령을 수행합니다.",
	}

	cmd.AddCommand(newRemoteGroupListCmd(client))
	cmd.AddCommand(newRemoteGroupSetCmd(client))
	cmd.AddCommand(newRemoteGroupClearCmd(client))
	cmd.AddCommand(newRemoteGroupRenameCmd(client))
	cmd.AddCommand(newRemoteGroupDeleteCmd(client, confirmFn))
	cmd.AddCommand(newRemoteGroupUpdateCmd(client, confirmFn))
	cmd.AddCommand(newRemoteGroupCommandCmd(client))

	return cmd
}

// remoteGroupTableHeaders 는 그룹 목록 테이블의 헤더이다.
var remoteGroupTableHeaders = []string{"GROUP", "NODE_COUNT"}

// remoteGroupRowFunc 는 그룹 맵에서 테이블 행을 추출한다.
// 빈 group_name 은 가상 "전체"(미지정) 버킷이므로 (all) 로 렌더링한다.
func remoteGroupRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", ""}
	}
	name := fmt.Sprintf("%v", m["group_name"])
	if name == "" {
		name = "(all)"
	}
	return []string{
		name,
		formatEpochValue(m["node_count"]),
	}
}

// newRemoteGroupListCmd 는 remote group list 서브커맨드를 생성한다.
// GET /api/v1/remote/groups 로 distinct 그룹 + 노드 수를 조회한다.
//
// 응답은 그룹 객체 배열({group_name, node_count})이며 []map[string]any 로 디코딩한다.
func newRemoteGroupListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "원격 노드 그룹 목록 조회",
		Long: `원격 노드 그룹 목록과 각 그룹의 노드 수를 조회합니다.

빈 그룹명은 그룹 미지정 노드를 의미하며 (all) 로 표시됩니다.

예시:
  xflow remote group list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var groups []map[string]any
			if err := (*client).Get("/api/v1/remote/groups", &groups); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, groups, remoteGroupTableHeaders, remoteGroupRowFunc)
		},
	}
}

// newRemoteGroupSetCmd 는 remote group set 서브커맨드를 생성한다.
// PUT /api/v1/remote/nodes/{instance_id}/group 로 노드를 그룹에 배정한다.
//
// 요청 본문은 {"group_name":"<name>"} 이며 응답은 {instance_id, group_name} 이다.
func newRemoteGroupSetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "set <instance_id> <group_name>",
		Short: "원격 노드를 그룹에 배정",
		Long: `원격 노드를 지정한 그룹에 배정하거나 그룹을 변경합니다.

예시:
  xflow remote group set node-01 1f`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}
			group := strings.TrimSpace(args[1])
			if group == "" {
				return ErrInvalidInput("group_name 을 지정해야 합니다")
			}

			body := map[string]any{"group_name": group}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/group", url.PathEscape(id))
			if err := (*client).Put(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 를 그룹 '%s' 에 배정했습니다.\n", id, group)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteGroupClearCmd 는 remote group clear 서브커맨드를 생성한다.
// DELETE /api/v1/remote/nodes/{instance_id}/group 로 노드의 그룹을 해제한다(204).
//
// 204 No Content 응답이므로 디코딩 대상은 비워둔다(client.Delete 가 빈 본문을 허용).
func newRemoteGroupClearCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "clear <instance_id>",
		Short: "원격 노드의 그룹 해제",
		Long: `원격 노드의 그룹을 해제하여 "전체"(미지정)로 환원합니다.

예시:
  xflow remote group clear node-01`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			// 204 No Content: 응답 본문이 없으므로 디코딩 대상을 nil 로 둔다.
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/group", url.PathEscape(id))
			if err := (*client).Delete(path, nil); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "노드 '%s' 의 그룹을 해제했습니다.\n", id)
			return nil
		},
	}
}

// newRemoteGroupRenameCmd 는 remote group rename 서브커맨드를 생성한다.
// PUT /api/v1/remote/groups/{group_name} 로 그룹을 일괄 이름변경한다.
//
// 요청 본문은 {"new_name":"<new>"} 이며 응답은 {group_name, moved} 이다.
func newRemoteGroupRenameCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <group_name> <new_name>",
		Short: "원격 노드 그룹 이름변경",
		Long: `그룹에 속한 모든 노드를 새 그룹명으로 일괄 이동합니다.

예시:
  xflow remote group rename 1f 1층`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			old := strings.TrimSpace(args[0])
			if old == "" {
				return ErrInvalidInput("group_name 을 지정해야 합니다")
			}
			newName := strings.TrimSpace(args[1])
			if newName == "" {
				return ErrInvalidInput("new_name 을 지정해야 합니다")
			}

			body := map[string]any{"new_name": newName}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/groups/%s", url.PathEscape(old))
			if err := (*client).Put(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "그룹 '%s' → '%s' 이름변경됨 (이동 노드 %v개)\n",
					old, newName, result["moved"])
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteGroupDeleteCmd 는 remote group delete 서브커맨드를 생성한다.
// DELETE /api/v1/remote/groups/{group_name} 로 그룹을 삭제한다(멤버 "전체" 이동).
//
// 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
// 응답은 {moved} 이다.
func newRemoteGroupDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <group_name>",
		Short: "원격 노드 그룹 삭제",
		Long: `그룹을 삭제하고 멤버 노드를 "전체"(미지정)로 이동합니다(파괴적 작업).

예시:
  xflow remote group delete 1f
  xflow remote group delete 1f --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return ErrInvalidInput("group_name 을 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("그룹 '%s' 를 삭제하시겠습니까? (멤버는 전체로 이동)", name)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/groups/%s", url.PathEscape(name))
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "그룹 '%s' 삭제됨 (이동 노드 %v개)\n", name, result["moved"])
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}

// remoteGroupUpdateFlagSet 는 group update 의 플래그 집합을 정의한다.
// 커맨드 생성과 단위 테스트(newRemoteGroupUpdateCmdForTest)에서 공유한다.
func remoteGroupUpdateFlagSet(cmd *cobra.Command) {
	cmd.Flags().String("version", "", "고정할 버전 (예: v0.19.0)")
	cmd.Flags().String("strategy", "", "버전 해석 전략 (latest|pin|per_arch)")
	cmd.Flags().String("channel", "", "릴리즈 채널 (예: stable)")
	cmd.Flags().Bool("restart", false, "업데이트 후 재시작")
	cmd.Flags().StringArray("version-by-arch", nil, `per_arch 전략용 "os/arch=version" (반복 지정 가능)`)
}

// buildGroupUpdateBodyFromCmd 는 group update 플래그를 요청 본문으로 변환한다.
// 명시적으로 설정된(Changed) 플래그만 본문에 포함하여 서버 측 omitempty 의도와 맞춘다
// (device metadata buildMetadataBody 패턴 참고). 플래그가 하나도 없으면 빈 맵을 반환하며,
// 이는 "채널 최신으로 업데이트"를 의미한다(서버가 허용).
func buildGroupUpdateBodyFromCmd(cmd *cobra.Command) (map[string]any, error) {
	body := map[string]any{}

	if cmd.Flags().Changed("version") {
		v, _ := cmd.Flags().GetString("version")
		body["version"] = v
	}
	if cmd.Flags().Changed("strategy") {
		v, _ := cmd.Flags().GetString("strategy")
		body["strategy"] = v
	}
	if cmd.Flags().Changed("channel") {
		v, _ := cmd.Flags().GetString("channel")
		body["channel"] = v
	}
	if cmd.Flags().Changed("restart") {
		v, _ := cmd.Flags().GetBool("restart")
		body["restart"] = v
	}
	if cmd.Flags().Changed("version-by-arch") {
		pairs, _ := cmd.Flags().GetStringArray("version-by-arch")
		vba := make(map[string]string, len(pairs))
		for _, p := range pairs {
			k, v, ok := strings.Cut(p, "=")
			if !ok || k == "" {
				return nil, ErrInvalidInput(fmt.Sprintf("잘못된 version-by-arch 형식: %q (os/arch=version 형식 필요)", p))
			}
			vba[k] = v
		}
		body["version_by_arch"] = vba
	}

	return body, nil
}

// newRemoteGroupUpdateCmdForTest 는 group update 플래그만 구성한 커맨드를 생성한다.
// buildGroupUpdateBodyFromCmd 단위 테스트에서 플래그 파싱용으로 사용한다.
func newRemoteGroupUpdateCmdForTest() *cobra.Command {
	cmd := &cobra.Command{Use: "update"}
	remoteGroupUpdateFlagSet(cmd)
	return cmd
}

// newRemoteGroupUpdateCmd 는 remote group update 서브커맨드를 생성한다.
// POST /api/v1/remote/groups/{group_name}/update 로 그룹 단위 fleet 업데이트를 트리거한다.
//
// 파괴적(전 멤버 업데이트) 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
// 응답은 {group_name, results} 이며 results(노드별 결과)를 출력한다.
func newRemoteGroupUpdateCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "update <group_name>",
		Short: "원격 노드 그룹 일괄 업데이트",
		Long: `그룹 내 승인·온라인 노드를 일괄 원격 업데이트합니다(파괴적 작업).

예시:
  xflow remote group update 1f --version v0.19.0 --restart
  xflow remote group update 1f --strategy latest --channel stable
  xflow remote group update 1f --strategy per_arch \
    --version-by-arch linux/amd64=v0.19.0 --version-by-arch linux/arm64=v0.19.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return ErrInvalidInput("group_name 을 지정해야 합니다")
			}

			body, err := buildGroupUpdateBodyFromCmd(cmd)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("그룹 '%s' 의 모든 노드를 업데이트하시겠습니까?", name)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/groups/%s/update", url.PathEscape(name))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			return printGroupDispatchResult(cmd, name, result)
		},
	}

	remoteGroupUpdateFlagSet(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}

// newRemoteGroupCommandCmd 는 remote group command 서브커맨드를 생성한다.
// POST /api/v1/remote/groups/{group_name}/command 로 그룹 단위 명령을 디스패치한다.
//
// 요청 본문은 {"domain":..,"action":..,"args":<rawjson>} 이며 domain/action 은 필수이다.
// --args 는 JSON 문자열로 받아 검증 후 그대로 전달한다(미지정 시 args 필드 생략).
// 응답은 {group_name, results} 이다.
func newRemoteGroupCommandCmd(client **Client) *cobra.Command {
	var (
		domain  string
		action  string
		rawArgs string
	)

	cmd := &cobra.Command{
		Use:   "command <group_name>",
		Short: "원격 노드 그룹 일괄 명령",
		Long: `그룹 내 승인·온라인 노드에 임의 명령을 일괄 디스패치합니다.

예시:
  xflow remote group command 1f --domain system --action ping
  xflow remote group command 1f --domain flow --action restart --args '{"flow_id":"f1"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return ErrInvalidInput("group_name 을 지정해야 합니다")
			}
			if strings.TrimSpace(domain) == "" {
				return ErrInvalidInput("--domain 은 필수입니다")
			}
			if strings.TrimSpace(action) == "" {
				return ErrInvalidInput("--action 은 필수입니다")
			}

			body := map[string]any{
				"domain": domain,
				"action": action,
			}
			// --args 는 JSON 으로 검증 후 그대로 전달한다(미지정 시 필드 생략).
			if rawArgs != "" {
				var parsed json.RawMessage
				if err := json.Unmarshal([]byte(rawArgs), &parsed); err != nil {
					return ErrInvalidInput(fmt.Sprintf("--args JSON 파싱 실패: %v", err))
				}
				body["args"] = parsed
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/groups/%s/command", url.PathEscape(name))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			return printGroupDispatchResult(cmd, name, result)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "명령 도메인 (필수, 예: system, flow)")
	cmd.Flags().StringVar(&action, "action", "", "명령 액션 (필수, 예: ping, restart)")
	cmd.Flags().StringVar(&rawArgs, "args", "", "명령 인자 (JSON 문자열, 선택)")

	return cmd
}

// printGroupDispatchResult 는 그룹 단위 디스패치(update/command) 결과를 출력한다.
// table/text 포맷에서는 그룹명 헤더 + 노드별 결과를, json/yaml 에서는 원본 응답을 출력한다.
func printGroupDispatchResult(cmd *cobra.Command, group string, result map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		fmt.Fprintf(w, "Group: %s\n\n", group)
		return PrintResult(w, "text", result["results"], nil, nil)
	}
	return PrintResult(w, format, result, nil, nil)
}
