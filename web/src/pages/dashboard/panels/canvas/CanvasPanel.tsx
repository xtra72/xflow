// 캔버스 패널 진입점 — 데이터 바인딩 + 견고성 분기 (SPEC-CANVAS-001 T7/T11).
//
// 이 파일이 지는 일은 **번역 하나**다: `usePanelSeriesData` 가 돌려준 시리즈를 요소별
// 목표 스타일·문구로 옮겨 `CanvasSurface` 에 넘긴다. 그리기·트윈·rAF 루프·DPR·가시성은
// 전부 표면(T10)이 이미 소유하므로 여기에는 렌더 루프가 없다.
//
// 데이터 축의 규율 셋(REQ-03):
//   1. **훅을 새로 만들지 않는다.** 신규 REST 경로도, 신규 백엔드 엔드포인트도 없다.
//      조회는 기존 통합 훅 하나이며, 그 뒤에 store / TSDB / sysmetrics 3종이 이미 있다.
//   2. **원본 config 를 그대로 넘긴다.** 파싱한 view 가 아니다 — 소스 판정(다중 소스 배열·
//      X축 구간 등)은 그 계약이 소유하므로, 파서를 거친 좁은 객체를 넘기면 소스가 사라진다
//      (`HeatmapPanel.tsx:139` 선례).
//   3. **수명주기를 새로 만들지 않는다.** 인터벌 해제·AbortController 중단은 훅의 몫이다.
//
// 견고성 축(REQ-05)은 네 갈래이며 각각 인수 기준에 묶여 있다.
//   - AC-E1 요소 0개 → 빈 상태 안내(렌더 예외 없음).
//   - AC-E2 시리즈 결측/무값 → 요소는 **기본 스타일로 그대로 그려지고**, `{value}` 는 결측
//     표기로 치환되며, `nodata` 행은 정상적으로 일치한다.
//   - AC-E3 규칙 미일치 → 기본 스타일·기본 문구(오류가 아니다).
//   - AC-E4 폴링 실패 → **마지막 성공 판독값을 계속 내려보내고** 오류 배지만 덧붙인다.
//     표면은 props 가 그대로면 아무것도 그리지 않으므로, "지우지 않는다" 는 곧 "빈 맵으로
//     갈아치우지 않는다" 이다.
//
// **SPEC-CANVAS-002 가 이 파일에 더한 것은 편집 배선 하나다**(T6): 기존 세 겹 게이팅
// (`usePanelEditMode`)을 그대로 쓰고, 표면의 `overlay` 슬롯에 편집 층을 얹고, 001 이
// 받아만 두었던 `onConfigChange` 를 **실제로 쓴다**. 등록 6지점은 건드리지 않는다 —
// 001 이 그 콜백을 미리 흘려 둔 덕분에 그럴 필요가 없다(REQ-05 금지 조항).
//
// @spec SPEC-CANVAS-001 · SPEC-CANVAS-002 (T6 — 편집 배선)

import { useCallback, useMemo, useRef } from 'react';
import { Shapes } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import type { ChartEntry, StoreSeriesRef, StoreSourceConfig } from '../charts/chartChannelTypes';
import { storeSeriesId } from '../charts/chartChannelTypes';
import { usePanelSeriesData } from '../charts/usePanelSeriesData';
import type { VisibilitySource } from '../charts/visiblePolling';
import { usePanelEditMode } from '../PanelEditToggle';
import type { CanvasElement } from './canvasConfig';
import { parseCanvasConfig } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { evaluateRules, type ResolvedStyle } from './canvasRules';
import { renderTextTemplate } from './canvasText';
import CanvasSurface, {
  type CanvasOverlayContext,
  type FrameScheduler,
} from './CanvasSurface';

// --- 타입 ---------------------------------------------------------------

