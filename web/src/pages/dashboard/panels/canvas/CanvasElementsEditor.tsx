// 캔버스 요소 목록 편집기 (SPEC-CANVAS-001 T8 · REQ-01(6) · REQ-02 · REQ-03 · REQ-04).
//
// 001 에서 사용자는 요소의 기하를 **수치로** 저술한다(가정 A4). 캔버스 위 드래그 배치·
// 크기 조절·스냅은 002 의 몫이므로 여기에는 없다. 그래서 이 화면의 어휘는 "숫자 칸과
// 선택 상자" 이며, 배열 순서가 곧 그리기 순서(뒤가 위)라는 사실을 **화면에 적어** 순서
// 단추가 왜 z-order 인지 사용자가 알 수 있게 한다 — z-order 수단은 배열 순서 하나뿐이고,
// 그것을 움직이는 규칙도 `canvasEditArrange.moveElementTo` 하나뿐이다.
//
// 펼친 요소 카드는 **탭 넷**으로 선다(스타일 · 텍스트 · 배치 · 데이터). 그 넷은 새 축이
// 아니라 이미 있던 묶음을 가른 것이며, 어느 탭이 열려 있는지는 config 에 적지 않는다 —
// 자세한 근거는 아래 §요소 카드의 네 갈래(탭)와 `activeTab` 주석에 있다.
//
// 본문이 `PanelSettingsDialog.tsx`(8,250행+)가 아니라 여기 있는 이유는 §위험 R4 다.
// 다이얼로그에는 `<CollapsibleSection>` 마운트 지점만 두고, 규칙 표는 이 편집기와도
// 분리된 컴포넌트(`CanvasRuleTableEditor`)로 둔다 — 요소 편집과 조건 저술은 서로 다른
// 관심사이고, 한 파일에 합치면 R4 를 다이얼로그에서 이 파일로 옮기기만 한 셈이 된다.
//
// 이 편집기가 지키는 규율 셋:
//   1. **빈 칸은 부재다.** 비운 옵셔널 칸을 `''` 나 `0` 으로 직렬화하면 사용자가 지정한
//      적 없는 값이 config 에 박힌다(`CanvasRuleTableEditor.setOrDelete` 와 같은 규율).
//   2. **저장 왕복에 값이 바뀌지 않는다.** 여기서 내보낸 config 를 `parseCanvasConfig`
//      로 읽으면 같은 것이 나와야 한다 — 그래서 파서가 죄는 범위(0..1 불투명도, 비음수
//      두께·글자 크기, 정수 소수 자리)를 편집기가 먼저 지킨다.
//   3. **바인딩은 동일성 키로 참조한다.** 표시 이름이 아니다 — 이름을 바꾸면 바인딩이
//      끊기기 때문이며, 이는 히트맵이 센서 좌표를 `storeSeriesId` 로 재키잉한 것과 같은
//      판단이다(`sensorIdentity.ts`).
//
// @spec SPEC-CANVAS-001

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import {
  ChevronDown,
  ChevronRight,
  ChevronsDown,
  ChevronsUp,
  ChevronUp,
  Trash2,
} from 'lucide-react';

import { FieldHelp } from '@/components/property/FieldHelp';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import ColorSwatchButton from '../../colorSwatchPalette';
import {
  storeSeriesId,
  storeSeriesLabel,
  type ChartDataSourceKind,
  type StoreSourceConfig,
  type SysmetricsSourceConfig,
  type TsdbSourceConfig,
} from '../charts/chartChannelTypes';
import { nextSelection } from '../charts/panelEditSelection';
import { toStoreShapedConfig } from '../charts/useSysMetricsChartData';
import {
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_TWEEN_EASING,
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  MIN_ELEMENT_EXTENT,
  isNumericElement,
  parseCanvasConfig,
  type BoxGeometry,
  type CanvasElement,
  type CanvasElementKind,
  type CanvasSize,
  type ElementAlign,
  type ElementFontWeight,
  type ElementStyle,
  type Geometry,
  type LineGeometry,
  type PointGeometry,
  type RuleRow,
  type TweenEasing,
  type TweenSpec,
} from './canvasConfig';
import { bringToFront, moveElementTo, sendToBack } from './canvasEditArrange';
import {
  useCanvasEditSelection,
  useCanvasLiveSeries,
  type CanvasSeriesOption,
} from './canvasEditContext';
import { withElementText } from './canvasElementFactory';
import { fitCanvasSizeToStage } from './canvasGeometry';
import { useCanvasStageAspect } from './canvasStageAspect';
import CanvasRuleTableEditor from './CanvasRuleTableEditor';

// --- 상수 ---------------------------------------------------------------

/** 도형 원시형 4종. 표시 순서는 §명세의 나열 순서를 따른다. */
const ELEMENT_KINDS: readonly CanvasElementKind[] = ['rect', 'ellipse', 'line', 'text'];

/** 이징 4종. */
const TWEEN_EASINGS: readonly TweenEasing[] = ['linear', 'ease-in', 'ease-out', 'ease-in-out'];

/** 종류 라벨 키. 리터럴 맵으로 두어야 어떤 키가 쓰이는지 검색으로 확인된다. */
const KIND_LABEL_KEY: Record<CanvasElementKind, string> = {
  rect: 'dashboard.canvas.elements.kindRect',
  ellipse: 'dashboard.canvas.elements.kindEllipse',
  line: 'dashboard.canvas.elements.kindLine',
  text: 'dashboard.canvas.elements.kindText',
};

// --- 요소 카드의 네 갈래(탭) ---------------------------------------------
//
// 한 요소가 지닌 축은 넷으로 갈린다 — **무엇으로 그리는가(스타일) · 무엇을 적는가
// (텍스트) · 어디에 얼마만큼 놓는가(배치) · 무엇을 읽어 어떻게 반응하는가(데이터)**.
// 묶음 일곱을 세로로 쌓던 동안에는 그 넷이 한 두루마리로 이어져, 한 축을 고치려면
// 다른 셋을 지나쳐야 했다.
//
// **탭은 재배치일 뿐이다.** 새로 생긴 축도, config 에 새로 적히는 값도 없다 — 이미
// 있던 묶음이 넷으로 나뉘어 설 뿐이며, 어느 탭이 열려 있는지는 펼침 여부와 같은 부류의
// **보기 상태**라 config 에 넣지 않는다(아래 `activeTab` 주석).
type ElementTab = 'style' | 'text' | 'arrange' | 'data';

/** 탭의 화면 차례. 참고 화면의 스타일 · 텍스트 · 배치에 데이터를 더한 넷이다. */
const ELEMENT_TABS: readonly ElementTab[] = ['style', 'text', 'arrange', 'data'];

/** 탭 라벨 키. 리터럴 맵으로 두어야 어떤 키가 쓰이는지 검색으로 확인된다. */
const TAB_LABEL_KEY: Record<ElementTab, string> = {
  style: 'dashboard.canvas.elements.tabStyle',
  text: 'dashboard.canvas.elements.tabText',
  arrange: 'dashboard.canvas.elements.tabArrange',
  data: 'dashboard.canvas.elements.tabData',
};

/** 이징 라벨 키. */
const EASING_LABEL_KEY: Record<TweenEasing, string> = {
  'linear': 'dashboard.canvas.elements.easingLinear',
  'ease-in': 'dashboard.canvas.elements.easingIn',
  'ease-out': 'dashboard.canvas.elements.easingOut',
  'ease-in-out': 'dashboard.canvas.elements.easingInOut',
};

// --- 기하 축의 두 갈래: **위치**와 **크기** ------------------------------
//
// 한 덩어리로 늘어놓던 좌표 칸들을 두 묶음으로 가른다. 가르는 선은 새로 지어낸 것이
// 아니라 `canvasEditGeometry.handlesFor` 가 이미 종류마다 다르게 그어 둔 그 선이다 —
// 캔버스에서 **옮기는 손잡이**와 **크기를 바꾸는 손잡이**가 갈리는 자리와 목록에서
// 칸이 갈리는 자리가 같아야, 두 화면이 같은 모델을 말한다.
//
// 종류마다 비대칭인 것이 요점이다:
//   - rect · ellipse: 위치(x·y) + 크기(w·h).
//   - line: 두 끝점뿐이다. **크기 묶음이 없다** — 선의 길이는 두 끝점에서 따라 나오는
//     값이지 따로 저술하는 값이 아니다. 없는 묶음을 만들어 보이면 사용자는 그 칸을 찾다
//     못 찾는다.
//   - text: 기준점뿐이다. 문구의 "크기" 는 글자 크기이며 그것은 문구 스타일 묶음에 있다.
//     여기에 빈 크기 묶음을 두면 글자 크기를 두 자리에서 찾게 된다.

/** rect · ellipse 의 위치 축. */
const BOX_POSITION_AXES = ['x', 'y'] as const;
/** rect · ellipse 의 크기 축. */
const BOX_SIZE_AXES = ['w', 'h'] as const;
/** line 의 기하 축 — 전부 위치다(두 끝점). */
const LINE_AXES = ['x1', 'y1', 'x2', 'y2'] as const;
/** text 의 기하 축 — 기준점뿐이다. */
const POINT_AXES = ['x', 'y'] as const;

/**
 * 입력 공통 클래스(규칙 표 행과 같은 치수).
 *
 * 글자 크기는 주변 설정 화면의 기본값(`text-xs` = 12px)이다. 이 편집기만 11px 이하로
 * 적혀 있어 같은 다이얼로그 안에서 유독 작게 보였다 — 옆 절(`ChartPanelSections`)은
 * `text-xs` 를 기본으로 쓰고 9px 는 한 군데도 쓰지 않는다.
 */
const INPUT_CLASS =
  'min-w-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 ' +
  'text-xs text-(--color-text-primary) outline-none focus:border-blue-500';

/** 순서 이동·삭제 아이콘 버튼 공통 클래스. */
const ICON_BUTTON_CLASS =
  'shrink-0 text-(--color-text-muted) hover:text-(--color-text-secondary) disabled:opacity-30';

/**
 * 순서 조작 단추 공통 클래스 — 아이콘과 글자를 함께 이고 눌리는 면적을 갖는다.
 *
 * 아이콘만 두지 않는 이유는 넷이 서로 **한 칸**과 **끝까지** 로만 갈리기 때문이다.
 * 겹화살표 하나로 그 차이를 말하려면 사용자가 아이콘 관례를 이미 알고 있어야 하고,
 * 모르면 눌러 보고 되돌리는 수밖에 없다 — 순서는 되돌리기가 눈에 띄지 않는 조작이다.
 */
const ORDER_BUTTON_CLASS =
  'inline-flex shrink-0 items-center gap-0.5 rounded border border-(--color-border-default) ' +
  'px-1.5 py-0.5 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-hover) ' +
  'disabled:cursor-not-allowed disabled:opacity-40';

/** 소구획 제목 클래스. 칸의 이름이므로 본문과 같은 크기다. */
const GROUP_LABEL_CLASS = 'shrink-0 text-xs font-medium text-(--color-text-muted)';

