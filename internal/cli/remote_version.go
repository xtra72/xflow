package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoteVersionCmd 는 remote version 서브커맨드 그룹을 생성한다(SPEC-CLI-004 P3c).
// 타겟 버전·업데이트 소스·노드 버전 이력/업데이트 API 를 CLI 로 노출한다.
//
// 하위 그룹/커맨드:
//
//	target get/set   전역 타겟 버전 조회/설정
//	source get/set   업데이트 소스(URL/채널) 조회/설정
//	history          노드별 버전 변경 이력 조회
//	update           노드 단위 원격 자가 업데이트(파괴적)
//
// confirmFn 은 update(노드 자가 업데이트) 같은 파괴적 작업의 확인 프롬프트에 사용된다
// (remote node revoke 패턴 참고).
func newRemoteVersionCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "타겟 버전·업데이트 소스·노드 버전 관리",
		Long:  "전역 타겟 버전, 업데이트 소스(URL/채널), 노드별 버전 이력 조회 및 노드 단위 업데이트를 수행합니다.",
	}

	cmd.AddCommand(newRemoteVersionTargetCmd(client))
	cmd.AddCommand(newRemoteVersionSourceCmd(client))
	cmd.AddCommand(newRemoteVersionHistoryCmd(client))
	cmd.AddCommand(newRemoteVersionUpdateCmd(client, confirmFn))

	return cmd
}

// newRemoteVersionTargetCmd 는 remote version target 서브그룹을 생성한다.
// get / set 하위 커맨드로 전역 타겟 버전을 조회/설정한다.
func newRemoteVersionTargetCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target",
		Short: "전역 타겟 버전 조회/설정",
		Long:  "원격 노드가 따라야 할 전역 타겟 버전을 조회하거나 설정합니다.",
	}

	cmd.AddCommand(newRemoteVersionTargetGetCmd(client))
	cmd.AddCommand(newRemoteVersionTargetSetCmd(client))

	return cmd
}

// newRemoteVersionTargetGetCmd 는 remote version target get 서브커맨드를 생성한다.
// GET /api/v1/remote/target-version 로 전역 타겟 버전을 조회한다.
//
// 응답은 {version} 이며, 빈 version 은 "(미설정)"으로 표시한다.
func newRemoteVersionTargetGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "전역 타겟 버전 조회",
		Long: `전역 타겟 버전을 조회합니다.

예시:
  xflow remote version target get`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]any
			if err := (*client).Get("/api/v1/remote/target-version", &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				v := fmt.Sprintf("%v", result["version"])
				if result["version"] == nil || v == "" {
					v = "(미설정)"
				}
				fmt.Fprintf(w, "타겟 버전: %s\n", v)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteVersionTargetSetCmd 는 remote version target set 서브커맨드를 생성한다.
// PUT /api/v1/remote/target-version 로 전역 타겟 버전을 설정한다.
//
// 요청 본문은 {"version":"<version>"} 이며 응답은 {version} 을 에코한다.
// 참고: 서버는 빈 version 을 "미설정"으로 처리하므로 ""을 전달하면 타겟 버전이 해제된다.
func newRemoteVersionTargetSetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "set <version>",
		Short: "전역 타겟 버전 설정",
		Long: `전역 타겟 버전을 설정합니다.

빈 문자열("")을 전달하면 타겟 버전이 해제됩니다(서버가 미설정으로 처리).

예시:
  xflow remote version target set v0.19.0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// version 은 trim 하지 않는다: 빈 문자열은 의도적 해제 의미로 그대로 전달한다.
			version := args[0]

			body := map[string]any{"version": version}

			var result map[string]any
			if err := (*client).Put("/api/v1/remote/target-version", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				if version == "" {
					fmt.Fprintln(w, "타겟 버전을 해제했습니다.")
				} else {
					fmt.Fprintf(w, "타겟 버전을 '%s' 로 설정했습니다.\n", version)
				}
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteVersionSourceCmd 는 remote version source 서브그룹을 생성한다.
// get / set 하위 커맨드로 업데이트 소스(URL/채널)를 조회/설정한다.
func newRemoteVersionSourceCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "업데이트 소스 조회/설정",
		Long:  "원격 업데이트 소스(URL 및 채널)를 조회하거나 설정합니다.",
	}

	cmd.AddCommand(newRemoteVersionSourceGetCmd(client))
	cmd.AddCommand(newRemoteVersionSourceSetCmd(client))

	return cmd
}

// newRemoteVersionSourceGetCmd 는 remote version source get 서브커맨드를 생성한다.
// GET /api/v1/remote/update-source 로 업데이트 소스를 조회한다.
//
// 응답은 {update_url, channel?} 이며, 빈 update_url 은 "(미설정)"으로 표시한다.
func newRemoteVersionSourceGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "업데이트 소스 조회",
		Long: `업데이트 소스(URL 및 채널)를 조회합니다.

예시:
  xflow remote version source get`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var result map[string]any
			if err := (*client).Get("/api/v1/remote/update-source", &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				u := fmt.Sprintf("%v", result["update_url"])
				if result["update_url"] == nil || u == "" {
					u = "(미설정)"
				}
				fmt.Fprintf(w, "업데이트 URL: %s\n", u)
				// channel 은 nullable 이므로 존재할 때만 표시한다.
				if ch := nullableString(result["channel"]); ch != "" {
					fmt.Fprintf(w, "채널:         %s\n", ch)
				}
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}
}

// newRemoteVersionSourceSetCmd 는 remote version source set 서브커맨드를 생성한다.
// PUT /api/v1/remote/update-source 로 업데이트 소스를 설정한다.
//
// 요청 본문은 {"update_url":"<url>","channel":"<ch>"} 이며 channel 은 명시적으로
// 설정된(Changed) 경우에만 포함한다. URL 검증(빈 값 또는 https:// 만 허용)은 서버가
// 수행하며, 클라이언트의 MapAPIError 가 검증 실패를 surface 한다.
func newRemoteVersionSourceSetCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <url>",
		Short: "업데이트 소스 설정",
		Long: `업데이트 소스(URL 및 채널)를 설정합니다.

URL 은 빈 값이거나 https:// 로 시작해야 하며, 검증은 서버가 수행합니다.

예시:
  xflow remote version source set https://updates.example.com
  xflow remote version source set https://updates.example.com --channel stable`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// url 은 trim 하지 않는다: 빈 문자열은 의도적 해제 의미로 그대로 전달한다.
			updateURL := args[0]

			body := map[string]any{"update_url": updateURL}
			// channel 은 명시적으로 설정된 경우에만 포함한다(서버 측 omitempty 의도와 일치).
			if cmd.Flags().Changed("channel") {
				v, _ := cmd.Flags().GetString("channel")
				body["channel"] = v
			}

			var result map[string]any
			if err := (*client).Put("/api/v1/remote/update-source", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				if updateURL == "" {
					fmt.Fprintln(w, "업데이트 소스를 해제했습니다.")
				} else {
					fmt.Fprintf(w, "업데이트 소스를 '%s' 로 설정했습니다.\n", updateURL)
				}
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().String("channel", "", "릴리스 채널 (예: stable)")

	return cmd
}

// remoteVersionHistoryHeaders 는 노드 버전 이력 테이블의 헤더이다.
var remoteVersionHistoryHeaders = []string{"VERSION", "CHANGED_AT"}

// remoteVersionHistoryRowFunc 는 버전 이력 맵에서 테이블 행을 추출한다.
// changed_at(epoch ms)은 정수 문자열로 렌더링한다.
func remoteVersionHistoryRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["version"]),
		formatEpochValue(m["changed_at"]),
	}
}

