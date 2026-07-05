package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newDashboardCmd 는 대시보드 설정 조회·저장 커맨드 그룹을 생성한다.
// 대시보드 API(GET/PUT /api/v1/dashboards/{shared,mine})를 CLI 로 노출한다.
//
// 서브커맨드 그룹:
//
//	shared  공유 대시보드 (scope=global)
//	mine    개인 대시보드 (scope=user, JWT username 으로 owner 결정)
//
// 각 그룹은 get / set 두 하위 커맨드를 포함한다.
// (DELETE 및 If-Match 동시성 제어는 본 P4 범위 밖이다.)
func newDashboardCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "대시보드 설정 조회·저장",
		Long:  "공유(shared) 및 개인(mine) 대시보드 스냅샷을 조회하거나 저장합니다.",
	}

	cmd.AddCommand(newDashboardSharedCmd(client))
	cmd.AddCommand(newDashboardMineCmd(client))

	return cmd
}

// dashboardDetailFieldOrder 는 대시보드 스냅샷 상세 출력의 필드 순서이다.
var dashboardDetailFieldOrder = []string{
	"scope", "owner", "version", "updatedAt", "payload",
}

// dashboardDetailLabelMap 은 대시보드 스냅샷 상세 출력의 필드 라벨 매핑이다.
var dashboardDetailLabelMap = map[string]string{
	"scope":     "Scope",
	"owner":     "Owner",
	"version":   "Version",
	"updatedAt": "Updated At",
	"payload":   "Payload",
}

// dashboardDetailSectionKeys 는 별도 섹션으로 출력할 대시보드 상세 키 목록이다.
var dashboardDetailSectionKeys = map[string]bool{
	"payload": true,
}

// printDashboardSnapshot 은 단일 대시보드 스냅샷 응답을 포맷에 맞게 출력한다.
// table/text 포맷에서는 updatedAt 을 epoch ms 정수 문자열로 정규화한 뒤
// DetailFormatter 로 렌더링하고, json/yaml 은 원본을 그대로 통과시킨다.
func printDashboardSnapshot(cmd *cobra.Command, snap map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		// updatedAt 은 epoch ms 이므로 과학표기 출력을 피하기 위해 정수 문자열로 정규화한다.
		view := make(map[string]any, len(snap))
		for k, v := range snap {
			view[k] = v
		}
		if _, ok := view["updatedAt"]; ok {
			view["updatedAt"] = formatEpochValue(view["updatedAt"])
		}
		df := NewDetailFormatter(dashboardDetailFieldOrder, dashboardDetailLabelMap, dashboardDetailSectionKeys)
		return df.Format(view, w)
	}
	return PrintResult(w, format, snap, nil, nil)
}

// newDashboardSharedCmd 는 dashboard shared 서브커맨드 그룹을 생성한다.
// 공유 대시보드(scope=global)의 get / set 을 포함한다.
func newDashboardSharedCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shared",
		Short: "공유 대시보드 (scope=global)",
		Long:  "모든 사용자가 공유하는 대시보드 스냅샷을 조회하거나 저장합니다.",
	}

	cmd.AddCommand(newDashboardGetCmd(client, "/api/v1/dashboards/shared", "shared"))
	cmd.AddCommand(newDashboardSetCmd(client, "/api/v1/dashboards/shared", "shared"))

	return cmd
}

// newDashboardMineCmd 는 dashboard mine 서브커맨드 그룹을 생성한다.
// 개인 대시보드(scope=user)의 get / set 을 포함한다.
func newDashboardMineCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mine",
		Short: "개인 대시보드 (scope=user)",
		Long:  "인증된 사용자 본인의 대시보드 스냅샷을 조회하거나 저장합니다.",
	}

	cmd.AddCommand(newDashboardGetCmd(client, "/api/v1/dashboards/mine", "mine"))
	cmd.AddCommand(newDashboardSetCmd(client, "/api/v1/dashboards/mine", "mine"))

	return cmd
}