/**
 * 요소 카드 안 묶음의 제목 클래스 — **제 줄에 서는 제목**이다.
 *
 * 위 `GROUP_LABEL_CLASS`(패널 축의 인라인 라벨)보다 진하고 조금 더 밝다. 같은 크기(12px)를
 * 쓰면서 굵기·밝기만 올리는 것이 요점이다: 제목을 키우면 카드가 제목의 벽이 되고, 흐리게
 * 두면 제 줄에 서 있어도 칸 사이에 묻혀 묶음의 시작이 보이지 않는다.
 */
const GROUP_HEADING_CLASS = 'shrink-0 text-xs font-semibold text-(--color-text-secondary)';

/** 안내문 클래스 — 본문보다 한 단계 작은 부차 문구다(9px 는 쓰지 않는다). */
const HINT_CLASS = 'px-1 text-[11px] leading-tight text-(--color-text-muted)';

/**
 * 순번 배지. 11px 두 자리가 들어가야 하므로 `h-4 w-4`(16px) 로는 좁다 —
 * 글자를 키우면 상자도 함께 키운다.
 */
const ORDER_BADGE_CLASS =
  'flex h-5 min-w-5 shrink-0 items-center justify-center rounded px-0.5 ' +
  'bg-(--color-bg-elevated) text-[11px] tabular-nums text-(--color-text-muted)';

// --- 타입 ---------------------------------------------------------------

export interface CanvasElementsEditorProps {
  /**
   * 패널 config **원본**(불투명 JSON). 그리기용 view 는 여기서 `parseCanvasConfig` 로
   * 만든다 — 파싱한 객체를 밖에서 받으면 시리즈 목록을 만들 소스 블록이 사라진다.
   */
  config: Record<string, unknown>;
  /** 최상위 얕은 병합 패치(다이얼로그의 `handleConfigChange` 규약). */
  onConfigChange: (patch: Record<string, unknown>) => void;
}

/**
 * 바인딩 선택지 한 줄 — 값은 판독값 키, 표시는 사람이 읽는 이름이다.
 *
 * 형상은 라이브 채널이 소유한다(`canvasEditContext`). 같은 형상을 두 번 적으면 두 목록이
 * 한 `<select>` 에 섞여 들어가는 이 자리에서 조용히 갈라질 수 있다.
 */
type SeriesOption = CanvasSeriesOption;

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 데이터 소스 config 에서 바인딩 선택지를 만든다 — **살아 있는 패널이 없을 때의 폴백**이다.
 *
 * 값(=`binding.series`)은 `CanvasPanel` 이 판독값을 키잉한 공간이어야 하는데, **config 만
 * 보고서는 그 공간을 알 수 없다.** 참조 하나가 태그 필터로 여러 컬럼으로 펼쳐지면
 * `CanvasPanel.resolveSeriesReadings` 는 정렬을 포기하고 조회 이름을 키로 쓰지만, 여기서는
 * 아래처럼 여전히 동일성 키를 낸다 — 그 어긋남이 곧 "골랐는데 값이 안 나온다" 였다.
 *
 * 그래서 지금 이 함수의 자리는 **최선 추정 폴백**이다. 살아 있는 미리보기가 자기 키 집합을
 * 내놓았다면(`useCanvasLiveSeries`) 그쪽이 이긴다. 이 함수는 미리보기가 아직 첫 조회를
 * 마치지 않았거나(설정 창을 막 열었을 때) 곁에 미리보기가 아예 없을 때만 쓰인다.
 *
 * 폴백이 내는 공간은 소스마다 다르다:
 *   - **store**: 시리즈 동일성 키 `storeSeriesId(key, field, tags)`. 히트맵이 센서 좌표를
 *     키잉하는 공간과 같다(`sensorIdentity.heatmapSensorId`) — 표시 이름(alias)을 바꿔도
 *     바인딩이 끊기지 않는 이유다.
 *   - **tsdb · sysmetrics**: `store_source.series` 가 비어 정렬이 성립하지 않으므로
 *     `CanvasPanel` 은 **조회 이름**을 그대로 동일성 키로 쓴다. 그 이름은 훅이
 *     `storeSeriesLabel` 한 곳에서 만들므로(`useStoreChartData`), 여기서도 같은 함수로
 *     만든다. sysmetrics 는 조회 전에 store 형상으로 옮겨지므로 그 변환을 먼저 태운다.
 *
 * 목록은 최선 추정이다 — 태그 바인딩 모드처럼 항목 하나가 런타임에 N개로 펼쳐지는
 * 경우까지 config 만으로 알 수는 없다. 그래서 저장된 바인딩이 목록에 없어도 지우지
 * 않고 별도 항목으로 되살린다(아래 `bindingOptionsFor`).
 */
function buildSeriesOptions(config: Record<string, unknown>): SeriesOption[] {
  const kind = (config.data_source as ChartDataSourceKind | undefined) ?? 'store';
  const out: SeriesOption[] = [];

  if (kind === 'tsdb') {
    const src = config.tsdb_source as TsdbSourceConfig | undefined;
    for (const ref of src?.series ?? []) {
      const label = storeSeriesLabel(ref, src?.series_name_format);
      out.push({ id: label, label });
    }
  } else if (kind === 'sysmetrics') {
    const shaped = toStoreShapedConfig(config.sysmetrics_source as SysmetricsSourceConfig | undefined);
    for (const ref of shaped?.series ?? []) {
      const label = storeSeriesLabel(ref, shaped?.series_name_format);
      out.push({ id: label, label });
    }
  } else {
    const src = config.store_source as StoreSourceConfig | undefined;
    for (const ref of src?.series ?? []) {
      out.push({
        id: storeSeriesId(ref.key, ref.field ?? '', ref.tags ?? {}),
        label: storeSeriesLabel(ref, src?.series_name_format),
      });
    }
  }

  // 같은 동일성 키가 두 번 나오면 앞선 것이 이긴다 — `CanvasPanel` 의 판독값 병합 규칙과
  // 같다(뒤 컬럼은 같은 시리즈의 중복이다).
  const seen = new Set<string>();
  return out.filter((o) => (seen.has(o.id) ? false : (seen.add(o.id), true)));
}

/**
 * 저장된 바인딩이 목록에 없으면 그 키를 선택지로 되살린다.
 *
 * 없는 값을 `<select>` 에 두면 브라우저가 첫 항목(= 바인딩 없음)을 대신 고른 것처럼
 * 그려, 사용자가 손대지 않은 바인딩이 화면에서 사라진 것으로 보인다. 소스를 잠시
 * 바꿨다가 되돌리는 흔한 편집 도중에도 그렇다.
 */
function bindingOptionsFor(options: readonly SeriesOption[], current: string | undefined): SeriesOption[] {
  if (current === undefined || options.some((o) => o.id === current)) return [...options];
  return [...options, { id: current, label: current }];
}

/** 기하가 rect/ellipse 형상인가. */
function isBox(g: Geometry): g is BoxGeometry {
  return 'w' in g;
}

/** 기하가 line 형상인가. */
function isLine(g: Geometry): g is LineGeometry {
  return 'x1' in g;
}

/**
 * 기하를 새 종류가 요구하는 형상으로 옮긴다.
 *
 * 기준점(좌상단 · 첫 끝점 · 정렬 기준)은 **살리고**, 대응이 없는 칸만 기본값으로 채운다.
 * 종류를 바꿨다고 요소가 화면 반대편으로 튀면 사용자는 그것을 "종류 변경" 이 아니라
 * "요소가 사라짐" 으로 읽는다.
 */
function toBoxGeometry(g: Geometry): BoxGeometry {
  if (isBox(g)) return g;
  if (isLine(g)) return { x: g.x1, y: g.y1, w: DEFAULT_BOX_GEOMETRY.w, h: DEFAULT_BOX_GEOMETRY.h };
  return { x: g.x, y: g.y, w: DEFAULT_BOX_GEOMETRY.w, h: DEFAULT_BOX_GEOMETRY.h };
}

/** 두 끝점 형상으로 옮긴다(위 규율 동일). */
function toLineGeometry(g: Geometry): LineGeometry {
  if (isLine(g)) return g;
  const anchor = isBox(g) ? { x: g.x, y: g.y } : g;
  return {
    x1: anchor.x,
    y1: anchor.y,
    x2: DEFAULT_LINE_GEOMETRY.x2,
    y2: DEFAULT_LINE_GEOMETRY.y2,
  };
}

/** 기준점 형상으로 옮긴다(위 규율 동일). */
function toPointGeometry(g: Geometry): PointGeometry {
  if (isBox(g)) return { x: g.x, y: g.y };
  if (isLine(g)) return { x: g.x1, y: g.y1 };
  return g;
}

/**
 * 요소의 종류를 바꾼다. **스타일 · 문구 · 바인딩 · 규칙은 그대로 남는다.**
 *
 * `CanvasElement` 는 `kind` 로 판별하는 합집합이라, 동적으로 받은 종류로 요소를 만들려면
 * 객체 리터럴 하나가 아니라 switch 가 필요하다 — 리터럴 하나로는 `kind` 와 `geometry` 의
 * 짝을 컴파일러가 확인할 수 없다.
 */
function withKind(el: CanvasElement, kind: CanvasElementKind): CanvasElement {
  const { geometry: _dropped, kind: _prevKind, ...rest } = el;
  switch (kind) {
    case 'rect':
      return { ...rest, kind, geometry: toBoxGeometry(el.geometry) };
    case 'ellipse':
      return { ...rest, kind, geometry: toBoxGeometry(el.geometry) };
    case 'line':
      return { ...rest, kind, geometry: toLineGeometry(el.geometry) };
    default:
      return { ...rest, kind, geometry: toPointGeometry(el.geometry) };
  }
}

/**
 * 씨앗 스타일·계단 오프셋·id 규칙은 **`canvasElementFactory.ts` 가 소유한다**
 * (SPEC-CANVAS-002 T9). 캔버스 도형 팔레트가 같은 것을 만들어야 하므로 두 호출부가 한
 * 구현을 부른다 — 규칙이 둘이 되면 "어디서 더했는가" 에 따라 결과가 달라진다(가정 A7).
 *
 * **문구 템플릿 편집도 같은 모듈을 지난다**(`withElementText`). 도형에 라벨이 생기는
 * 순간 글자색을 심는 규칙이 거기 있고, 여기에 사본을 두지 않는다 — 씨앗 규칙과 같은 이유다.
 */

/**
 * 기하 좌표 입력 파싱. 빈 칸·비수치는 0 으로 본다(규칙 표 임계값과 같은 규율).
 *
 * **캔버스 안으로 자르지 않는다.** 캔버스 밖으로 일부 걸치는 배치도 뜻이 있는 저술이며,
 * 렌더러가 그것을 허용한다(파서의 좌표 정책과 같은 이유). 다만 두 가지는 막는다 —
 * 비유한 값(NaN 이 기하에 들어가면 그 요소는 화면에서 통째로 사라진다)과 **소수 자리**
 * (좌표계가 정수라 저장할 곳이 없다. 파서가 어차피 반올림하므로 여기서 먼저 반올림해야
 * 저장 왕복에 값이 바뀌지 않는다 — 규율 2).
 */
function parseCoordinate(raw: string): number {
  const n = Number(raw.trim());
  return Number.isFinite(n) ? Math.round(n) : 0;
}

