package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoteTokenCmd 는 remote token 서브커맨드 그룹을 생성한다(SPEC-CLI-004 P3b).
// 등록 토큰 API(/api/v1/remote/enrollment-tokens)를 CLI 로 노출한다.
//
// 하위 커맨드:
//
//	create  토큰 발급(원본 토큰 1회 노출)
//	list    토큰 메타데이터 목록(토큰/해시 미노출)
//	revoke  토큰 폐기(파괴적)
//
// confirmFn 은 revoke(폐기)의 확인 프롬프트에 사용된다(remote node revoke 패턴 참고).
func newRemoteTokenCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "등록 토큰 관리",
		Long:  "원격 노드 등록(enrollment) 토큰의 발급, 목록 조회, 폐기를 수행합니다.",
	}

	cmd.AddCommand(newRemoteTokenCreateCmd(client))
	cmd.AddCommand(newRemoteTokenListCmd(client))
	cmd.AddCommand(newRemoteTokenRevokeCmd(client, confirmFn))

	return cmd
}

// remoteTokenCreateFlagSet 는 token create 의 플래그 집합을 정의한다.
// 커맨드 생성과 단위 테스트(newRemoteTokenCreateCmdForTest)에서 공유한다.
func remoteTokenCreateFlagSet(cmd *cobra.Command) {
	cmd.Flags().String("label", "", "토큰 라벨 (선택)")
	cmd.Flags().String("expires-in", "", "만료 기간 (Go duration, 예: 24h)")
	cmd.Flags().Int("max-uses", 0, "최대 사용 횟수 (0=무제한)")
}

// buildTokenCreateBodyFromCmd 는 token create 플래그를 요청 본문으로 변환한다.
// 명시적으로 설정된(Changed) 플래그만 본문에 포함하여 서버 측 omitempty 의도와 맞춘다
// (device metadata buildMetadataBody 패턴 참고).
func buildTokenCreateBodyFromCmd(cmd *cobra.Command) map[string]any {
	body := map[string]any{}

	if cmd.Flags().Changed("label") {
		v, _ := cmd.Flags().GetString("label")
		body["label"] = v
	}
	if cmd.Flags().Changed("expires-in") {
		v, _ := cmd.Flags().GetString("expires-in")
		body["expires_in"] = v
	}
	if cmd.Flags().Changed("max-uses") {
		v, _ := cmd.Flags().GetInt("max-uses")
		body["max_uses"] = v
	}

	return body
}

// newRemoteTokenCreateCmdForTest 는 token create 플래그만 구성한 커맨드를 생성한다.
// buildTokenCreateBodyFromCmd 단위 테스트에서 플래그 파싱용으로 사용한다.
func newRemoteTokenCreateCmdForTest() *cobra.Command {
	cmd := &cobra.Command{Use: "create"}
	remoteTokenCreateFlagSet(cmd)
	return cmd
}

// remoteTokenCreatedFieldOrder 는 토큰 발급 응답 상세 출력의 필드 순서이다.
var remoteTokenCreatedFieldOrder = []string{
	"id", "token", "label", "expires_at", "max_uses",
}

// remoteTokenCreatedLabelMap 는 토큰 발급 응답 상세 출력의 필드 라벨 매핑이다.
var remoteTokenCreatedLabelMap = map[string]string{
	"id":         "ID",
	"token":      "Token",
	"label":      "Label",
	"expires_at": "Expires At",
	"max_uses":   "Max Uses",
}

