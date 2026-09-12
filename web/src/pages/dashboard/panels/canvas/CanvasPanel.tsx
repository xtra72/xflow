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
//   - AC-E1 요소 0개 → 빈 상태 안내(렌더 예외 없음). **안내는 표면을 대신하지 않고 겹친다**
//     (AC-E9 — 002 의 팔레트가 그 표면 위에 있으므로, 대신하면 첫 도형을 놓을 곳이 사라진다).
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
// **SPEC-CANVAS-006 이 이 파일에 더한 것은 둘이다**: 표면에 `workspace={edit.active}` 를
// 넘기는 한 줄(M4)과, 편집 중 캔버스 크기를 잰 패널 상자에서 유도하는 게이트(M8 ·
// REQ-07). 둘은 **같은 게이트**를 지나며, 그 공유가 곧 이 회차의 안전 논거다 —
// 표시 없이 유도만 도는 상태가 형상 자체로 존재하지 않는다(불변식 I15).
//
// @spec SPEC-CANVAS-001 · SPEC-CANVAS-002 (T6 — 편집 배선) · SPEC-CANVAS-006 (M4 · M8)

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { Shapes } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import type { ChartEntry, StoreSeriesRef, StoreSourceConfig } from '../charts/chartChannelTypes';
import { storeSeriesId } from '../charts/chartChannelTypes';
import { usePanelSeriesData } from '../charts/usePanelSeriesData';
import type { VisibilitySource } from '../charts/visiblePolling';
import { usePanelEditMode } from '../PanelEditToggle';
import type { CanvasSize } from './canvasConfig';
import { isNumericElement, parseCanvasConfig } from './canvasConfig';
import { isGroup, type CanvasNode } from './group/groupTypes';
import {
  useCanvasLiveSeriesPublisher,
  type CanvasSeriesOption,
} from './canvasEditContext';
import CanvasEditOverlay from './CanvasEditOverlay';
import { type StageSize } from './canvasGeometry';
import { derivedCanvasSize } from './canvasWorkspace';
import { useCanvasStageAspectPublisher } from './canvasStageAspect';
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
  /**
   * 최신 판독값 **원본**. 숫자 축을 지나지 않았으므로 `"ON"` 같은 문자열도 살아 있다.
   *
   * `value` 옆에 나란히 두는 것이 요점이다. 요소가 `numeric: false` 로 저술되면 화면에
   * 나가야 하는 것은 숫자로 읽어 낸 것이 아니라 **받은 그대로**인데, `latestFiniteValue`
   * 는 비숫자를 걸러 내므로 그 값은 이 자리가 없으면 판독 단계에서 이미 사라진다.
   */
  raw: unknown;
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
 * 타임라인에서 **마지막으로 값이 있던 칸**을 서식 없이 그대로 뽑는다.
 *
 * 위 `latestFiniteValue` 와 훑는 방향·건너뛰는 이유(빈 버킷)는 같고, **거르는 기준만**
 * 다르다: 여기서는 숫자인지 묻지 않는다. `numeric: false` 로 저술된 요소가 화면에 내보내야
 * 하는 것이 정확히 이 값이며, 숫자 판정을 여기서 한 번 더 하면 `"ON"` 은 이 함수를 지나는
 * 순간 사라져 `{value}` 에 닿을 길이 없어진다.
 *
 * `null` · `undefined` 만 "값 없음" 으로 건너뛴다 — 훅이 빈 버킷을 담는 형태가 그 둘이다.
 */