/**
 * 크기 축(w · h)의 입력 파싱. 좌표와 같되 **최소 크기 아래로 내려가지 않는다**.
 *
 * 파서는 폭·높이 0 인 기하를 퇴화로 보고 씨앗 기하로 되살린다. 그 되살림이 옳은 것은
 * 옛 config 를 읽을 때이고, 사용자가 칸을 비우는 순간 요소가 화면 반대편으로 순간이동하는
 * 것은 옳지 않다. 그래서 쓰는 쪽이 애초에 만들지 않는다 — 캔버스 손잡이가 같은 이유로
 * 같은 하한을 지킨다(`canvasEditGeometry` §기하 쓰기 단일 통로).
 */
function parseExtent(raw: string): number {
  return Math.max(MIN_ELEMENT_EXTENT, parseCoordinate(raw));
}

/**
 * 캔버스 한 축 크기의 입력 파싱. 비운 칸은 **부재가 아니라 최소값**이다 — 캔버스 크기에는
 * "지정 안 함" 이 없고(투영이 언제나 요구한다), 0 축은 모든 요소를 화면에서 지운다.
 */
function parseCanvasDimension(raw: string): number {
  const n = Math.round(Number(raw.trim()));
  if (!Number.isFinite(n)) return MIN_CANVAS_DIMENSION;
  return Math.min(Math.max(n, MIN_CANVAS_DIMENSION), MAX_CANVAS_DIMENSION);
}

/** 옵셔널 수치 입력 파싱. **빈 칸은 부재**이며 0 이 아니다. */
function parseOptionalNumber(raw: string): number | undefined {
  const v = raw.trim();
  if (v === '') return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

/** 비음수로 죈다(파서가 음수를 폴백으로 바꾸므로 여기서 먼저 지킨다 — 규율 2). */
function nonNegative(v: number | undefined): number | undefined {
  return v === undefined ? undefined : Math.max(0, v);
}

/** 0..1 로 죈다(파서의 불투명도 clamp 와 같은 범위 — 규율 2). */
function clamp01(v: number | undefined): number | undefined {
  return v === undefined ? undefined : v < 0 ? 0 : v > 1 ? 1 : v;
}

/** 소수 자리는 비음수 정수다(파서의 `Math.trunc` 와 같은 규율 — 규율 2). */
function decimalsOf(v: number | undefined): number | undefined {
  return v === undefined ? undefined : Math.max(0, Math.trunc(v));
}

/** 빈 문자열을 부재로 접는다(규율 1). 공백만 있는 값도 파서가 버리므로 여기서 접는다. */
function optionalText(raw: string): string | undefined {
  return raw.trim() === '' ? undefined : raw;
}

/** 스타일 키 하나를 갈아끼우되, 부재면 키 자체를 지운다(규율 1). */
function setStyleField<K extends keyof ElementStyle>(
  style: ElementStyle,
  key: K,
  value: ElementStyle[K],
): ElementStyle {
  const next: ElementStyle = { ...style };
  if (value === undefined) delete next[key];
  else next[key] = value;
  return next;
}

/** 요소 필드 하나를 갈아끼우되, 부재면 키 자체를 지운다(규율 1). */
function setElementField<K extends keyof CanvasElement>(
  el: CanvasElement,
  key: K,
  value: CanvasElement[K] | undefined,
): CanvasElement {
  const next = { ...el };
  if (value === undefined) delete next[key];
  else next[key] = value;
  return next;
}

/** `{index}` 자리를 1-기반 번호로 채운다(규칙 표 aria 문구 규약과 동일). */
function withIndex(label: string, idx: number): string {
  return label.replace('{index}', String(idx + 1));
}

/**
 * 접힌 줄의 한 줄 요약.
 *
 * 접기의 값은 "덜 보기" 가 아니라 "목록을 훑을 수 있음" 이다. 그래서 접혀도 각 줄은
 * 자기 정체성(id) · 살아 있는지(바인딩) · 조건이 몇 줄인지를 계속 말한다 — 종류는 바로
 * 옆 선택 상자가 이미 말하고 있으므로 여기서 되풀이하지 않는다.
 *
 * 펼쳐도 같은 문구를 유지한다. 펼칠 때만 사라지면 머리줄의 폭이 바뀌어 옆 버튼들이
 * 자리를 옮기고, 그러면 연달아 접었다 펴는 동안 삭제 버튼이 손 밑에서 움직인다.
 */
function summaryOf(el: CanvasElement, t: TranslationFn): string {
  const parts = [
    el.id,
    el.binding === undefined
      ? t('dashboard.canvas.elements.summaryStatic')
      : t('dashboard.canvas.elements.summaryBound'),
  ];
  const ruleCount = el.rules?.length ?? 0;
  if (ruleCount > 0) {
    parts.push(t('dashboard.canvas.elements.summaryRules').replace('{count}', String(ruleCount)));
  }
  return parts.join(' · ');
}

// --- 하위 표현 -----------------------------------------------------------

/**
 * 기하 좌표 한 칸. 축 이름을 눈에 보이게 붙여 어느 칸이 무엇인지 알 수 있게 한다.
 *
 * **정수 칸이다**(`step=1`). 좌표계가 정수이므로 화살표 한 번이 곧 저장되는 한 단위이며,
 * 소수를 적어 넣어도 파싱이 반올림한다 — 브라우저의 `step` 검증에 기대지 않는 것은
 * 붙여넣기·IME 처럼 검증을 지나치는 입력 경로가 있기 때문이다.
 *
 * `extent` 인 칸(폭·높이)만 하한을 갖는다. 위치는 음수도 캔버스 밖도 합법이므로 하한이
 * 없다 — 없는 하한을 `min={0}` 으로 적어 두면 그것이 곧 사용자 의도를 자르는 자리가 된다.
 */
function GeometryInput({
  axis,
  value,
  onChange,
  ariaLabel,
  testId,
  extent = false,
}: {
  axis: string;
  value: number;
  onChange: (next: number) => void;
  ariaLabel: string;
  testId: string;
  /** 이 칸이 **크기** 축인가. 참이면 최소 크기 아래로 내려가지 않는다. */
  extent?: boolean;
}) {
  return (
    <label className="flex min-w-0 flex-1 items-center gap-1">
      <span className="shrink-0 text-[11px] text-(--color-text-muted)">{axis}</span>
      <input
        type="number"
        step={1}
        {...(extent ? { min: MIN_ELEMENT_EXTENT } : {})}
        value={value}
        onChange={(e) => onChange(extent ? parseExtent(e.target.value) : parseCoordinate(e.target.value))}
        aria-label={ariaLabel}
        data-testid={testId}
        className={cn(INPUT_CLASS, 'w-full text-center tabular-nums')}
      />
    </label>
  );
}

/**
 * 캔버스 한 축 크기의 입력 칸.
 *
 * `GeometryInput` 과 나란한 모양이되 **부재가 없다**(위 `parseCanvasDimension`). 요소의
 * 좌표는 비워 둘 수 있는 값이 아니고 캔버스 크기는 더더욱 그렇다 — 이 두 정수가 모든
 * 좌표의 분모이므로, 값이 없는 순간 그릴 수 있는 것이 하나도 없다.
 */
function CanvasSizeInput({
  axis,
  value,
  onChange,
  ariaLabel,
  testId,
}: {
  axis: string;
  value: number;
  onChange: (next: number) => void;
  ariaLabel: string;
  testId: string;
}) {
  return (
    <label className="flex items-center gap-1">
      <span className="shrink-0 text-[11px] text-(--color-text-muted)">{axis}</span>
      <input
        type="number"
        step={1}
        min={MIN_CANVAS_DIMENSION}
        max={MAX_CANVAS_DIMENSION}
        value={value}
        onChange={(e) => onChange(parseCanvasDimension(e.target.value))}
        aria-label={ariaLabel}
        data-testid={testId}
        className={cn(INPUT_CLASS, 'w-16 text-center tabular-nums')}
      />
    </label>
  );
}

/**
 * 이름 붙은 칸 묶음 하나 — **제목이 제 줄, 칸이 그 아래 줄**.
 *
 * 제목과 칸을 한 줄에 늘어놓던 형태를 버린 이유는 취향이 아니라 줄바꿈이다. 칸은
 * `flex-wrap` 으로 흐르는데 제목이 같은 흐름 안에 있으면, 칸이 넘쳐 다음 줄로 내려간
 * 순간 그 칸은 **아래 묶음의 제목과 나란히** 선다 — 화면은 "크기 · [W] [H] [테두리]"
 * 처럼 읽히고 어느 칸이 어느 묶음의 것인지 눈으로 끊기지 않는다. 제목을 제 줄에
 * 세우면 그 모호함이 구조로 사라진다.
 *
 * 여덟 묶음(순서 · 크기 · 위치 · 도형 · 텍스트 · 데이터 · 전환 효과 · 규칙)이 네 탭에
 * 나뉘어 서지만 쓰는 것은 모두 이 하나다 — 묶음마다 제목 span 과 flex 클래스를 되풀이하면
 * 어느 묶음이 왜 다른 간격을 갖는지 알 수 없게 되고, 탭이 갈린 뒤에는 그 차이가 "탭마다
 * 다르게 생겼다" 로 보인다.
 */
function FieldGroup({
  label,
  testId,
  help,
  children,
}: {
  label: string;
  testId: string;
  /** 묶음 제목 뒤에 붙는 `?` 도움말. 없으면 아이콘도 없다. */
  help?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-1" data-testid={testId}>
      <div className="flex min-w-0 items-center gap-1">
        <span className={GROUP_HEADING_CLASS}>{label}</span>
        {help}
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">{children}</div>
    </div>
  );
}

/**
 * 요소 카드의 탭 줄 — **진짜 탭**이다(장식된 단추 넷이 아니다).
 *
 * 이 화면에 이미 있는 탭 관례를 그대로 따른다(`pages/admin/UserManagementPage.tsx`):
 * `role="tablist"` 는 좌우 화살표 이동을 **전제한 계약**이라 클릭만 처리하면 스크린리더
 * 사용자가 탭 사이를 옮길 방법이 없다. 그래서 셋을 함께 갖춘다 —
 *   - roving tabindex: 열린 탭만 탭 순서에 남는다(탭 줄 전체가 하나의 정지점이다),
 *   - 좌우 화살표로 이동하고 양끝에서 순환하며(WAI-ARIA tabs 패턴),
 *   - 옮긴 뒤 **초점도 함께 옮긴다** — 초점이 남으면 다음 화살표가 옮긴 자리가 아니라
 *     떠나온 자리에서 다시 세어 사용자가 두 칸씩 건너뛴 것처럼 느낀다.
 *
 * 공유 컴포넌트가 없어 관례만 가져온다. 저 파일의 탭은 라우터 질의를 쓰고 아이콘을 이고
 * 페이지 폭을 채우므로, 카드 안 좁은 자리에 그대로 옮길 수 있는 것이 아니다. 대신 겉모습은
 * 같은 화면의 세그먼트 컨트롤(`SeriesRangeField`)을 따라 카드 폭에 맞춘다.
 */
function ElementTabs({
  idx,
  active,
  onSelect,
  label,
  labelOf,
}: {
  /** 요소 순번. DOM id 와 testId 를 요소마다 갈라 준다(같은 화면에 여러 카드가 선다). */
  idx: number;
  active: ElementTab;
  onSelect: (tab: ElementTab) => void;
  /** 탭 줄 자체의 이름(`aria-label`). */
  label: string;
  labelOf: (tab: ElementTab) => string;
}) {
  const refs = useRef(new Map<ElementTab, HTMLButtonElement>());

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>, i: number): void => {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    e.preventDefault();
    const delta = e.key === 'ArrowRight' ? 1 : -1;
    const next = ELEMENT_TABS[(i + delta + ELEMENT_TABS.length) % ELEMENT_TABS.length];
    if (next === undefined) return;
    onSelect(next);
    refs.current.get(next)?.focus();
  };

  return (
    <div
      role="tablist"
      aria-label={label}
      data-testid={`canvas-element-tabs-${idx}`}
      className="flex min-w-0 gap-0.5 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-0.5"
    >
      {ELEMENT_TABS.map((tab, i) => {
        const selected = tab === active;
        return (
          <button
            key={tab}
            type="button"
            role="tab"
            id={`canvas-element-tab-${idx}-${tab}`}
            aria-selected={selected}
            aria-controls={`canvas-element-tabpanel-${idx}`}
            tabIndex={selected ? 0 : -1}
            ref={(node) => {
              if (node === null) refs.current.delete(tab);
              else refs.current.set(tab, node);
            }}
            onClick={() => onSelect(tab)}
            onKeyDown={(e) => onKeyDown(e, i)}
            data-testid={`canvas-element-tab-${tab}-${idx}`}
            className={cn(
              'min-w-0 flex-1 truncate rounded px-2 py-1 text-xs font-medium transition-colors',
              // 초점 테두리는 **보여야 한다** — 화살표 이동이 어디에 닿았는지 알 수 있는
              // 유일한 표시다. `focus-visible` 이라 마우스로 누를 때는 남지 않는다.
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500',
              selected
                ? 'bg-blue-600 text-white'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
            )}
          >
            {labelOf(tab)}
          </button>
        );
      })}
    </div>
  );
}