// newRemoteTokenCreateCmd 는 remote token create 서브커맨드를 생성한다.
// POST /api/v1/remote/enrollment-tokens 로 등록 토큰을 발급한다.
//
// 응답은 EnrollmentTokenCreatedDTO {id, token, label?, expires_at?, max_uses?} 이다.
// 보안: 원본 token 은 발급 시 1회만 노출되므로 명확히 출력하고, 이후 조회 불가임을
// 사용자에게 안내한다(토큰 값을 다른 곳에 로깅하지 않는다 — REQ-H06).
func newRemoteTokenCreateCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "등록 토큰 발급",
		Long: `원격 노드 등록 토큰을 발급합니다.

발급된 원본 토큰은 이 응답에서 한 번만 노출되며, 이후에는 다시 조회할 수 없습니다.

예시:
  xflow remote token create
  xflow remote token create --label edge-fleet --expires-in 24h --max-uses 5`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := buildTokenCreateBodyFromCmd(cmd)

			var result map[string]any
			if err := (*client).Post("/api/v1/remote/enrollment-tokens", body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				df := NewDetailFormatter(remoteTokenCreatedFieldOrder, remoteTokenCreatedLabelMap, nil)
				if err := df.Format(result, w); err != nil {
					return err
				}
				fmt.Fprintln(w, "\n주의: 위 토큰은 지금 한 번만 표시됩니다. 안전한 곳에 보관하세요. 이후에는 다시 표시되지 않습니다.")
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	remoteTokenCreateFlagSet(cmd)

	return cmd
}

// remoteTokenTableHeaders 는 토큰 목록 테이블의 헤더이다.
var remoteTokenTableHeaders = []string{
	"ID", "LABEL", "CREATED_AT", "EXPIRES_AT", "USES", "MAX_USES", "REVOKED",
}

// remoteTokenRowFunc 는 토큰 메타 맵에서 테이블 행을 추출한다.
// created_at/expires_at(epoch ms)은 정수 문자열로, revoked(bool)은 yes/no 로 렌더링한다.
// expires_at/max_uses 는 nullable 이므로 없으면 빈 문자열로 표시한다.
func remoteTokenRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["id"]),
		nullableString(m["label"]),
		formatEpochValue(m["created_at"]),
		formatEpochValue(m["expires_at"]),
		formatEpochValue(m["uses"]),
		formatEpochValue(m["max_uses"]),
		boolToYesNo(m["revoked"]),
	}
}

// nullableString 은 nil 이면 빈 문자열, 아니면 %v 문자열을 반환한다.
func nullableString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// newRemoteTokenListCmd 는 remote token list 서브커맨드를 생성한다.
// GET /api/v1/remote/enrollment-tokens 로 토큰 메타데이터 목록을 조회한다.
//
// 응답은 토큰 메타 배열({id, label?, created_at, expires_at?, max_uses?, uses, revoked})
// 이며 []map[string]any 로 디코딩한다. 토큰 원본/해시는 노출되지 않는다.
func newRemoteTokenListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "등록 토큰 목록 조회",
		Long: `등록 토큰의 메타데이터 목록을 조회합니다(토큰 원본은 노출되지 않습니다).

예시:
  xflow remote token list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var tokens []map[string]any
			if err := (*client).Get("/api/v1/remote/enrollment-tokens", &tokens); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			return PrintResult(w, format, tokens, remoteTokenTableHeaders, remoteTokenRowFunc)
		},
	}
}

// newRemoteTokenRevokeCmd 는 remote token revoke 서브커맨드를 생성한다.
// DELETE /api/v1/remote/enrollment-tokens/{id} 로 토큰을 폐기한다(204).
//
// 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
func newRemoteTokenRevokeCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "등록 토큰 폐기",
		Long: `등록 토큰을 폐기합니다(파괴적 작업).

예시:
  xflow remote token revoke tok-01
  xflow remote token revoke tok-01 --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return ErrInvalidInput("토큰 ID 를 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("등록 토큰 '%s' 를 폐기하시겠습니까?", id)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			// 204 No Content: 응답 본문이 없으므로 디코딩 대상을 nil 로 둔다.
			path := fmt.Sprintf("/api/v1/remote/enrollment-tokens/%s", url.PathEscape(id))
			if err := (*client).Delete(path, nil); err != nil {
				return err
			}

			fmt.Fprintf(w, "등록 토큰 '%s' 폐기됨\n", id)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}