function latestRawValue(entries: ChartEntry[] | undefined): unknown {
  if (!entries) return null;
  for (let i = entries.length - 1; i >= 0; i--) {
    const v = entries[i]!.value;
    if (v !== null && v !== undefined) return v;
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
    const entries = seriesEntries.get(name);
    readings.set(id, { value: latestFiniteValue(entries), raw: latestRawValue(entries), name });
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
  elements: readonly CanvasNode[],
  readings: Map<string, SeriesReading>,
): CanvasFrame {
  const targetStyles: Record<string, ResolvedStyle> = {};
  const texts: Record<string, string | undefined> = {};

  for (const el of elements) {
    // SPEC-CANVAS-004 M1 — 그룹은 그릴 도형이 없어 제 스타일도 문구도 내지 않는다.
    // 부품의 복합 키 항목(§프레임 키 표면 1·2)은 M9 가 2단 순회로 더한다.
    if (isGroup(el)) continue;
    const reading = el.binding ? readings.get(el.binding.series) : undefined;
    const numeric = isNumericElement(el);
    // 숫자로 읽지 않는 요소에는 **비교할 수가 없다**. 그래서 규칙 평가에 넘기는 값은
    // `null` 이며, 그 결과 `nodata` 행만 일치할 수 있다(스칼라 연산자는 값이 있어야 성립한다
    // — `canvasRules.matchesRule`). 조용히 그렇게 되면 사용자는 "규칙이 고장났다" 로 읽으므로,
    // 편집기가 규칙 표 제목 뒤 `?` 로 그 사실을 말한다(`rulesNonNumericHint`).
    const value = numeric ? (reading?.value ?? null) : null;
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
            raw: reading?.raw,
            numeric,
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

  // --- 결함 D: 바인딩 선택지를 **판독값과 같은 공간**에서 낸다 ---
  //
  // `buildCanvasFrame` 이 `readings.get(el.binding.series)` 로 찾는 바로 그 키 집합을 그대로
  // 내놓는다. 목록 편집기가 config 로 키를 **추측**하면 위 `resolveSeriesReadings` 의 정렬
  // 판정과 갈라지고(참조 1개가 컬럼 3개로 펼쳐지는 흔한 경우), 그때 고른 키는 어느 판독값
  // 과도 만나지 못한다. 두 공간을 하나로 묶는 유일한 길은 **찾는 쪽이 내놓는 것**이다.
  //
  // 래치된 `readings` 를 쓴다 — 폴링이 한 번 실패했다고 사용자가 편집 중인 드롭다운이
  // 비워지면, 고르던 항목이 손 밑에서 사라진다(AC-E4 와 같은 근거).
  const liveSeriesOptions = useMemo<CanvasSeriesOption[]>(
    () => [...readings].map(([id, reading]) => ({ id, label: reading.name })),
    [readings],
  );

  const publishSeries = useCanvasLiveSeriesPublisher();

  // 렌더 중에 쓰지 않고 효과로 미룬다 — 렌더 단계에서 남의 상태를 갈면 React 가 그 렌더를
  // 버리고 다시 돌린다. 매 폴링마다 이 효과는 다시 돌지만(판독값 맵이 새 참조다) 발행은
  // **값이 실제로 달라졌을 때만** 상태를 갈므로 고리가 생기지 않는다(`sameSeriesOptions`).
  useEffect(() => {
    publishSeries(liveSeriesOptions);
  }, [publishSeries, liveSeriesOptions]);

  // AC-E1: 요소가 없으면 빈 상태 안내를 **표면 위에 겹쳐** 알린다(렌더 예외 없음).
  //
  // **표면을 대신하지 않는다**(AC-E9). 001 이 이 자리를 분기로 쓴 것은 그때 패널이 렌더
  // 전용이어서(가정 A7) 요소 0개면 그릴 것도, 누를 것도 없었기 때문이다. 002 가 오버레이에
  // 도형 팔레트를 얹으면서 그 전제가 깨졌다 — 표면이 없으면 오버레이도 없고, 오버레이가
  // 없으면 팔레트도 없다. 즉 **첫 도형을 놓아야 할 바로 그때 놓을 곳이 사라진다.** 안내는
  // 여전히 필요하므로 없애지 않고 겹치는 층으로 옮긴다.
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
    // SPEC-CANVAS-004 M6 — 오버레이가 쓰는 원소 타입이 `CanvasNode` 로 넓어졌다. M3~M5 동안
    // 여기서 그룹을 걸러 내려보냈고, 그래서 오버레이의 쓰기가 손으로 저술한 그룹을
    // 떨어뜨렸다. 그 좁히기가 사라진 자리가 이 한 줄이다.
    (next: CanvasNode[]) => onConfigChange?.({ elements: next }),
    [onConfigChange],
  );

  // --- SPEC-CANVAS-006 M8: 캔버스 크기는 편집 중 **패널 크기**다 (REQ-07) ---
  //
  // ## 게이트가 둘로 줄었다 — 그리고 그 둘은 사실 하나다
  //
  // 0.10.0 은 이 맞춤을 조건 넷과 한 번 표식(`autoFitDone`)으로 막았다. 그 근거는 하나뿐
  // 이었다: 저장된 요소 좌표는 **절대 캔버스 단위**라, 캔버스가 바뀌면 사용자가 놓아 둔
  // 자리의 뜻이 **조용히** 달라지고 그 어긋남은 되돌릴 수 없다. 위험했던 것은 좌표가
  // 움직인다는 사실이 아니라 그것이 **조용하다**는 사실이었다.
  //
  // **006 이 그 침묵을 없앴다.** 출력 영역이 더 넓은 작업 영역 안에 그려진 사각형이 되면
  // (M6), 뜻이 달라진 요소는 그 사각형 안에 남거나 **눈에 보이게** 밖으로 나가고, 밖으로
  // 나가도 사라지지 않으며(M2), **끌어서 되돌릴 수 있다**(M7). 그래서 조건 넷 가운데 셋이
  // 근거를 잃었다: "요소가 없다" · "크기가 기본값이다" · "한 번만".
  //
  // 남는 것은 둘이고, 정찰이 확인한 대로 그 둘은 하나로 겹친다 — `usePanelEditMode` 가
  // `active = canEdit && (editing || forced)` 이고 `canEdit = onConfigChange !== undefined`
  // 이므로 **편집이 켜졌다는 것은 곧 쓸 곳이 있다는 뜻**이다. 그럼에도 둘 다 적는다(한쪽
  // 형상이 바뀌는 날 다른 쪽이 남아 있어야 한다).
  //
  //   1. 쓸 곳이 있다(`onConfigChange`) — 없으면 저장할 데가 없다.
  //   2. **편집 중이다**(`edit.active`) — 보기만 하는 사용자가 대시보드를 고쳐 쓰는 일은
  //      없어야 한다. 그리고 이 게이트는 **작업 영역·출력 영역 표시를 켜는 바로 그
  //      게이트**다(`workspace={edit.active}` 한 줄 아래). 둘이 같은 하나이므로 "표시 없이
  //      유도만 도는" 상태가 **형상 자체로 존재하지 않는다**(불변식 I15 · 위험 R13).
  //
  // ## 무진동은 "다시 돌지 않는다" 가 아니라 "다시 돌아도 같은 값" 이다
  //
  // 이 콜백의 신원은 `cfg.canvas` 에 매여 있으므로, 쓰기가 config 를 갈면 표면의 통보
  // 효과가 **같은 `outer` 로 다시 돈다**. 그때 진동을 막는 것은 유도가 **잰 상자만의
  // 함수**(항등)라는 사실이다 — 같은 상자에 두 번 물으면 같은 수가 나오고, 그러면
  // `derivedCanvasSize` 가 **받은 객체를 그대로** 돌려주어 아래 참조 비교가 쓰기를 삼킨다
  // (불변식 I19 · 가정 A17). **저장된 크기를 유도의 입력으로 되먹이지 말 것** — 그 순간
  // 쓰기가 다음 유도의 입력이 되어 리사이즈 한 번이 연쇄 쓰기가 된다.
  const handleStageMeasured = useCallback(
    (outer: StageSize) => {
      if (onConfigChange === undefined || !edit.active) return;
      const stored: CanvasSize = cfg.canvas;
      const derived = derivedCanvasSize(stored, outer);
      // 같은 참조 = 바꿀 것이 없다(잴 수 없는 상자이거나 이미 맞아 있다). **새 비교를
      // 만들지 않는다** — 파서가 매 렌더 새 객체를 내므로 값 비교로는 억제되지 않는다.
      if (derived === stored) return;
      onConfigChange({ canvas: derived });
    },
    [onConfigChange, edit.active, cfg.canvas],
  );

  /**
   * 잰 바깥 상자를 편집기로 흘려보낸다(`canvasStageAspect`). provider 가 없으면(대시보드에
   * 놓인 패널) 무동작이며 참조도 고정이라 아무 효과도 다시 돌지 않는다.
   */
  const publishStageAspect = useCanvasStageAspectPublisher();

  /** 표면이 크기를 잴 때마다 부르는 하나의 통로 — 발행과 자동 맞춤이 같은 값을 본다. */
  const handleStage = useCallback(
    (outer: StageSize) => {
      publishStageAspect(outer);
      handleStageMeasured(outer);
    },
    [publishStageAspect, handleStageMeasured],
  );

  /**
   * 표면의 오버레이 슬롯. 투영 한 벌(스테이지 px + 캔버스 단위 크기)과 실측 글자 폭은
   * **표면이 든 것을 그대로** 받아 넘긴다(측정원이 하나다 — AC-E2).
   *
   * 이 렌더 prop 은 표면의 어느 효과 의존성에도 들어가지 않으므로 프레임을 예약하지
   * 않는다. 선택·호버가 루프를 깨우지 않는다는 보장이 여기서 성립한다(AC-E4).
   */
  const renderOverlay = useCallback(
    ({ projection, textWidths }: CanvasOverlayContext) => (
      <>
        {/*
          빈 상태 안내(AC-E1 · AC-E9). 표면의 컨테이너가 이미 `relative` 이므로 이 층은
          `absolute inset-0` 하나로 캔버스와 정확히 같은 상자를 덮는다 — 제목 줄까지 덮지
          않는다.

          **포인터를 먹지 않는다.** `pointer-events-none` 이 없으면 이 안내가 곧 편집을
          막는 유리판이 되어, 팔레트 버튼도 캔버스 누름도 안내에 걸린다 — 고치려던 결함이
          모양만 바꿔 되돌아온다. 오버레이보다 **먼저** 놓아 팔레트가 위에 오게 한다.
        */}
        {isEmpty && (
          <div
            data-testid="canvas-empty"
            className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-2 rounded-md border border-dashed border-(--color-border-default) p-4 text-center"
          >
            <Shapes className="h-6 w-6 text-(--color-text-muted)" />
            <span className="text-sm text-(--color-text-muted)">
              {t('dashboard.canvas.emptyState')}
            </span>
          </div>
        )}
        <CanvasEditOverlay
          enabled={edit.active}
          elements={cfg.elements}
          projection={projection}
          textWidths={textWidths}
          onElementsChange={handleElementsChange}
        />
      </>
    ),
    [isEmpty, t, edit.active, cfg.elements, handleElementsChange],
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

      {/* 표면은 **언제나** 있다. 오류 상태에서도 내리지 않고(AC-E4), 요소가 0개여도 내리지
          않는다(AC-E9) — 빈 안내는 위 `renderOverlay` 안에서 겹치는 층으로 나온다. */}
      <CanvasSurface
        elements={cfg.elements}
        canvas={cfg.canvas}
        targetStyles={frame.targetStyles}
        texts={frame.texts}
        panelTween={cfg.tween}
        background={cfg.background}
        visibilitySource={visibilitySource}
        scheduler={scheduler}
        overlay={renderOverlay}
        onStageMeasured={handleStage}
        // 작업 영역은 **편집 게이팅에 얹힌다**(SPEC-CANVAS-006 M4). 새 토글을 두지 않는
        // 것에 뜻이 있다 — 세 겹 게이팅이 이미 "지금 편집 중인가" 를 소유하므로, 넷째
        // 개념이 생기면 사용자가 패널마다 다른 규칙을 배운다. 그리고 이 한 게이트가
        // 뒤에 설 항상 맞춤(M7)과 **같은 게이트**여야 표시 없이 맞춤만 도는 상태가 형상
        // 자체로 존재하지 않는다(불변식 I15 · 위험 R13).
        //
        // 꺼져 있으면 `workspaceBox` 가 `stageLattice` 결과를 그대로 옮겨 담아 상자 둘이
        // 겹치므로 대시보드의 그림은 한 픽셀도 달라지지 않는다(REQ-05 · AC-06).
        workspace={edit.active}
      />

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