// --- 컴포넌트 -----------------------------------------------------------

/**
 * 캔버스 요소 목록 편집기.
 *
 * 저술 값에 대해서는 제어 컴포넌트다 — 값 상태를 들지 않고 매 편집마다 config 패치를
 * 올린다.
 *
 * **요소를 만드는 자리는 여기가 아니다.** 도형은 미리보기 왼쪽 도크의 팔레트
 * (`CanvasEditDock`)에서 놓는다 — 한때 이 목록 하단에도 종류별 추가 버튼 한 줄이
 * 있었지만, 팔레트가 이름과 누를 면적을 갖춘 뒤로는 같은 함수를 부르는 입구가 둘일
 * 이유가 없었다. 이 편집기는 **이미 있는 요소를 수치로 고치는 자리**다.
 *
 * 예외는 **펼침 여부** 하나다. 이것은 저술이 아니라 보기 상태라 config 에 넣지 않는다 —
 * 넣으면 한 사람이 줄을 접은 것이 대시보드를 함께 보는 모두의 저장된 값이 된다. 그래서
 * 지역 상태이고, 요소 **id** 로 키잉한다(순번으로 키잉하면 순서 이동·삭제가 남의 펼침
 * 상태를 물려받는다 — 바인딩을 이름이 아니라 동일성 키로 참조하는 것과 같은 이유다).
 */