// newDashboardGetCmd 는 대시보드 get 서브커맨드를 생성한다.
// GET {path} 로 스냅샷을 조회한다 (path 는 shared/mine 경로).
//
// 응답은 DashboardSnapshot 형식
// {"scope":"","owner":null,"version":N,"updatedAt":<epoch ms>,"payload":{...}} 이다.
func newDashboardGetCmd(client **Client, path, scope string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: fmt.Sprintf("%s 대시보드 스냅샷 조회", scope),
		Long: fmt.Sprintf(`%s 대시보드 스냅샷을 조회합니다.

예시:
  xflow dashboard %s get
  xflow dashboard %s get --format json`, scope, scope, scope),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var snap map[string]any
			if err := (*client).Get(path, &snap); err != nil {
				return err
			}
			return printDashboardSnapshot(cmd, snap)
		},
	}
}

// newDashboardSetCmd 는 대시보드 set 서브커맨드를 생성한다.
// PUT {path} 로 {"payload": <json>} 본문을 전송하여 스냅샷을 저장한다.
//
// --payload 플래그는 인라인 JSON('{...}') 또는 '@file.json'(파일 경로)을 받는다.
// 어느 경우든 JSON 으로 파싱 가능해야 하며, 아니면 ErrInvalidInput 을 반환한다.
func newDashboardSetCmd(client **Client, path, scope string) *cobra.Command {
	var payload string

	cmd := &cobra.Command{
		Use:   "set",
		Short: fmt.Sprintf("%s 대시보드 스냅샷 저장", scope),
		Long: fmt.Sprintf(`%s 대시보드 스냅샷을 저장합니다.

--payload 는 인라인 JSON 또는 '@파일경로'(JSON 파일 읽기)를 받습니다.

예시:
  xflow dashboard %s set --payload '{"widgets":[]}'
  xflow dashboard %s set --payload @dashboard.json`, scope, scope, scope),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := buildDashboardPutBody(payload)
			if err != nil {
				return err
			}

			var snap map[string]any
			if err := (*client).Put(path, body, &snap); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "%s 대시보드가 저장되었습니다 (version=%s).\n",
					scope, formatEpochValue(snap["version"]))
				return printDashboardSnapshot(cmd, snap)
			}
			return PrintResult(w, format, snap, nil, nil)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "대시보드 payload (인라인 JSON 또는 @파일경로)")
	_ = cmd.MarkFlagRequired("payload")

	return cmd
}

// buildDashboardPutBody 는 --payload 플래그 값을 PUT 요청 본문으로 변환한다.
//
// payload 가 '@' 로 시작하면 그 뒤를 파일 경로로 간주하여 파일 내용을 읽고,
// 그렇지 않으면 인라인 JSON 으로 취급한다. 어느 경우든 JSON 으로 파싱 가능해야 한다.
// 최종 본문은 {"payload": <raw json>} 형식이다 (DashboardPutRequest 의 payload 필드).
func buildDashboardPutBody(payload string) (map[string]any, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return nil, ErrInvalidInput("payload 를 지정해야 합니다 (--payload '{...}' 또는 --payload @file.json)")
	}

	var raw []byte
	if strings.HasPrefix(trimmed, "@") {
		filePath := strings.TrimSpace(trimmed[1:])
		if filePath == "" {
			return nil, ErrInvalidInput("@ 뒤에 파일 경로를 지정해야 합니다")
		}
		data, err := readFile(filePath)
		if err != nil {
			return nil, err
		}
		raw = data
	} else {
		raw = []byte(trimmed)
	}

	// payload 가 유효한 JSON 인지 검증한다.
	var parsed json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, ErrInvalidInput(fmt.Sprintf("payload JSON 파싱 실패: %v", err))
	}

	return map[string]any{"payload": parsed}, nil
}
