package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newDashboardCmd 는 대시보드 스냅샷 조회 커맨드 그룹을 생성한다.
// 읽기 전용 호환 shim(GET /api/v1/dashboards/{shared,mine})을 CLI 로 노출한다.
//
// 서브커맨드 그룹:
//
//	shared  공유 대시보드 (scope=global)
//	mine    개인 대시보드 (scope=user, JWT username 으로 owner 결정)
//
// 각 그룹은 get 하위 커맨드만 포함한다.
//
// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.3)
// set(PUT) 하위 커맨드는 제거되었다. 대시보드가 1급 엔티티가 되면서 묶음 단위 쓰기는
// "어느 대시보드의 어느 version 에 대한 쓰기인가" 를 결정할 수 없어 낙관적 동시성이
// 성립하지 않으므로, 서버가 PUT/DELETE /dashboards/{shared,mine} 라우트를 등록하지
// 않는다(호출 시 404). 되살리려면 서버 계약부터 바뀌어야 한다 — 여기만 되돌리면
// 404 를 내는 커맨드가 다시 생긴다.
//
// 대시보드 쓰기는 현재 CLI 에 노출되어 있지 않다. 대시보드 단위 커맨드
// (create/update/delete)는 본 SPEC 의 범위 밖이다.
func newDashboardCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "대시보드 스냅샷 조회",
		Long:  "공유(shared) 및 개인(mine) 대시보드 스냅샷을 조회합니다 (읽기 전용).",
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
// 공유 대시보드(scope=global)의 get 을 포함한다 (읽기 전용).
func newDashboardSharedCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shared",
		Short: "공유 대시보드 (scope=global)",
		Long:  "모든 사용자가 공유하는 대시보드 스냅샷을 조회합니다 (읽기 전용).",
	}

	cmd.AddCommand(newDashboardGetCmd(client, "/api/v1/dashboards/shared", "shared"))

	return cmd
}

// newDashboardMineCmd 는 dashboard mine 서브커맨드 그룹을 생성한다.
// 개인 대시보드(scope=user)의 get 을 포함한다 (읽기 전용).
func newDashboardMineCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mine",
		Short: "개인 대시보드 (scope=user)",
		Long:  "인증된 사용자 본인의 대시보드 스냅샷을 조회합니다 (읽기 전용).",
	}

	cmd.AddCommand(newDashboardGetCmd(client, "/api/v1/dashboards/mine", "mine"))

	return cmd
}

// newDashboardGetCmd 는 대시보드 get 서브커맨드를 생성한다.
// GET {path} 로 스냅샷을 조회한다 (path 는 shared/mine 경로).
//
// 응답은 DashboardSnapshot 형식
// {"scope":"","owner":null,"version":N,"updatedAt":<epoch ms>,"payload":{...}} 이다.
// 서버는 이 형상을 신규 1급 엔티티 모델에서 합성해 돌려준다(SPEC-DASHBOARD-004 §4.4).
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
