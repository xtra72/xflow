package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoteReleaseCmd 는 remote release 서브커맨드 그룹을 생성한다(SPEC-CLI-004 P3c).
// 릴리스 카탈로그 API(/api/v1/remote/releases)를 CLI 로 노출한다.
//
// 하위 커맨드:
//
//	list          릴리스 목록 + 에셋 수 조회
//	create        릴리스 메타데이터 생성
//	delete        릴리스 삭제(파괴적)
//	delete-asset  릴리스의 특정 에셋 삭제(파괴적)
//
// 참고: 에셋 업로드(multipart/form-data)와 공개 릴리스 피드(/updates/releases)는
// 이 작업의 범위가 아니다.
//
// confirmFn 은 delete/delete-asset 같은 파괴적 작업의 확인 프롬프트에 사용된다
// (remote node revoke 패턴 참고).
func newRemoteReleaseCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "릴리스 카탈로그 관리",
		Long:  "원격 업데이트용 릴리스 카탈로그의 목록 조회, 생성, 삭제, 에셋 삭제를 수행합니다.",
	}

	cmd.AddCommand(newRemoteReleaseListCmd(client))
	cmd.AddCommand(newRemoteReleaseCreateCmd(client))
	cmd.AddCommand(newRemoteReleaseDeleteCmd(client, confirmFn))
	cmd.AddCommand(newRemoteReleaseDeleteAssetCmd(client, confirmFn))

	return cmd
}

// remoteReleaseTableHeaders 는 릴리스 목록 테이블의 헤더이다.
var remoteReleaseTableHeaders = []string{"VERSION", "CHANNEL", "PUBLISHED_AT", "ASSETS", "NOTES"}

// remoteReleaseRowFunc 는 릴리스 맵에서 테이블 행을 추출한다.
// published_at_ms(epoch ms)는 정수 문자열로, assets 는 에셋 개수로 렌더링한다.
func remoteReleaseRowFunc(item any) []string {
	m, ok := item.(map[string]any)
	if !ok {
		return []string{"", "", "", "", ""}
	}
	return []string{
		fmt.Sprintf("%v", m["version"]),
		fmt.Sprintf("%v", m["channel"]),
		formatEpochValue(m["published_at_ms"]),
		formatEpochValue(assetCount(m["assets"])),
		fmt.Sprintf("%v", m["notes"]),
	}
}

// assetCount 는 assets 배열의 길이를 반환한다(JSON 디코딩 결과 []any).
// 배열이 아니면 0 을 반환한다.
func assetCount(v any) int {
	if arr, ok := v.([]any); ok {
		return len(arr)
	}
	return 0
}

// extractReleases 는 릴리스 목록 응답({"releases":[...]})에서 releases 배열을
// 맵 슬라이스로 추출한다(device.go 의 extractHistoryEntries 패턴 참고).
func extractReleases(resp map[string]any) []map[string]any {
	raw, ok := resp["releases"].([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

// newRemoteReleaseListCmd 는 remote release list 서브커맨드를 생성한다.
// GET /api/v1/remote/releases 로 릴리스 카탈로그를 조회한다.
//
// 응답은 객체 {"releases":[...]} 이며(배열 아님) map[string]any 로 디코딩한 뒤
// releases 배열을 추출한다. json/yaml 포맷은 전체 객체를 그대로 통과시킨다.
func newRemoteReleaseListCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "릴리스 목록 조회",
		Long: `릴리스 카탈로그 목록과 각 릴리스의 에셋 수를 조회합니다.

예시:
  xflow remote release list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp map[string]any
			if err := (*client).Get("/api/v1/remote/releases", &resp); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				releases := extractReleases(resp)
				return PrintResult(w, "table", releases, remoteReleaseTableHeaders, remoteReleaseRowFunc)
			}
			// json/yaml: 전체 객체({releases:[...]})를 그대로 통과시킨다.
			return PrintResult(w, format, resp, nil, nil)
		},
	}
}

// remoteReleaseDetailFieldOrder 는 릴리스 상세(create 응답) 출력의 필드 순서이다.
var remoteReleaseDetailFieldOrder = []string{
	"version", "channel", "notes", "published_at_ms", "assets",
}

// remoteReleaseDetailLabelMap 는 릴리스 상세 출력의 필드 라벨 매핑이다.
var remoteReleaseDetailLabelMap = map[string]string{
	"version":         "Version",
	"channel":         "Channel",
	"notes":           "Notes",
	"published_at_ms": "Published At",
	"assets":          "Assets",
}

// remoteReleaseDetailSectionKeys 는 별도 섹션으로 출력할 릴리스 상세 키 목록이다.
// assets(중첩 배열)는 별도 섹션으로 렌더링한다(remote node summary 패턴 참고).
var remoteReleaseDetailSectionKeys = map[string]bool{
	"assets": true,
}

// remoteReleaseDetailDisplay 는 table/text 출력을 위한 표시용 사본을 만든다.
// published_at_ms(epoch ms)를 정수 문자열화하여 float64 과학표기를 방지한다.
// 원본은 변경하지 않는다.
func remoteReleaseDetailDisplay(rel map[string]any) map[string]any {
	out := make(map[string]any, len(rel))
	for k, v := range rel {
		out[k] = v
	}
	if v, ok := out["published_at_ms"]; ok {
		out["published_at_ms"] = formatEpochValue(v)
	}
	return out
}

// printRemoteReleaseDetail 은 단일 릴리스 상세를 포맷에 맞게 출력한다.
// table/text 는 epoch-ms 필드를 정수 문자열로 변환한 표시용 사본을 사용하고,
// json/yaml 은 원본을 그대로 통과시킨다.
func printRemoteReleaseDetail(cmd *cobra.Command, rel map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" || format == "text" {
		df := NewDetailFormatter(remoteReleaseDetailFieldOrder, remoteReleaseDetailLabelMap, remoteReleaseDetailSectionKeys)
		return df.Format(remoteReleaseDetailDisplay(rel), w)
	}
	return PrintResult(w, format, rel, nil, nil)
}

// newRemoteReleaseCreateCmd 는 remote release create 서브커맨드를 생성한다.
// POST /api/v1/remote/releases 로 릴리스 메타데이터를 생성한다.
//
// 요청 본문은 {"version":..,"channel":..,"notes":..} 이며 version 은 인자에서 항상
// 채우고 channel/notes 는 명시적으로 설정된(Changed) 플래그만 포함한다
// (device metadata buildMetadataBody 패턴 참고). 응답은 릴리스 객체이다.
func newRemoteReleaseCreateCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <version>",
		Short: "릴리스 생성",
		Long: `릴리스 메타데이터를 생성합니다.

예시:
  xflow remote release create v0.19.0
  xflow remote release create v0.19.0 --channel stable --notes "버그 수정"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := strings.TrimSpace(args[0])
			if version == "" {
				return ErrInvalidInput("version 을 지정해야 합니다")
			}

			// version 은 인자에서 항상 포함하고, channel/notes 는 Changed 시에만 포함한다.
			body := map[string]any{"version": version}
			if cmd.Flags().Changed("channel") {
				v, _ := cmd.Flags().GetString("channel")
				body["channel"] = v
			}
			if cmd.Flags().Changed("notes") {
				v, _ := cmd.Flags().GetString("notes")
				body["notes"] = v
			}

			var rel map[string]any
			if err := (*client).Post("/api/v1/remote/releases", body, &rel); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "릴리스 '%s' 생성됨\n\n", version)
				return printRemoteReleaseDetail(cmd, rel)
			}
			return PrintResult(w, format, rel, nil, nil)
		},
	}

	cmd.Flags().String("channel", "", "릴리스 채널 (예: stable)")
	cmd.Flags().String("notes", "", "릴리스 노트 (선택)")

	return cmd
}