export interface CanvasPanelProps {
  panelId: string;
  title?: string;
  /**
   * 패널 config(불투명 JSON). 그리기용 view 는 `parseCanvasConfig` 로 읽되, 데이터 조회에는
   * **이 원본 객체를 그대로** 넘긴다(위 규율 2).
   */
  config: Record<string, unknown>;
  /**
   * config 최상위 얕은 병합 패치(다이얼로그의 `handleConfigChange` 규약).
   *
   * 001 은 렌더 전용이라 이 자리를 **받아만 두었다**(가정 A7). **002 가 이것을 실제로
   * 쓴다**(T6) — 캔버스 드래그가 요소 기하를 바꾸면 `{ elements }` 패치로 흘려보낸다.
   * 콜백이 없으면 편집도 꺼진다(`canEdit`) — 끌어도 저장할 곳이 없기 때문이다.
   */
  onConfigChange?: (config: Record<string, unknown>) => void;
  /** 제목 변경 콜백. 001 과 같은 이유로 받아만 둔다. */
  onTitleChange?: (title: string) => void;
  /**
   * 설정 미리보기처럼 **항상** 편집인 자리인가. 그때는 토글을 감춘다(REQ-01 · AC-07).
   * 통계·파이가 쓰는 `forceEdit` 와 같은 이름·같은 뜻이다. 다이얼로그가 이 값을 넘기는
   * 것은 T11 의 몫이며, 미지정이면 종전과 같이 대시보드 편집모드 게이팅만 남는다.
   */
  forceEdit?: boolean;
  /** 가시성 판정 주입(테스트 결정성). 미지정이면 표면이 document 를 쓴다. */
  visibilitySource?: VisibilitySource;
  /** 프레임 예약기 주입(테스트 결정성). 미지정이면 표면이 rAF 를 쓴다. */
  scheduler?: FrameScheduler;
}

/**
 * 한 시리즈의 현재 판독값.
 *
 * `name` 을 함께 들고 다니는 이유는 `{name}` 토큰 때문이다. 표시명은 훅이 정한 이름
 * (alias 또는 `key · metric{k=v}`)이며 **로케일에 의존하지 않는다** — 이 경로의 훅이
 * I18n Provider 없이 돌아야 한다는 기존 제약이 그 대가로 얻어지는 성질이다(REQ-03).
 */
interface SeriesReading {
  /** 최신 유한값. 결측(시리즈 없음·값 없음·비유한)이면 null. */
  value: number | null;
  /** 시리즈 표시명. `{name}` 치환 값. */
  name: string;
}

/** 요소 id 로 키잉된 한 프레임분의 목표. `CanvasSurface` 가 그대로 먹는다. */
interface CanvasFrame {
  targetStyles: Record<string, ResolvedStyle>;
  texts: Record<string, string | undefined>;
}

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 타임라인에서 **마지막 유한 숫자값**을 뽑는다(`binding.agg: 'last'`).
 *
 * 뒤에서부터 훑는 것이 요점이다. `seriesEntries` 의 타임라인은 값이 없는 버킷을 `null` 로
 * 담으므로(훅 계약), 맨 끝 한 칸만 보면 빈 버킷이 하나만 걸려도 멀쩡한 시리즈가 통째로
 * 결측이 된다. `heatmapJoin.latestFiniteValue` 와 같은 규율이다.
 *
 * 숫자가 아닌 값 · NaN · Infinity 는 전부 건너뛴다 — 규칙 평가와 문구 치환에 비유한 값이
 * 흘러들면 비교가 조용히 거짓이 되고 `toFixed` 가 "Infinity" 를 화면에 찍는다
 * (§품질 게이트 Secured: 외부 입력은 숫자 파싱 후 NaN/Infinity 방어).
 */
function latestFiniteValue(entries: ChartEntry[] | undefined): number | null {
  if (!entries) return null;
  for (let i = entries.length - 1; i >= 0; i--) {
    const v = entries[i]!.value;
    if (typeof v === 'number' && Number.isFinite(v)) return v;
  }
  return null;
}

/**
 * 조회 결과(**표시 이름** 공간)를 바인딩이 쓰는 **동일성 키** 공간으로 옮긴다.
 *
 * 훅은 시리즈를 표시 이름(alias)으로 키잉하는데, 바인딩은 시리즈 동일성 키로 참조한다
 * (REQ-03, 가정 A5). 두 공간이 어긋나면 이름을 바꾼 순간 바인딩이 끊기므로, 히트맵이
 * `resolveSensorSeries` 로 세운 다리를 그대로 따른다: 컬럼 수와 config 의 시리즈 수가 같을
 * 때(=요청과 응답이 1:1) 인덱스로 짝지어 동일성 키를 붙인다.
 *
 * 정렬 불가(한 key 가 다중 컬럼으로 펼쳐진 경우) 또는 store 가 아닌 소스(TSDB·sysmetrics —
 * `store_source.series` 가 비어 있다)에서는 **조회 이름이 곧 동일성 키**다. 이 경로에서도
 * 이름은 훅이 만든 로케일 비의존 문자열이므로 REQ-03 의 제약을 깨지 않는다.
 */