// newRemoteVersionHistoryCmd 는 remote version history 서브커맨드를 생성한다.
// GET /api/v1/remote/nodes/{instance_id}/version-history?limit=N 로 노드별 버전 이력을 조회한다.
//
// 응답은 버전 이력 객체 배열([{version, changed_at}])이며 []map[string]any 로 디코딩한다.
// --limit 은 Changed 이고 0 보다 클 때만 query 파라미터로 추가한다(device history 패턴 참고).
func newRemoteVersionHistoryCmd(client **Client) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history <instance_id>",
		Short: "노드 버전 변경 이력 조회",
		Long: `노드의 버전 변경 이력을 최신순으로 조회합니다.

예시:
  xflow remote version history node-01
  xflow remote version history node-01 --limit 50`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			path := fmt.Sprintf("/api/v1/remote/nodes/%s/version-history", url.PathEscape(id))
			if cmd.Flags().Changed("limit") && limit > 0 {
				query := url.Values{}
				query.Set("limit", strconv.Itoa(limit))
				path += "?" + query.Encode()
			}

			var entries []map[string]any
			if err := (*client).Get(path, &entries); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, entries, remoteVersionHistoryHeaders, remoteVersionHistoryRowFunc)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "조회할 최대 이력 개수")

	return cmd
}

// newRemoteVersionUpdateCmd 는 remote version update 서브커맨드를 생성한다.
// POST /api/v1/remote/nodes/{instance_id}/update 로 노드 단위 원격 자가 업데이트를 트리거한다.
//
// 요청 본문은 {"version":..,"channel":..,"restart":..} 이며 명시적으로 설정된(Changed)
// 플래그만 포함한다(device metadata buildMetadataBody 패턴 참고). 노드 자가 업데이트를
// 유발하는 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
// 응답은 {instance_id, target_version, result} 이다.
func newRemoteVersionUpdateCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "update <instance_id>",
		Short: "노드 단위 원격 업데이트",
		Long: `노드를 원격으로 자가 업데이트합니다(파괴적 작업).

예시:
  xflow remote version update node-01 --version v0.19.0 --restart
  xflow remote version update node-01 --channel stable --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("instance_id 를 지정해야 합니다")
			}

			// 명시적으로 설정된 플래그만 본문에 포함한다(서버 측 omitempty 의도와 일치).
			body := map[string]any{}
			if cmd.Flags().Changed("version") {
				v, _ := cmd.Flags().GetString("version")
				body["version"] = v
			}
			if cmd.Flags().Changed("channel") {
				v, _ := cmd.Flags().GetString("channel")
				body["channel"] = v
			}
			if cmd.Flags().Changed("restart") {
				v, _ := cmd.Flags().GetBool("restart")
				body["restart"] = v
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("노드 '%s' 를 업데이트하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/nodes/%s/update", url.PathEscape(id))
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "노드 '%s' 업데이트 요청됨 (타겟: %v, 결과: %v)\n",
					id, result["target_version"], result["result"])
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().String("version", "", "업데이트할 버전 (예: v0.19.0)")
	cmd.Flags().String("channel", "", "릴리스 채널 (예: stable)")
	cmd.Flags().Bool("restart", false, "업데이트 후 재시작")
	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}