// newRemoteReleaseDeleteCmd 는 remote release delete 서브커맨드를 생성한다.
// DELETE /api/v1/remote/releases/{version} 로 릴리스를 삭제한다.
//
// 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
// 응답은 {version, deleted} 이다.
func newRemoteReleaseDeleteCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <version>",
		Short: "릴리스 삭제",
		Long: `릴리스를 삭제합니다(파괴적 작업).

예시:
  xflow remote release delete v0.19.0
  xflow remote release delete v0.19.0 --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := strings.TrimSpace(args[0])
			if version == "" {
				return ErrInvalidInput("version 을 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("릴리스 '%s' 를 삭제하시겠습니까?", version)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/remote/releases/%s", url.PathEscape(version))
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "릴리스 '%s' 삭제됨\n", version)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}

// newRemoteReleaseDeleteAssetCmd 는 remote release delete-asset 서브커맨드를 생성한다.
// DELETE /api/v1/remote/releases/{version}/assets/{os}/{arch} 로 특정 에셋을 삭제한다.
//
// 파괴적 작업이므로 --yes 가 없으면 confirmFn 으로 확인을 요청한다.
// version/os/arch 각 경로 세그먼트는 url.PathEscape 로 이스케이프한다.
// 응답은 {version, os, arch, deleted} 이다.
func newRemoteReleaseDeleteAssetCmd(client **Client, confirmFn func(string, io.Reader) bool) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete-asset <version> <os> <arch>",
		Short: "릴리스 에셋 삭제",
		Long: `릴리스의 특정 OS/아키텍처 에셋을 삭제합니다(파괴적 작업).

예시:
  xflow remote release delete-asset v0.19.0 linux amd64
  xflow remote release delete-asset v0.19.0 linux arm64 --yes`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := strings.TrimSpace(args[0])
			if version == "" {
				return ErrInvalidInput("version 을 지정해야 합니다")
			}
			osName := strings.TrimSpace(args[1])
			if osName == "" {
				return ErrInvalidInput("os 를 지정해야 합니다")
			}
			arch := strings.TrimSpace(args[2])
			if arch == "" {
				return ErrInvalidInput("arch 를 지정해야 합니다")
			}

			w := cmd.OutOrStdout()

			if !yes {
				prompt := fmt.Sprintf("릴리스 '%s' 의 에셋 '%s/%s' 를 삭제하시겠습니까?", version, osName, arch)
				if !confirmFn(prompt, os.Stdin) {
					fmt.Fprintln(w, "취소되었습니다.")
					return nil
				}
			}

			var result map[string]any
			// 각 경로 세그먼트를 개별 이스케이프한다(slash 등 특수문자 방지).
			path := fmt.Sprintf("/api/v1/remote/releases/%s/assets/%s/%s",
				url.PathEscape(version), url.PathEscape(osName), url.PathEscape(arch))
			if err := (*client).Delete(path, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			if format == "table" || format == "text" {
				fmt.Fprintf(w, "릴리스 '%s' 의 에셋 '%s/%s' 삭제됨\n", version, osName, arch)
				return nil
			}
			return PrintResult(w, format, result, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "확인 프롬프트 건너뛰기")

	return cmd
}