function resolveSeriesReadings(
  seriesNames: readonly string[],
  seriesEntries: Map<string, ChartEntry[]>,
  refs: readonly StoreSeriesRef[],
): Map<string, SeriesReading> {
  const readings = new Map<string, SeriesReading>();
  const aligned = seriesNames.length === refs.length;

  seriesNames.forEach((name, index) => {
    const ref = aligned ? refs[index] : undefined;
    const id = ref ? storeSeriesId(ref.key, ref.field ?? '', ref.tags ?? {}) : name;
    // 첫 컬럼이 이긴다 — 같은 동일성 키가 두 번 나오면 뒤 컬럼은 같은 시리즈의 중복이다.
    if (readings.has(id)) return;
    readings.set(id, { value: latestFiniteValue(seriesEntries.get(name)), name });
  });

  return readings;
}

/**
 * 요소 목록 + 판독값 → 한 프레임분의 목표 스타일·문구.
 *
 * 세 갈래를 여기서 가른다.
 * - **정적 요소**(바인딩 없음): 규칙 평가 대상이 아니다. 기본 스타일로 그리고, 기본 문구
 *   템플릿은 값 없이 치환한다(`{value}` → 결측 표기).
 * - **바인딩 요소, 값 있음**: 규칙 표를 평가해 첫 일치 행의 패치를 기본 스타일 위에 덮는다.
 * - **바인딩 요소, 값 결측**(AC-E2): 값을 `null` 로 넘긴다. `nodata` 행이 정상적으로 일치하고,
 *   일치 행이 없으면 기본 스타일 그대로다 — **요소를 숨기지 않는다.**
 *
 * 문구 템플릿은 `ResolvedStyle.text ?? el.text` 다. 규칙 행이 문구 패치를 담았을 때만
 * `text` 가 실리므로, 부재는 "요소의 기본 문구를 쓰라" 는 뜻이다(canvasRules 계약).
 */
function buildCanvasFrame(
  elements: readonly CanvasElement[],
  readings: Map<string, SeriesReading>,
): CanvasFrame {
  const targetStyles: Record<string, ResolvedStyle> = {};
  const texts: Record<string, string | undefined> = {};

  for (const el of elements) {
    const reading = el.binding ? readings.get(el.binding.series) : undefined;
    const value = reading?.value ?? null;
    const style = el.binding
      ? evaluateRules(value, el.rules, el.style)
      : { ...el.style };
    targetStyles[el.id] = style;

    const template = style.text ?? el.text;
    texts[el.id] =
      template === undefined
        ? undefined
        : renderTextTemplate(template, {
            value,
            name: reading?.name,
            unit: el.unit,
            decimals: el.decimals,
          });
  }

  return { targetStyles, texts };
}

// --- 컴포넌트 -----------------------------------------------------------