export default function CanvasElementsEditor({ config, onConfigChange }: CanvasElementsEditorProps) {
  const { t } = useTranslation();

  const cfg = useMemo(() => parseCanvasConfig(config), [config]);
  const elements = cfg.elements;

  // --- 결함 D: 바인딩 선택지는 **살아 있는 패널이 낸 키 집합**을 먼저 쓴다 ---
  //
  // 미리보기 패널이 내놓은 목록은 추측이 아니라 **판독값을 실제로 키잉한 그 집합**이므로,
  // 여기서 고른 값은 정의상 값을 찾는다(`canvasEditContext` 의 라이브 시리즈 채널 주석).
  //
  // 비었을 때 config 로 떨어지는 것은 두 자리를 함께 덮는다: 곁에 미리보기가 없는 자리
  // (provider 없음 — 편집기 단독 렌더)와, 미리보기가 아직 첫 조회를 마치지 않은 순간이다.
  // 소스가 정말 비어 있으면 폴백도 비므로 "고를 것이 없다" 안내는 그대로 뜬다.
  const configSeriesOptions = useMemo(() => buildSeriesOptions(config), [config]);
  const liveSeriesOptions = useCanvasLiveSeries();
  const seriesOptions = liveSeriesOptions.length > 0 ? liveSeriesOptions : configSeriesOptions;

  // --- 0.10.0: 패널 비율에 맞춘 캔버스 크기 ---
  //
  // 패널 비율을 아는 것은 `ResizeObserver` 를 든 표면뿐이고 이 편집기는 다이얼로그의 다른
  // 자리에 있으므로, 살아 있는 패널이 내놓은 값을 그대로 받는다(`canvasStageAspect`).
  // 여기서 저장된 캔버스 비율로 패널 크기를 **역산**하면 그 추측이 곧 두 번째 출처가 되고,
  // 두 값이 갈라지는 날 "맞췄는데 여백이 남는다" 가 된다(위험 R1 과 같은 부류다).
  //
  // `null` 은 "맞출 것이 없다" 는 뜻이며 두 경우를 함께 덮는다: 곁에 미리보기가 없어
  // 아직 아무도 재지 않았거나(provider 없음 — 편집기 단독 렌더), 이미 맞아 있어
  // `fitCanvasSizeToStage` 가 받은 객체를 그대로 돌려준 경우다. 어느 쪽이든 누를 이유가
  // 없으므로 단추는 같은 조건으로 잠긴다.
  const stageOuter = useCanvasStageAspect();
  const fittedCanvas = useMemo<CanvasSize | null>(() => {
    if (stageOuter === null) return null;
    const next = fitCanvasSizeToStage(cfg.canvas, stageOuter);
    return next === cfg.canvas ? null : next;
  }, [cfg.canvas, stageOuter]);

  /** **사용자가 손으로** 펼쳐 둔 요소의 id 집합. 기본은 전부 접힘이다. */
  const [expandedIds, setExpandedIds] = useState<ReadonlySet<string>>(() => new Set());

  /**
   * 열린 탭 — **요소마다가 아니라 편집기 하나에 하나다(공유)**.
   *
   * 펼침 여부와 같은 부류의 보기 상태이므로 config 에 넣지 않는다(넣으면 한 사람이 연 탭이
   * 대시보드를 함께 보는 모두의 저장된 값이 된다). 그것과 갈리는 것은 **키잉**이다: 펼침은
   * 요소 id 로 키잉하는데, 탭은 그러지 않는다.
   *
   * 요소마다 따로 들면, 좌표를 줄줄이 손보려고 요소를 옮겨 다닐 때마다 카드가 **저마다 다른
   * 탭**으로 열린다 — 방금 배치를 보고 있었는데 다음 요소는 스타일로 열리고, 사용자는 매번
   * 같은 탭을 다시 고르게 된다. 하나로 두면 "배치를 보고 있다" 는 상태가 요소를 건너가도
   * 유지되고, 그 편이 놀라움이 적다. 대가는 한 요소에서 탭을 바꾸면 다른 카드도 함께
   * 바뀌는 것인데, 여러 카드를 동시에 펼쳐 두는 것 자체가 흔치 않고(캔버스 자동 펼침은
   * 언제나 한 줄뿐이다) 바뀐 결과가 화면에 그대로 보이므로 숨은 변화가 아니다.
   */
  const [activeTab, setActiveTab] = useState<ElementTab>('style');

  // --- SPEC-CANVAS-002 T10: 캔버스 선택 → 속성 편집 연동 ---
  //
  // **provider 가 없어도 동작한다.** 대시보드에 놓인 패널 곁에는 이 편집기가 없고, 이
  // 편집기만 단독으로 뜨는 자리(테스트·다이얼로그 밖)에서는 캔버스 선택이 없다. 그때
  // `useCanvasEditSelection` 은 로컬 선택으로 떨어지고 `autoExpandedId` 는 언제나 `null`
  // 이라, 아래 배선 전체가 조용히 무동작이 된다(canvasEditContext.tsx 의 계약).
  const { selection, setSelection, autoExpandedId } = useCanvasEditSelection();

  /**
   * **캔버스가 펼친 행** 하나. `expandedIds` 와 **따로** 든다 — 그래야 캔버스는 자기가
   * 펼친 것만 회수하고 사용자가 손으로 펼친 행은 건드리지 않는다(AC-06).
   *
   * 한 개뿐인 것에 뜻이 있다: 다음 선택이 오면 이 값이 통째로 갈리므로 이전 것이 저절로
   * 접힌다. 둘 이상 선택되면 `autoExpandedId` 가 `null` 이라 아무 행도 펼쳐지지 않는다 —
   * 펼침이 쌓이면 이미 고친 "설정이 모두 펼쳐져 복잡하다" 로 되돌아간다.
   */
  const [canvasExpandedId, setCanvasExpandedId] = useState<string | null>(null);

  /** 행 요소. 시야로 스크롤할 대상을 id 로 든다(순번으로 들면 순서 이동이 남을 가리킨다). */
  const rowRefs = useRef(new Map<string, HTMLDivElement>());

  /**
   * 캔버스 선택이 바뀔 때마다 펼침을 그 하나로 옮기고 시야로 스크롤한다.
   *
   * `scrollIntoView` 는 **jsdom 에 없다** — 그래서 옵셔널 호출이다. 없다고 배선이 죽으면
   * 안 되는 자리이고(펼침은 스크롤과 무관하게 일어나야 한다), 실제 브라우저에서는 늘 있다.
   */
  useEffect(() => {
    setCanvasExpandedId(autoExpandedId);
    if (autoExpandedId === null) return;
    rowRefs.current.get(autoExpandedId)?.scrollIntoView?.({ block: 'nearest' });
  }, [autoExpandedId]);

  /** 행이 펼쳐져 있는가 — 손으로 펼쳤거나(집합) 캔버스가 펼쳤거나(한 개) 둘 중 하나다. */
  const isExpanded = (id: string): boolean => expandedIds.has(id) || id === canvasExpandedId;

  /**
   * 행 머리줄을 눌렀을 때 — 펼침을 뒤집고, **펼치는 방향에서만** 그 요소를 고른다.
   *
   * 반대 방향(T10 은 캔버스 → 목록이었다)의 배선이 여기 한 줄이다. 같은 공유 컨텍스트를
   * 쓰므로 오버레이의 윤곽선·손잡이가 곧바로 그 요소로 옮겨 간다 — 선택의 출처가 둘이
   * 되면 "캔버스에서 고른 것" 과 "목록에서 고른 것" 이 서로 다른 것을 가리킬 수 있다.
   *
   * **왜 접는 방향에서는 고르지 않는가.** `autoExpandedId` 는 선택에서 **파생**된다
   * (`canvasEditContext.canvasAutoExpandedId`). 접으면서도 고르면 그 선택이 곧바로 같은
   * 행을 도로 펼치므로, 접기 단추가 접지 못하는 단추가 된다. 즉 "접으면서 고른다" 는
   * 파생 관계와 모순이다. 그리고 뜻으로 보아도 사용자가 요청한 것은 **"요소 설정을
   * 열면 그 요소가 골라진다"** 이지 "닫아도 골라진다" 가 아니다.
   *
   * 고르기는 오버레이와 **같은 규칙**(`nextSelection`)을 지난다. 이미 골라져 있으면 그
   * 함수가 **같은 참조**를 돌려주므로 `setSelection` 이 상태를 갈지 않고, 그래서 이
   * 되먹임 변이 다시 렌더를 부르지 않는다(진동 없음의 근거 절반이 이 한 줄이다).
   */
  const toggleExpanded = (id: string): void => {
    const open = isExpanded(id);
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (open) next.delete(id);
      else next.add(id);
      return next;
    });
    // 캔버스가 펼친 행을 손으로 접을 때는 그 자동 펼침도 함께 회수한다 — 회수하지 않으면
    // 파생 조건이 그대로 남아 도로 펼쳐지고, 사용자는 접을 방법이 없는 행을 갖게 된다.
    if (open && id === canvasExpandedId) setCanvasExpandedId(null);
    if (!open) {
      const picked = nextSelection(selection, id, false);
      if (picked !== selection) setSelection(picked);
    }
  };

  const emit = (next: CanvasElement[]): void => onConfigChange({ elements: next });

  const replaceAt = (idx: number, el: CanvasElement): void => {
    emit(elements.map((e, i) => (i === idx ? el : e)));
  };

  const removeAt = (idx: number): void => emit(elements.filter((_, i) => i !== idx));

  /**
   * 순서 이동 = z-order 조작. 001 의 유일한 z-order 수단이라 일급 동작이다.
   *
   * **규칙은 `canvasEditArrange.moveElementTo` 한 곳에 있다.** 캔버스 팔레트의 앞/뒤
   * 보내기(T14)도 같은 함수를 지나므로, "목록에서 눌렀는가 캔버스에서 눌렀는가" 에 따라
   * 결과가 달라질 수 없다(REQ-04 — 두 번째 정렬 규칙을 만들지 않는다). 여기서 인라인
   * splice 를 한 벌 더 들고 있던 동안에는 그 규율이 주석일 뿐이었다.
   *
   * 끝을 넘어서는 이동은 그 함수가 목표 위치를 배열 안으로 죄어 **제자리**로 만들고,
   * 제자리면 받은 배열을 그대로(같은 참조) 돌려준다 — 그 참조 비교가 곧 "쓸 일이 없다"
   * 이며, 이 함수가 예전에 `target < 0 || target >= length` 로 직접 세던 판정과 같다.
   *
   * 식별은 배열 위치가 아니라 `nodeId` 다(REQ-06).
   */
  const moveAt = (idx: number, delta: -1 | 1): void => {
    const el = elements[idx];
    if (el === undefined) return;
    const next = moveElementTo(elements, el.id, idx + delta);
    if (next === elements) return;
    emit([...next]);
  };

  /**
   * 맨 앞 · 맨 뒤로 보내기 — **위 `moveAt` 과 같은 규칙을 지난다**.
   *
   * `bringToFront` · `sendToBack` 은 `canvasEditArrange` 안에서 `moveElementTo` 로 지어져
   * 있으므로(같은 파일 §z-order), 여기서 두 번째 정렬 규칙이 생기지 않는다 — 네 동작이
   * 모두 한 함수를 지난다는 것이 REQ-04 가 요구하는 전부다. 캔버스 도크의 앞/뒤 보내기가
   * 부르는 것도 바로 이 두 함수다.
   *
   * 그 둘이 **집합**을 받는 것은 도크가 여러 개를 함께 보내기 때문이다. 목록에서는 한
   * 요소만 다루므로 한 원소짜리 집합을 넘긴다 — 무리의 상대 순서를 지키는 그 함수의 규율은
   * 원소가 하나면 아무 일도 하지 않는다.
   */
  const sendAt = (idx: number, toFront: boolean): void => {
    const el = elements[idx];
    if (el === undefined) return;
    const ids = new Set([el.id]);
    const next = toFront ? bringToFront(elements, ids) : sendToBack(elements, ids);
    if (next === elements) return;
    emit([...next]);
  };

  /**
   * 지속 시간이 트윈의 **존재 스위치**다 — 비우면 트윈 자체가 사라진다(요소에서는
   * "패널 기본 사용", 패널에서는 "즉시 전환"). 이징만으로는 트윈을 만들 수 없다:
   * 지속 시간 없는 이징은 뜻이 없고, 임의의 기본 지속 시간을 넣으면 사용자가 설정한
   * 적 없는 애니메이션이 생겨 렌더 루프가 깨어난다(REQ-05 유휴 정지).
   */
  const tweenWithDuration = (
    current: TweenSpec | undefined,
    duration: number | undefined,
  ): TweenSpec | undefined =>
    duration === undefined
      ? undefined
      : { duration_ms: duration, easing: current?.easing ?? DEFAULT_TWEEN_EASING };

  /** 이징 변경은 **이미 있는** 트윈만 고친다(위 존재 스위치 규율). */
  const tweenWithEasing = (
    current: TweenSpec | undefined,
    easing: TweenEasing,
  ): TweenSpec | undefined => (current === undefined ? undefined : { ...current, easing });

  return (
    <div className="space-y-3" data-testid="canvas-elements-editor">
      {/* 캔버스 크기 — **모든 좌표의 분모**다. 그래서 배경색·트윈보다 먼저 온다: 요소를
          놓기 전에 종이 크기를 정하는 것이 순서이고, 이 값이 격자 칸 수와 수치 칸의
          범위를 함께 정한다.

          요소 좌표를 함께 늘이지 않는다. 늘이면 정수 좌표에 반올림 오차가 쌓여 사용자가
          손으로 맞춰 둔 자리가 크기를 바꿀 때마다 조금씩 어긋나고, 그 어긋남은 되돌릴 수
          없다. 대신 캔버스가 넓어지면 요소가 상대적으로 작아 보인다 — 종이를 키운 것이지
          그림을 키운 것이 아니라는 뜻이며, 그것이 이 칸의 정직한 동작이다. */}
      <div className="flex flex-wrap items-center gap-2">
        <span className={GROUP_LABEL_CLASS}>{t('dashboard.canvas.elements.panelSize')}</span>
        <FieldHelp
          text={t('dashboard.canvas.elements.panelSizeHint')}
          testId="canvas-panel-size-hint"
        />
        <CanvasSizeInput
          axis="W"
          value={cfg.canvas.width}
          onChange={(w) => onConfigChange({ canvas: { ...cfg.canvas, width: w } })}
          ariaLabel={t('dashboard.canvas.elements.panelWidthAria')}
          testId="canvas-panel-width"
        />
        <CanvasSizeInput
          axis="H"
          value={cfg.canvas.height}
          onChange={(h) => onConfigChange({ canvas: { ...cfg.canvas, height: h } })}
          ariaLabel={t('dashboard.canvas.elements.panelHeightAria')}
          testId="canvas-panel-height"
        />
        {/* 패널 비율에 맞춤 — **사용자가 눌렀을 때만** 크기를 고친다(0.10.0).
            자동으로 다시 맞추지 않는 이유는 `CanvasPanel` §자동 맞춤은 한 번뿐이다에
            있다: 저장된 요소 좌표는 절대 캔버스 단위라, 높이가 바뀌면 사용자가 놓아 둔
            자리의 뜻이 조용히 달라진다. 그래서 되돌릴 수 없는 그 변화는 손으로만 일어난다.
            바뀔 값을 `title` 에 적어 두므로 누르기 전에 결과를 알 수 있다. */}
        <button
          type="button"
          onClick={() => {
            if (fittedCanvas === null) return;
            onConfigChange({ canvas: fittedCanvas });
          }}
          disabled={fittedCanvas === null}
          aria-label={t('dashboard.canvas.elements.panelSizeFitAria')}
          // 바뀔 값은 로케일 문장 **밖에서** 붙인다. 자리표시자로 끼워 넣으면 번역이
          // 그 표시자를 잃는 날 수치가 조용히 사라지고, 사용자는 무엇이 바뀌는지 모른 채
          // 되돌릴 수 없는 단추를 누르게 된다.
          title={
            fittedCanvas === null
              ? t('dashboard.canvas.elements.panelSizeFitNone')
              : `${t('dashboard.canvas.elements.panelSizeFitTitle')}: ` +
                `${cfg.canvas.width}×${cfg.canvas.height} → ` +
                `${fittedCanvas.width}×${fittedCanvas.height}`
          }
          data-testid="canvas-panel-size-fit"
          className={cn(
            'shrink-0 rounded border border-(--color-border-default) px-2 py-1 text-xs',
            'text-(--color-text-secondary) hover:bg-(--color-bg-hover)',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          {t('dashboard.canvas.elements.panelSizeFit')}
        </button>
      </div>

      {/* 패널 축 — 배경색과 기본 트윈. 요소가 덮어쓸 수 있는 값들이다. */}
      <div className="flex flex-wrap items-center gap-2">
        <span className={GROUP_LABEL_CLASS}>{t('dashboard.canvas.elements.panelBackground')}</span>
        <ColorSwatchButton
          color={cfg.background}
          onChange={(c) => onConfigChange({ background: c })}
          ariaLabel={t('dashboard.canvas.elements.panelBackgroundAria')}
          testId="canvas-panel-background"
        />

        <span className={GROUP_LABEL_CLASS}>{t('dashboard.canvas.elements.panelTween')}</span>
        {/* 이 칸이 무엇을 하는지는 겉모습만으로 읽히지 않는다 — 값이 바뀌는 **순간에만**
            드러나는 효과이기 때문이다. 그래서 무엇이 옮겨 가는지와 0 의 뜻을 적되,
            줄로 깔지 않고 제목 뒤 `?` 뒤에 넣는다(설정 화면 전체의 규칙). */}
        <FieldHelp
          text={t('dashboard.canvas.elements.panelTweenHint')}
          testId="canvas-panel-tween-hint"
        />
        <input
          type="number"
          step="any"
          min={0}
          value={cfg.tween?.duration_ms ?? ''}
          onChange={(e) =>
            onConfigChange({
              tween: tweenWithDuration(cfg.tween, nonNegative(parseOptionalNumber(e.target.value))),
            })
          }
          placeholder={t('dashboard.canvas.elements.tweenDurationPlaceholder')}
          aria-label={t('dashboard.canvas.elements.panelTweenDurationAria')}
          data-testid="canvas-panel-tween-duration"
          className={cn(INPUT_CLASS, 'w-16 text-center tabular-nums')}
        />
        <select
          value={cfg.tween?.easing ?? DEFAULT_TWEEN_EASING}
          onChange={(e) =>
            onConfigChange({ tween: tweenWithEasing(cfg.tween, e.target.value as TweenEasing) })
          }
          aria-label={t('dashboard.canvas.elements.panelTweenEasingAria')}
          data-testid="canvas-panel-tween-easing"
          className={cn(INPUT_CLASS, 'shrink-0')}
        >
          {TWEEN_EASINGS.map((ea) => (
            <option key={ea} value={ea}>
              {t(EASING_LABEL_KEY[ea])}
            </option>
          ))}
        </select>
      </div>

      {/* 001 의 z-order 는 배열 순서 하나뿐이다 — 그 사실을 화면에 적는다.
          이 한 줄만 인라인으로 남는다: 목록 전체에 대한 말이라 뒤에 `?` 를 달 제목이
          없고, 제목 없는 물음표는 무엇을 묻는지 알 수 없는 단추가 된다. */}
      <p className={HINT_CLASS}>
        {t('dashboard.canvas.elements.orderHint')}
      </p>

      {elements.length === 0 ? (
        <p className={HINT_CLASS} data-testid="canvas-element-empty">
          {t('dashboard.canvas.elements.empty')}
        </p>
      ) : (
        <div className="space-y-2">
          {elements.map((el, idx) => {
            const bindingOptions = bindingOptionsFor(seriesOptions, el.binding?.series);
            const unbound = el.binding === undefined;
            const open = isExpanded(el.id);
            // 다중 선택에서는 자동 펼침이 없으므로 **표시만** 남는다(AC-06).
            const picked = selection.has(el.id);
            // 판정은 **파서 옆의 한 함수**를 부른다(`isNumericElement`) — 부재 = 숫자라는
            // 규칙이 이 화면에도 한 벌 더 적히면, 둘 중 하나가 참 판정으로 적히는 순간
            // 기존 config 의 표기가 편집기에서만 뒤집혀 보인다.
            const numeric = isNumericElement(el);
            // 좌표가 0..1 이라는 말은 종류마다 갈라 그리는 세 갈래에서 **같은 하나**를
            // 쓴다. 갈래마다 새로 적으면 같은 testId 가 세 벌 생기고, 그중 어느 것이
            // 화면에 섰는지는 종류를 봐야만 알 수 있다.
            const coordHelp = (
              <FieldHelp
                text={t('dashboard.canvas.elements.coordHint')}
                testId={`canvas-element-coord-help-${idx}`}
              />
            );

            return (
              <div
                key={el.id}
                ref={(node) => {
                  // 캔버스가 고른 행을 시야로 끌어올 때 쓴다. 언마운트된 행을 붙들고 있으면
                  // 지워진 요소가 지도에 남으므로 정리한다.
                  if (node === null) rowRefs.current.delete(el.id);
                  else rowRefs.current.set(el.id, node);
                }}
                data-testid={`canvas-element-${idx}`}
                data-element-id={el.id}
                data-selected={picked ? 'true' : undefined}
                className={cn(
                  'space-y-1.5 rounded-md border p-1.5',
                  picked ? 'border-blue-500' : 'border-(--color-border-default)',
                )}
              >
                {/* 1행: 순번 · 요소 요약(펼침 토글) · 순서 이동 · 삭제.
                    **종류는 이 줄에 없다** — 종류는 요소를 훑거나 순서를 바꾸는 일이
                    아니라 그 요소가 무엇으로 그려지는지를 정하는 저술이고, 그래서 아래
                    도형 묶음의 첫 칸에 선다. 머리줄에 남는 것은 펼치지 않고도 되어야 하는
                    일뿐이다. */}
                <div className="flex w-full items-center gap-1.5">
                  <span
                    className={ORDER_BADGE_CLASS}
                    data-testid={`canvas-element-order-${idx}`}
                  >
                    {idx + 1}
                  </span>

                  {/* 접기 토글 겸 **고르기**. 1행(순번 · 요약 · 순서 · 삭제)은 접혀도
                      남는다 — 훑기·순서 조작·삭제는 펼치지 않고도 되어야 하는 일이다.
                      펼치는 방향에서만 고른다(위 `toggleExpanded` 주석). */}
                  <button
                    type="button"
                    onClick={() => toggleExpanded(el.id)}
                    aria-expanded={open}
                    aria-label={withIndex(t('dashboard.canvas.elements.detailsAria'), idx)}
                    data-testid={`canvas-element-toggle-${idx}`}
                    className="flex min-w-0 flex-1 items-center gap-1 rounded text-left text-xs text-(--color-text-muted) hover:text-(--color-text-secondary)"
                  >
                    {open ? (
                      <ChevronDown className="h-3 w-3 shrink-0" />
                    ) : (
                      <ChevronRight className="h-3 w-3 shrink-0" />
                    )}
                    <span className="truncate">{summaryOf(el, t)}</span>
                  </button>

                  <button
                    type="button"
                    onClick={() => moveAt(idx, -1)}
                    disabled={idx === 0}
                    className={ICON_BUTTON_CLASS}
                    aria-label={withIndex(t('dashboard.canvas.elements.moveUpAria'), idx)}
                    data-testid={`canvas-element-move-up-${idx}`}
                  >
                    <ChevronUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => moveAt(idx, 1)}
                    disabled={idx === elements.length - 1}
                    className={ICON_BUTTON_CLASS}
                    aria-label={withIndex(t('dashboard.canvas.elements.moveDownAria'), idx)}
                    data-testid={`canvas-element-move-down-${idx}`}
                  >
                    <ChevronDown className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => removeAt(idx)}
                    className="shrink-0 text-(--color-text-muted) hover:text-red-500"
                    aria-label={withIndex(t('dashboard.canvas.elements.deleteAria'), idx)}
                    data-testid={`canvas-element-delete-${idx}`}
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>

                {/* 펼친 몸통 — **탭 넷**(스타일 · 텍스트 · 배치 · 데이터)이 머리줄 아래에
                    서고, 열린 탭의 묶음만 그 아래 판에 쌓인다. 묶음 자체의 규율은 그대로다:
                    **제목이 제 줄에 서고 칸이 그 아래 줄에 선다.** 바뀐 것은 일곱 묶음이 한
                    두루마리로 이어져 있던 것을 넷으로 **가른** 것뿐이며, 새로 생긴 축도 새로
                    저장되는 값도 없다.

                    머리줄(순번 · 요약 · 순서 이동 · 삭제)만 접혀도 남는다. **종류는 몸통 안
                    (스타일 탭의 첫 칸)에 있으므로 접으면 함께 접힌다.** */}
                {open && (
                  <div className="space-y-2">
                    <ElementTabs
                      idx={idx}
                      active={activeTab}
                      onSelect={setActiveTab}
                      label={withIndex(t('dashboard.canvas.elements.tabsAria'), idx)}
                      labelOf={(tab) => t(TAB_LABEL_KEY[tab])}
                    />

                    {/* 열린 탭 하나만 그린다 — 넷을 모두 그려 두고 감추면 접힌 카드가
                        지고 있던 약속("보이지 않는 칸은 DOM 에도 없다")이 탭에서만 깨진다. */}
                    <div
                      role="tabpanel"
                      id={`canvas-element-tabpanel-${idx}`}
                      aria-labelledby={`canvas-element-tab-${idx}-${activeTab}`}
                      data-testid={`canvas-element-tabpanel-${idx}`}
                      className="space-y-2"
                    >
                      {/* ==== 배치 탭 — 순서 · 크기 · 위치 ==== */}
                      {activeTab === 'arrange' && (
                        <>
                          {/* --- 순서(z-order) ---
                              001 이래로 z-order 수단은 배열 순서 하나뿐이고, 그것을 움직이는
                              규칙도 `canvasEditArrange.moveElementTo` 하나뿐이다. 네 단추는 그
                              하나에 주는 **목표 자리**만 다르다: 한 칸 앞/뒤는 `idx ± 1`,
                              맨 앞/맨 뒤는 그 함수로 지어진 `bringToFront`/`sendToBack` 이다.
                              그래서 여기 있는 것은 새 기능이 아니라 **이미 있던 동작의 노출**이다.

                              머리줄의 위/아래 단추는 그대로 남는다 — 그 둘은 접힌 채로 목록을
                              훑으며 순서를 고치기 위한 것이고(펼치지 않고도 되어야 하는 일),
                              이 넷은 카드를 펼쳐 한 요소를 다루는 동안의 온전한 벌이다. 두
                              입구가 **같은 함수**를 지나므로 결과가 갈릴 수 없다. */}
                          <FieldGroup
                            label={t('dashboard.canvas.elements.zorderLabel')}
                            testId={`canvas-element-zorder-${idx}`}
                          >
                            <button
                              type="button"
                              onClick={() => sendAt(idx, true)}
                              disabled={idx === elements.length - 1}
                              aria-label={withIndex(t('dashboard.canvas.elements.zorderFrontAria'), idx)}
                              data-testid={`canvas-element-zorder-front-${idx}`}
                              className={ORDER_BUTTON_CLASS}
                            >
                              <ChevronsDown className="h-3 w-3" aria-hidden="true" />
                              <span>{t('dashboard.canvas.elements.zorderFront')}</span>
                            </button>
                            <button
                              type="button"
                              onClick={() => sendAt(idx, false)}
                              disabled={idx === 0}
                              aria-label={withIndex(t('dashboard.canvas.elements.zorderBackAria'), idx)}
                              data-testid={`canvas-element-zorder-back-${idx}`}
                              className={ORDER_BUTTON_CLASS}
                            >
                              <ChevronsUp className="h-3 w-3" aria-hidden="true" />
                              <span>{t('dashboard.canvas.elements.zorderBack')}</span>
                            </button>
                            <button
                              type="button"
                              onClick={() => moveAt(idx, 1)}
                              disabled={idx === elements.length - 1}
                              aria-label={withIndex(t('dashboard.canvas.elements.zorderForwardAria'), idx)}
                              data-testid={`canvas-element-zorder-forward-${idx}`}
                              className={ORDER_BUTTON_CLASS}
                            >
                              <ChevronDown className="h-3 w-3" aria-hidden="true" />
                              <span>{t('dashboard.canvas.elements.zorderForward')}</span>
                            </button>
                            <button
                              type="button"
                              onClick={() => moveAt(idx, -1)}
                              disabled={idx === 0}
                              aria-label={withIndex(t('dashboard.canvas.elements.zorderBackwardAria'), idx)}
                              data-testid={`canvas-element-zorder-backward-${idx}`}
                              className={ORDER_BUTTON_CLASS}
                            >
                              <ChevronUp className="h-3 w-3" aria-hidden="true" />
                              <span>{t('dashboard.canvas.elements.zorderBackward')}</span>
                            </button>
                          </FieldGroup>

                          {/* --- 크기 · 위치 ---
                              종류마다 요구하는 축이 다르므로 갈라 그린다(합집합을 하나의 map 으로
                              접으면 `kind` 와 `geometry` 의 짝을 컴파일러가 확인하지 못한다).
                              좌표가 캔버스 단위의 정수라는 사실은 **위치 묶음 제목 뒤 `?`** 만
                              말한다 — 묶음마다 되풀이하면 같은 말이 한 화면에 두 번 선다.

                              크기가 위치보다 먼저 오는 것은 참고 화면의 차례다: 얼마만큼인지를
                              정한 다음 어디인지를 정한다. */}
                          {(el.kind === 'rect' || el.kind === 'ellipse') && (
                            <>
                              <FieldGroup
                                label={t('dashboard.canvas.elements.sizeLabel')}
                                testId={`canvas-element-size-${idx}`}
                              >
                                {BOX_SIZE_AXES.map((axis) => (
                                  <GeometryInput
                                    key={axis}
                                    axis={axis}
                                    extent
                                    value={el.geometry[axis]}
                                    onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                                    ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                                    testId={`canvas-element-geo-${axis}-${idx}`}
                                  />
                                ))}
                              </FieldGroup>
                              <FieldGroup
                                label={t('dashboard.canvas.elements.positionLabel')}
                                testId={`canvas-element-position-${idx}`}
                                help={coordHelp}
                              >
                                {BOX_POSITION_AXES.map((axis) => (
                                  <GeometryInput
                                    key={axis}
                                    axis={axis}
                                    value={el.geometry[axis]}
                                    onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                                    ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                                    testId={`canvas-element-geo-${axis}-${idx}`}
                                  />
                                ))}
                              </FieldGroup>
                            </>
                          )}
                          {/* 선은 두 끝점이 전부다 — 크기 묶음을 만들지 않는다(위 축 상수 주석). */}
                          {el.kind === 'line' && (
                            <FieldGroup
                              label={t('dashboard.canvas.elements.endpointsLabel')}
                              testId={`canvas-element-position-${idx}`}
                              help={coordHelp}
                            >
                              {LINE_AXES.map((axis) => (
                                <GeometryInput
                                  key={axis}
                                  axis={axis}
                                  value={el.geometry[axis]}
                                  onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                                  ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                                  testId={`canvas-element-geo-${axis}-${idx}`}
                                />
                              ))}
                            </FieldGroup>
                          )}
                          {/* 텍스트의 크기는 글자 크기이며 **텍스트 탭에 한 자리에만** 있다. */}
                          {el.kind === 'text' && (
                            <FieldGroup
                              label={t('dashboard.canvas.elements.positionLabel')}
                              testId={`canvas-element-position-${idx}`}
                              help={coordHelp}
                            >
                              {POINT_AXES.map((axis) => (
                                <GeometryInput
                                  key={axis}
                                  axis={axis}
                                  value={el.geometry[axis]}
                                  onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                                  ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                                  testId={`canvas-element-geo-${axis}-${idx}`}
                                />
                              ))}
                            </FieldGroup>
                          )}
                        </>
                      )}

                      {/* ==== 스타일 탭 — 무엇으로, 어떻게 그리는가 ==== */}
                      {activeTab === 'style' && (
                        <FieldGroup
                          label={t('dashboard.canvas.elements.shapeStyleLabel')}
                          testId={`canvas-element-shape-style-${idx}`}
                        >
                          {/* **첫 칸이 종류다.** 이 묶음이 "무엇을 어떻게 그리는가" 를 다루는
                              자리이고, 그중 가장 먼저 정해지는 것이 무엇으로 그릴지이기 때문이다
                              (종류가 바뀌면 아래 두께·색이 걸릴 도형 자체가 바뀐다). 종류를
                              바꾸면 기하가 새 형상으로 옮겨 실린다 — `withKind` 한 곳이 그 일을
                              하며, 탭으로 갈라도 그 경로는 그대로다. 머리줄이 아닌 것도 그대로다. */}
                          <select
                            value={el.kind}
                            onChange={(e) => replaceAt(idx, withKind(el, e.target.value as CanvasElementKind))}
                            aria-label={withIndex(t('dashboard.canvas.elements.kindAria'), idx)}
                            data-testid={`canvas-element-kind-${idx}`}
                            className={cn(INPUT_CLASS, 'shrink-0')}
                          >
                            {ELEMENT_KINDS.map((k) => (
                              <option key={k} value={k}>
                                {t(KIND_LABEL_KEY[k])}
                              </option>
                            ))}
                          </select>

                          {/* 차례는 참고 화면을 따른다 — **채우기 → 테두리(색과 두께가 붙어
                              선다) → 불투명도**. 한때 테두리를 먼저 두고 "바깥에서 안으로"
                              라 적었지만, 그 차례는 두께를 제 색에서 떼어 놓아 한 가지
                              (테두리)를 두 자리에서 만지게 했다.
                              비운 칸은 미지정이며 렌더측 기본을 따른다.
                              `fill` 의 자리는 **종류를 따른다.** 도형에서는 도형의 색이고,
                              `kind:'text'` 에서는 칠할 도형이 없어 렌더 층이 이 값을 글자색의
                              폴백으로 읽는다(`drawElement`: `textColor ?? fill`). 그래서 텍스트
                              요소에서는 이 칸이 텍스트 탭에 선다 — 없는 도형의 색을 "도형" 이라
                              부르면 화면이 거짓말을 한다. */}
                          {el.kind !== 'text' && (
                            <ColorSwatchButton
                              color={el.style.fill}
                              onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'fill', c) })}
                              ariaLabel={withIndex(t('dashboard.canvas.elements.fillAria'), idx)}
                              testId={`canvas-element-fill-${idx}`}
                            />
                          )}

                          <ColorSwatchButton
                            color={el.style.stroke}
                            onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'stroke', c) })}
                            ariaLabel={withIndex(t('dashboard.canvas.elements.strokeAria'), idx)}
                            testId={`canvas-element-stroke-${idx}`}
                          />
                          <input
                            type="number"
                            step="any"
                            min={0}
                            value={el.style.strokeWidth ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(
                                  el.style,
                                  'strokeWidth',
                                  nonNegative(parseOptionalNumber(e.target.value)),
                                ),
                              })
                            }
                            placeholder={t('dashboard.canvas.elements.strokeWidthPlaceholder')}
                            aria-label={withIndex(t('dashboard.canvas.elements.strokeWidthAria'), idx)}
                            data-testid={`canvas-element-stroke-width-${idx}`}
                            className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                          />

                          <input
                            type="number"
                            step="any"
                            min={0}
                            max={1}
                            value={el.style.opacity ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(el.style, 'opacity', clamp01(parseOptionalNumber(e.target.value))),
                              })
                            }
                            placeholder={t('dashboard.canvas.elements.opacityPlaceholder')}
                            aria-label={withIndex(t('dashboard.canvas.elements.opacityAria'), idx)}
                            data-testid={`canvas-element-opacity-${idx}`}
                            className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                          />

                          {/* `visible` 은 도형만이 아니라 요소 전체를 끄는 스위치지만, 이 탭이
                              요소를 그리는 방식을 다루는 자리이므로 여기 둔다(텍스트만 숨기는
                              축은 없다). 다만 그리는 방식이 아니라 그릴지 말지를 정하는 축이라
                              **줄 끝**에 선다 — 앞자리는 그리는 방식을 정하는 칸들의 것이다.
                              3지 선택인 것은 체크박스로 "미지정"과 "숨김"을 구분할 수 없고,
                              그 둘은 뜻이 다르기 때문이다(규칙 표의 같은 컨트롤과 같은 판단). */}
                          <select
                            value={el.style.visible === undefined ? '' : el.style.visible ? 'show' : 'hide'}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(
                                  el.style,
                                  'visible',
                                  e.target.value === '' ? undefined : e.target.value === 'show',
                                ),
                              })
                            }
                            aria-label={withIndex(t('dashboard.canvas.elements.visibleAria'), idx)}
                            data-testid={`canvas-element-visible-${idx}`}
                            className={cn(INPUT_CLASS, 'shrink-0')}
                          >
                            <option value="">{t('dashboard.canvas.elements.unset')}</option>
                            <option value="show">{t('dashboard.canvas.elements.visibleShow')}</option>
                            <option value="hide">{t('dashboard.canvas.elements.visibleHide')}</option>
                          </select>
                        </FieldGroup>
                      )}

                      {/* ==== 텍스트 탭 — 무엇을 적고, 그 글자를 어떻게 칠하는가 ====
                          내용(템플릿)과 글자에 걸리는 스타일이 **한 묶음**이다. 둘을 갈라
                          두었던 동안에는 값 템플릿을 적는 칸과 그 글자의 크기·색이 서로 다른
                          줄에 있어, 한 가지 일을 하려고 두 자리를 오가야 했다.
                          `?` 둘이 서로 다른 말을 한다: 토큰 3종의 사용법과, 색이 어떻게
                          칠해지는지(종류마다 규칙이 다르다).
                          **숫자 스위치는 여기 없다** — 값을 어떻게 읽을지는 무엇을 읽는지와
                          한 결정이므로 데이터 탭의 바인딩 옆에 선다. */}
                      {activeTab === 'text' && (
                        <FieldGroup
                          label={t('dashboard.canvas.elements.textStyleLabel')}
                          testId={`canvas-element-text-style-${idx}`}
                          help={
                            <>
                              <FieldHelp
                                text={t('dashboard.canvas.elements.tokenHelp')}
                                testId={`canvas-element-token-help-${idx}`}
                              />
                              {/* 두 갈래를 **화면에 적는다.** 렌더 층의 규칙이 종류마다 다르고
                                  (`drawElement`: 텍스트는 `textColor ?? fill`, 도형 라벨은
                                  `textColor` 만), 게다가 도형에 문구를 처음 적는 순간
                                  `canvasElementFactory` 가 글자색을 대신 심는다. 적어 두지 않으면
                                  사용자는 "고르지도 않은 색이 들어와 있다" 를 결함으로 읽는다. */}
                              <FieldHelp
                                text={
                                  el.kind === 'text'
                                    ? t('dashboard.canvas.elements.textStyleHintText')
                                    : t('dashboard.canvas.elements.textStyleHintShape')
                                }
                                testId={`canvas-element-text-style-help-${idx}`}
                              />
                            </>
                          }
                        >
                          {/* 문구만은 `setElementField` 가 아니라 팩토리를 지난다 — 도형에 라벨이
                              생기는 순간 글자색이 함께 심겨야 그 라벨이 실제로 칠해진다. */}
                          <input
                            type="text"
                            value={el.text ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, withElementText(el, optionalText(e.target.value)))
                            }
                            placeholder={t('dashboard.canvas.elements.textPlaceholder')}
                            aria-label={withIndex(t('dashboard.canvas.elements.textAria'), idx)}
                            data-testid={`canvas-element-text-${idx}`}
                            className={cn(INPUT_CLASS, 'min-w-16 flex-1')}
                          />

                          <input
                            type="number"
                            step="any"
                            min={0}
                            value={el.style.fontSize ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(
                                  el.style,
                                  'fontSize',
                                  nonNegative(parseOptionalNumber(e.target.value)),
                                ),
                              })
                            }
                            placeholder={t('dashboard.canvas.elements.fontSizePlaceholder')}
                            aria-label={withIndex(t('dashboard.canvas.elements.fontSizeAria'), idx)}
                            data-testid={`canvas-element-font-size-${idx}`}
                            className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                          />

                          <select
                            value={el.style.fontWeight ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(
                                  el.style,
                                  'fontWeight',
                                  e.target.value === '' ? undefined : (e.target.value as ElementFontWeight),
                                ),
                              })
                            }
                            aria-label={withIndex(t('dashboard.canvas.elements.fontWeightAria'), idx)}
                            data-testid={`canvas-element-font-weight-${idx}`}
                            className={cn(INPUT_CLASS, 'shrink-0')}
                          >
                            <option value="">{t('dashboard.canvas.elements.unset')}</option>
                            <option value="normal">{t('dashboard.canvas.elements.fontWeightNormal')}</option>
                            <option value="bold">{t('dashboard.canvas.elements.fontWeightBold')}</option>
                          </select>

                          <select
                            value={el.style.align ?? ''}
                            onChange={(e) =>
                              replaceAt(idx, {
                                ...el,
                                style: setStyleField(
                                  el.style,
                                  'align',
                                  e.target.value === '' ? undefined : (e.target.value as ElementAlign),
                                ),
                              })
                            }
                            aria-label={withIndex(t('dashboard.canvas.elements.alignAria'), idx)}
                            data-testid={`canvas-element-align-${idx}`}
                            className={cn(INPUT_CLASS, 'shrink-0')}
                          >
                            <option value="">{t('dashboard.canvas.elements.unset')}</option>
                            <option value="left">{t('dashboard.canvas.elements.alignLeft')}</option>
                            <option value="center">{t('dashboard.canvas.elements.alignCenter')}</option>
                            <option value="right">{t('dashboard.canvas.elements.alignRight')}</option>
                          </select>

                          <ColorSwatchButton
                            color={el.style.textColor}
                            onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'textColor', c) })}
                            ariaLabel={withIndex(t('dashboard.canvas.elements.textColorAria'), idx)}
                            testId={`canvas-element-text-color-${idx}`}
                          />
                          {/* 텍스트 요소의 `fill` 은 글자색 폴백이다 — 그래서 글자색 **바로 옆**에
                              선다(위 도형 묶음의 주석 참조). */}
                          {el.kind === 'text' && (
                            <ColorSwatchButton
                              color={el.style.fill}
                              onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'fill', c) })}
                              ariaLabel={withIndex(t('dashboard.canvas.elements.fillAria'), idx)}
                              testId={`canvas-element-fill-${idx}`}
                            />
                          )}
                        </FieldGroup>
                      )}

                      {/* ==== 데이터 탭 — 무엇을 읽고, 어떻게 반응하는가 ==== */}
                      {activeTab === 'data' && (
                        <>
                          {/* --- 데이터 ---
                              바인딩이 없으면 정적 도형이며, 규칙은 평가되지 않는다(그래서 아래
                              표가 읽기 전용으로 잠긴다). */}
                          <FieldGroup
                            label={t('dashboard.canvas.elements.bindingLabel')}
                            testId={`canvas-element-data-${idx}`}
                          >
                            <select
                              value={el.binding?.series ?? ''}
                              onChange={(e) =>
                                replaceAt(
                                  idx,
                                  setElementField(
                                    el,
                                    'binding',
                                    e.target.value === '' ? undefined : { series: e.target.value, agg: 'last' },
                                  ),
                                )
                              }
                              aria-label={withIndex(t('dashboard.canvas.elements.bindingAria'), idx)}
                              data-testid={`canvas-element-binding-${idx}`}
                              className={cn(INPUT_CLASS, 'min-w-24 flex-1')}
                            >
                              <option value="">{t('dashboard.canvas.elements.bindingNone')}</option>
                              {bindingOptions.map((o) => (
                                <option key={o.id} value={o.id}>
                                  {o.label}
                                </option>
                              ))}
                            </select>
                            {/* 고를 것이 하나도 없으면 **왜 없는지**를 말한다. 목록이 비는 정상적인
                                이유는 하나뿐이다 — 데이터 소스에 시리즈가 아직 없다. 그 말을 하지
                                않으면 사용자는 "바인딩 없음" 만 있는 상자를 보고 고장으로 읽는다.
                                히트맵의 `heatmapNoSensors` 가 같은 자리에서 같은 일을 하며, 가리키는
                                곳도 화면이 그 절을 부르는 이름(`dashboard.settings.dataSource`)
                                그대로다. */}
                            {bindingOptions.length === 0 && (
                              <p
                                className={cn(HINT_CLASS, 'w-full')}
                                data-testid={`canvas-element-binding-hint-${idx}`}
                              >
                                {t('dashboard.canvas.elements.bindingNoSeries')}
                              </p>
                            )}
                              {/* --- 숫자 스위치 ---
                                  **보이기/감추기 스위치가 아니다.** 판독값을 어떻게 **읽을지**를
                                  정한다: 켜면 숫자로 읽어 소수 자리로 반올림하고 단위를 붙이며(001
                                  이래의 동작), 끄면 받은 값을 **문자열 그대로** 내보낸다. 그래서 끈
                                  상태에서 소수 자리·단위는 걸릴 곳이 없고, 걸리지 않는 칸을 남겨 두면
                                  사용자는 고쳐도 아무 일도 일어나지 않는 칸을 만진다 — 그 둘을 함께
                                  감추는 이유다.

                                  `w-full` 로 제 줄을 차지한다. 위 칸들과 한 줄에 섞이면 "값을 어떻게
                                  읽는가" 라는 다른 층위의 결정이 글꼴 설정처럼 보인다. */}
                              <div className="flex w-full flex-wrap items-center gap-1.5">
                                <label className="flex items-center gap-1 text-xs text-(--color-text-primary)">
                                  {/* 체크는 **키를 지운다**(부재 = 숫자). 켠 상태를 `numeric: true` 로
                                      적어 두면 종전 config 와 형상이 갈라지고, 갈라진 만큼 왕복 시험이
                                      지켜야 할 것이 늘어난다(규율 1 — 기본값은 부재로 적는다). */}
                                  <input
                                    type="checkbox"
                                    checked={numeric}
                                    onChange={(e) =>
                                      replaceAt(
                                        idx,
                                        setElementField(el, 'numeric', e.target.checked ? undefined : false),
                                      )
                                    }
                                    aria-label={withIndex(t('dashboard.canvas.elements.numericAria'), idx)}
                                    data-testid={`canvas-element-numeric-${idx}`}
                                    className="h-3.5 w-3.5 shrink-0 accent-blue-500"
                                  />
                                  {t('dashboard.canvas.elements.numericLabel')}
                                </label>
                                <FieldHelp
                                  text={t('dashboard.canvas.elements.numericHint')}
                                  testId={`canvas-element-numeric-help-${idx}`}
                                />
                              </div>

                              {numeric && (
                                <div
                                  className="flex w-full flex-wrap items-center gap-1.5 rounded border border-(--color-border-default) px-1.5 py-1"
                                  data-testid={`canvas-element-numeric-fields-${idx}`}
                                >
                                  <input
                                    type="number"
                                    step={1}
                                    min={0}
                                    value={el.decimals ?? ''}
                                    onChange={(e) =>
                                      replaceAt(
                                        idx,
                                        setElementField(el, 'decimals', decimalsOf(parseOptionalNumber(e.target.value))),
                                      )
                                    }
                                    placeholder={t('dashboard.canvas.elements.decimalsPlaceholder')}
                                    aria-label={withIndex(t('dashboard.canvas.elements.decimalsAria'), idx)}
                                    data-testid={`canvas-element-decimals-${idx}`}
                                    className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                                  />
                                  <input
                                    type="text"
                                    value={el.unit ?? ''}
                                    onChange={(e) =>
                                      replaceAt(idx, setElementField(el, 'unit', optionalText(e.target.value)))
                                    }
                                    placeholder={t('dashboard.canvas.elements.unitPlaceholder')}
                                    aria-label={withIndex(t('dashboard.canvas.elements.unitAria'), idx)}
                                    data-testid={`canvas-element-unit-${idx}`}
                                    className={cn(INPUT_CLASS, 'w-16')}
                                  />
                                </div>
                              )}
                          </FieldGroup>

                          {/* --- 전환 효과 ---
                              패널 쪽과 같은 이유로 `?` 를 단다 — 다만 **비웠을 때의 뜻이 다르다**:
                              여기서 비우면 꺼지는 것이 아니라 패널 기본값을 따른다. */}
                          <FieldGroup
                            label={t('dashboard.canvas.elements.tweenLabel')}
                            testId={`canvas-element-tween-${idx}`}
                            help={
                              <FieldHelp
                                text={t('dashboard.canvas.elements.tweenHint')}
                                testId={`canvas-element-tween-hint-${idx}`}
                              />
                            }
                          >
                            <input
                              type="number"
                              step="any"
                              min={0}
                              value={el.tween?.duration_ms ?? ''}
                              onChange={(e) =>
                                replaceAt(
                                  idx,
                                  setElementField(
                                    el,
                                    'tween',
                                    tweenWithDuration(el.tween, nonNegative(parseOptionalNumber(e.target.value))),
                                  ),
                                )
                              }
                              placeholder={t('dashboard.canvas.elements.tweenDurationPlaceholder')}
                              aria-label={withIndex(t('dashboard.canvas.elements.tweenDurationAria'), idx)}
                              data-testid={`canvas-element-tween-duration-${idx}`}
                              className={cn(INPUT_CLASS, 'w-16 text-center tabular-nums')}
                            />
                            <select
                              value={el.tween?.easing ?? DEFAULT_TWEEN_EASING}
                              onChange={(e) =>
                                replaceAt(
                                  idx,
                                  setElementField(
                                    el,
                                    'tween',
                                    tweenWithEasing(el.tween, e.target.value as TweenEasing),
                                  ),
                                )
                              }
                              aria-label={withIndex(t('dashboard.canvas.elements.tweenEasingAria'), idx)}
                              data-testid={`canvas-element-tween-easing-${idx}`}
                              className={cn(INPUT_CLASS, 'shrink-0')}
                            >
                              {TWEEN_EASINGS.map((ea) => (
                                <option key={ea} value={ea}>
                                  {t(EASING_LABEL_KEY[ea])}
                                </option>
                              ))}
                            </select>
                          </FieldGroup>

                          {/* --- 규칙 ---
                              표 본문은 별도 컴포넌트다(§위험 R4) — 여기서는 붙이기만 한다.

                              숫자로 읽지 않는 요소에서는 **비교할 값 자체가 없다**. 패널이 규칙
                              평가에 `null` 을 넘기므로 스칼라 연산자 행은 하나도 일치하지 않고
                              `값 없음` 행만 남는다(`canvasRules.matchesRule`). 표가 조용히 죽으면
                              사용자는 그것을 고장으로 읽으므로, 그때만 제목 뒤에 `?` 하나가 더 붙어
                              이유를 말한다 — 줄로 깔지 않는 것은 이 화면 전체의 규칙이다. */}
                          <FieldGroup
                            label={t('dashboard.canvas.elements.rulesLabel')}
                            testId={`canvas-element-rules-${idx}`}
                            help={
                              numeric ? undefined : (
                                <FieldHelp
                                  text={t('dashboard.canvas.elements.rulesNonNumericHint')}
                                  testId={`canvas-element-rules-nonnumeric-help-${idx}`}
                                />
                              )
                            }
                          >
                            <div className="w-full rounded border border-(--color-border-default) p-1.5">
                              <CanvasRuleTableEditor
                                rules={el.rules}
                                onChange={(rules: RuleRow[] | undefined) =>
                                  replaceAt(idx, setElementField(el, 'rules', rules))
                                }
                                disabled={unbound}
                              />
                            </div>
                          </FieldGroup>
                        </>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* 여기에 종류별 추가 버튼 한 줄이 있었다. 도형 팔레트가 미리보기 왼쪽 도크로
          옮겨 오면서(`CanvasEditDock`) 같은 화면에 **같은 일을 하는 입구가 둘**이 되었고,
          둘 중 하나는 이름도 아이콘도 더 큰 자리를 가진 쪽이다. 두 입구가 정확히 같은
          함수(`appendElement`)를 부르는 한 남은 것은 중복뿐이라 걷어낸다.

          생성 경로 자체는 그대로다 — 도크가 그 함수를 부른다. */}
    </div>
  );
}