export default function CanvasPanel({
  title,
  config,
  onConfigChange,
  forceEdit = false,
  visibilitySource,
  scheduler,
}: CanvasPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();

  // 그리기용 view. 관용 파서라 결측·손상·구버전 config 에서도 예외를 던지지 않는다(REQ-01).
  const cfg = useMemo(() => parseCanvasConfig(config), [config]);

  // 조회는 **원본 config** 로 한다(HeatmapPanel:139 선례). 파싱한 view 를 넘기면 훅이
  // 소유한 소스 판정 축(다중 소스·X축 구간)이 사라진다.
  const series = usePanelSeriesData(config);
  const isError = series.status === 'error';

  // 동일성 키를 만들 원본 시리즈 목록. store 가 아닌 소스에서는 비어 있고, 그때는 조회
  // 이름이 곧 동일성 키가 된다(`resolveSeriesReadings` 주석).
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const refs = useMemo<StoreSeriesRef[]>(
    () => (Array.isArray(storeSource?.series) ? storeSource.series : []),
    [storeSource],
  );

  const liveReadings = useMemo(
    () => resolveSeriesReadings(series.seriesNames, series.seriesEntries, refs),
    [series.seriesNames, series.seriesEntries, refs],
  );

  // AC-E4: 마지막 **성공** 판독값을 붙들어 둔다.
  //
  // 래치 대상이 판독값이지 프레임 전체가 아닌 것이 요점이다. 프레임을 통째로 얼리면 오류가
  // 지속되는 동안 사용자가 요소를 고쳐도 화면이 옛 그림에 갇힌다 — 죽은 것은 데이터뿐이므로
  // 얼릴 것도 데이터뿐이다. 렌더 중 ref 쓰기는 히트맵의 좌표 공간 고정과 같은 형태다.
  const lastGoodRef = useRef(liveReadings);
  if (!isError) lastGoodRef.current = liveReadings;
  const readings = isError ? lastGoodRef.current : liveReadings;

  // 참조 안정성이 곧 유휴다 — 표면은 props 참조가 그대로면 프레임을 예약하지 않는다(AC-E6).
  const frame = useMemo(() => buildCanvasFrame(cfg.elements, readings), [cfg.elements, readings]);

  // AC-E1: 요소가 없으면 표면 대신 빈 상태 안내를 그린다(렌더 예외 없음).
  const isEmpty = cfg.elements.length === 0;
  const headerVisible = showTitle && !!title;

  // --- SPEC-CANVAS-002 T6: 캔버스 내 시각 편집 배선 ---
  //
  // **새 게이팅 개념을 만들지 않는다**(REQ-01). 히트맵·게이지·통계·바·파이가 전부
  // 따르는 세 겹 게이팅을 그대로 쓴다 — 캔버스만 다르게 두면 사용자가 패널마다 다른
  // 규칙을 배워야 한다.
  const edit = usePanelEditMode({
    canEdit: onConfigChange !== undefined,
    forced: forceEdit,
    testId: 'canvas-edit-toggle',
    below: headerVisible,
  });

  /** 드래그가 만든 새 요소 배열을 config 로 흘려보낸다 — 기하 쓰기의 유일한 출구다. */
  const handleElementsChange = useCallback(
    (next: CanvasElement[]) => onConfigChange?.({ elements: next }),
    [onConfigChange],
  );

  /**
   * 표면의 오버레이 슬롯. 스테이지 크기와 실측 글자 폭은 **표면이 잰 것을 그대로**
   * 받아 넘긴다(측정원이 하나다 — AC-E2).
   *
   * 이 렌더 prop 은 표면의 어느 효과 의존성에도 들어가지 않으므로 프레임을 예약하지
   * 않는다. 선택·호버가 루프를 깨우지 않는다는 보장이 여기서 성립한다(AC-E4).
   */
  const renderOverlay = useCallback(
    ({ stage, textWidths }: CanvasOverlayContext) => (
      <CanvasEditOverlay
        enabled={edit.active}
        elements={cfg.elements}
        stage={stage}
        textWidths={textWidths}
        onElementsChange={handleElementsChange}
      />
    ),
    [edit.active, cfg.elements, handleElementsChange],
  );

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-2 shadow">
      {headerVisible && (
        <div className="mb-1 flex shrink-0 items-center gap-2" data-testid="canvas-title">
          <Shapes className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span
            className="truncate text-sm font-semibold text-(--color-text-primary)"
            style={titleStyle}
          >
            {title}
          </span>
        </div>
      )}

      {isEmpty ? (
        <div
          data-testid="canvas-empty"
          className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 rounded-md border border-dashed border-(--color-border-default) p-4 text-center"
        >
          <Shapes className="h-6 w-6 text-(--color-text-muted)" />
          <span className="text-sm text-(--color-text-muted)">
            {t('dashboard.canvas.emptyState')}
          </span>
        </div>
      ) : (
        // 오류 상태에서도 **표면을 내리지 않는다**(AC-E4). 마지막 프레임은 표면의 백킹 버퍼에
        // 남아 있고, 내려보내는 목표도 마지막 성공값이라 다시 그릴 것이 없다.
        <CanvasSurface
          elements={cfg.elements}
          targetStyles={frame.targetStyles}
          texts={frame.texts}
          panelTween={cfg.tween}
          background={cfg.background}
          visibilitySource={visibilitySource}
          scheduler={scheduler}
          overlay={renderOverlay}
        />
      )}

      {/* 배치 편집 토글. `canEdit && dashboardEditMode && !forced` 일 때만 나온다. */}
      {edit.toggle}

      {/* 오류 배지(AC-E4): 마지막 프레임을 유지한 채 상태만 덧띄우고 다음 주기에 재시도한다. */}
      {isError && (
        <span
          data-testid="canvas-error"
          className="pointer-events-none absolute right-2 top-2 rounded bg-red-500/90 px-2 py-0.5 text-xs text-white"
        >
          {t('dashboard.canvas.error')}
        </span>
      )}
    </div>
  );
}
