// 캔버스 요소 목록 편집기 (SPEC-CANVAS-001 T8 · REQ-01(6) · REQ-02 · REQ-03 · REQ-04).
//
// 001 에서 사용자는 요소의 기하를 **수치로** 저술한다(가정 A4). 캔버스 위 드래그 배치·
// 크기 조절·스냅·z-order 조작은 002 의 몫이므로 여기에는 없다. 그래서 이 화면의 어휘는
// "숫자 칸과 선택 상자" 이며, 배열 순서가 곧 그리기 순서(뒤가 위)라는 사실을 **화면에
// 적어** 위/아래 이동 버튼이 왜 z-order 인지 사용자가 알 수 있게 한다 — 001 의 z-order
// 수단은 배열 순서 하나뿐이다.
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

import { useMemo } from 'react';
import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
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
import { toStoreShapedConfig } from '../charts/useSysMetricsChartData';
import {
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  DEFAULT_TWEEN_EASING,
  parseCanvasConfig,
  type BoxGeometry,
  type CanvasElement,
  type CanvasElementKind,
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

/** 이징 라벨 키. */
const EASING_LABEL_KEY: Record<TweenEasing, string> = {
  'linear': 'dashboard.canvas.elements.easingLinear',
  'ease-in': 'dashboard.canvas.elements.easingIn',
  'ease-out': 'dashboard.canvas.elements.easingOut',
  'ease-in-out': 'dashboard.canvas.elements.easingInOut',
};

/** rect · ellipse 의 기하 축. */
const BOX_AXES = ['x', 'y', 'w', 'h'] as const;
/** line 의 기하 축. */
const LINE_AXES = ['x1', 'y1', 'x2', 'y2'] as const;
/** text 의 기하 축. */
const POINT_AXES = ['x', 'y'] as const;

/** 입력 공통 클래스(규칙 표 행과 같은 치수). */
const INPUT_CLASS =
  'min-w-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 ' +
  'text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500';

/** 순서 이동·삭제 아이콘 버튼 공통 클래스. */
const ICON_BUTTON_CLASS =
  'shrink-0 text-(--color-text-muted) hover:text-(--color-text-secondary) disabled:opacity-30';

/** 소구획 제목 클래스. */
const GROUP_LABEL_CLASS = 'shrink-0 text-[10px] font-medium text-(--color-text-muted)';

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

/** 바인딩 선택지 한 줄 — 값은 동일성 키, 표시는 사람이 읽는 이름이다. */
interface SeriesOption {
  id: string;
  label: string;
}

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 데이터 소스 config 에서 바인딩 선택지를 만든다.
 *
 * 값(=`binding.series`)은 **`CanvasPanel` 이 조회 결과를 옮겨 놓는 그 공간**이어야 한다
 * (`CanvasPanel.resolveSeriesReadings`). 그 공간은 소스마다 다르다:
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

/** 배열 안에서 쓰이지 않은 요소 id 를 만든다(결정적 — 테스트가 값을 예측할 수 있다). */
function nextElementId(elements: readonly CanvasElement[]): string {
  const used = new Set(elements.map((e) => e.id));
  let n = 1;
  while (used.has(`el-${n}`)) n++;
  return `el-${n}`;
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

/** 신규 요소. 기본 스타일은 **비워** 둔다 — 사용자가 고르지 않은 색을 넣지 않는다. */
function newElement(id: string, kind: CanvasElementKind): CanvasElement {
  switch (kind) {
    case 'rect':
      return { id, kind, geometry: { ...DEFAULT_BOX_GEOMETRY }, style: {} };
    case 'ellipse':
      return { id, kind, geometry: { ...DEFAULT_BOX_GEOMETRY }, style: {} };
    case 'line':
      return { id, kind, geometry: { ...DEFAULT_LINE_GEOMETRY }, style: {} };
    default:
      return { id, kind, geometry: { ...DEFAULT_POINT_GEOMETRY }, style: {} };
  }
}

/**
 * 기하 좌표 입력 파싱. 빈 칸·비수치는 0 으로 본다(규칙 표 임계값과 같은 규율).
 *
 * **0..1 로 자르지 않는다.** 스테이지 밖으로 일부 걸치는 배치도 뜻이 있는 저술이며,
 * 렌더러가 그것을 허용한다(파서의 좌표 정책과 같은 이유). 다만 비유한 값은 막는다 —
 * NaN 이 기하에 들어가면 그 요소는 화면에서 통째로 사라진다.
 */
function parseCoordinate(raw: string): number {
  const n = Number(raw.trim());
  return Number.isFinite(n) ? n : 0;
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

// --- 하위 표현 -----------------------------------------------------------

/** 기하 좌표 한 칸. 축 이름을 눈에 보이게 붙여 어느 칸이 무엇인지 알 수 있게 한다. */
function GeometryInput({
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
    <label className="flex min-w-0 flex-1 items-center gap-1">
      <span className="shrink-0 text-[10px] text-(--color-text-muted)">{axis}</span>
      <input
        type="number"
        step="any"
        value={value}
        onChange={(e) => onChange(parseCoordinate(e.target.value))}
        aria-label={ariaLabel}
        data-testid={testId}
        className={cn(INPUT_CLASS, 'w-full text-center tabular-nums')}
      />
    </label>
  );
}

// --- 컴포넌트 -----------------------------------------------------------

/**
 * 캔버스 요소 목록 편집기.
 *
 * 제어 컴포넌트다 — 내부 상태를 들지 않고 매 편집마다 config 패치를 올린다. 추가할
 * 종류만은 상태가 필요해 보이지만, 그것도 두지 않는다: 종류 선택 상자를 각 종류마다
 * 하나씩 두는 대신 "종류별 추가 버튼" 으로 두면 상태 없이도 같은 일이 된다.
 */
export default function CanvasElementsEditor({ config, onConfigChange }: CanvasElementsEditorProps) {
  const { t } = useTranslation();

  const cfg = useMemo(() => parseCanvasConfig(config), [config]);
  const elements = cfg.elements;
  const seriesOptions = useMemo(() => buildSeriesOptions(config), [config]);

  const emit = (next: CanvasElement[]): void => onConfigChange({ elements: next });

  const replaceAt = (idx: number, el: CanvasElement): void => {
    emit(elements.map((e, i) => (i === idx ? el : e)));
  };

  const addElement = (kind: CanvasElementKind): void => {
    emit([...elements, newElement(nextElementId(elements), kind)]);
  };

  const removeAt = (idx: number): void => emit(elements.filter((_, i) => i !== idx));

  /** 순서 이동 = z-order 조작. 001 의 유일한 z-order 수단이라 일급 동작이다. */
  const moveAt = (idx: number, delta: -1 | 1): void => {
    const target = idx + delta;
    if (target < 0 || target >= elements.length) return;
    const next = elements.slice();
    const [moved] = next.splice(idx, 1);
    if (moved !== undefined) next.splice(target, 0, moved);
    emit(next);
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

      {/* 001 의 z-order 는 배열 순서 하나뿐이다 — 그 사실을 화면에 적는다. */}
      <p className="px-1 text-[10px] leading-tight text-(--color-text-muted)">
        {t('dashboard.canvas.elements.orderHint')}
      </p>

      {elements.length === 0 ? (
        <p className="px-1 text-[10px] text-(--color-text-muted)" data-testid="canvas-element-empty">
          {t('dashboard.canvas.elements.empty')}
        </p>
      ) : (
        <div className="space-y-2">
          {elements.map((el, idx) => {
            const bindingOptions = bindingOptionsFor(seriesOptions, el.binding?.series);
            const unbound = el.binding === undefined;

            return (
              <div
                key={el.id}
                data-testid={`canvas-element-${idx}`}
                data-element-id={el.id}
                className="space-y-1.5 rounded-md border border-(--color-border-default) p-1.5"
              >
                {/* 1행: 순번 · 종류 · 순서 이동 · 삭제 */}
                <div className="flex w-full items-center gap-1.5">
                  <span
                    className="flex h-4 w-4 shrink-0 items-center justify-center rounded bg-(--color-bg-elevated) text-[9px] tabular-nums text-(--color-text-muted)"
                    data-testid={`canvas-element-order-${idx}`}
                  >
                    {idx + 1}
                  </span>

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

                  <span className="min-w-0 flex-1 truncate text-[10px] text-(--color-text-muted)">
                    {el.id}
                  </span>

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

                {/* 2행: 기하. 종류마다 요구하는 칸이 다르므로 종류별로 갈라 그린다 —
                    합집합을 하나의 map 으로 접으면 `kind` 와 `geometry` 의 짝을 컴파일러가
                    확인하지 못한다. 좌표는 정규화(0..1)이며 라벨로 그 사실을 알린다. */}
                <div className="flex w-full flex-wrap items-center gap-1.5">
                  <span className={GROUP_LABEL_CLASS}>
                    {t('dashboard.canvas.elements.geometryLabel')}
                  </span>
                  {(el.kind === 'rect' || el.kind === 'ellipse') &&
                    BOX_AXES.map((axis) => (
                      <GeometryInput
                        key={axis}
                        axis={axis}
                        value={el.geometry[axis]}
                        onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                        ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                        testId={`canvas-element-geo-${axis}-${idx}`}
                      />
                    ))}
                  {el.kind === 'line' &&
                    LINE_AXES.map((axis) => (
                      <GeometryInput
                        key={axis}
                        axis={axis}
                        value={el.geometry[axis]}
                        onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                        ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                        testId={`canvas-element-geo-${axis}-${idx}`}
                      />
                    ))}
                  {el.kind === 'text' &&
                    POINT_AXES.map((axis) => (
                      <GeometryInput
                        key={axis}
                        axis={axis}
                        value={el.geometry[axis]}
                        onChange={(v) => replaceAt(idx, { ...el, geometry: { ...el.geometry, [axis]: v } })}
                        ariaLabel={withIndex(t('dashboard.canvas.elements.geoAria'), idx).replace('{axis}', axis)}
                        testId={`canvas-element-geo-${axis}-${idx}`}
                      />
                    ))}
                </div>

                {/* 3행: 기본 스타일. 비운 칸은 미지정이며 렌더측 기본을 따른다. */}
                <div className="flex w-full flex-wrap items-center gap-1.5">
                  <span className={GROUP_LABEL_CLASS}>
                    {t('dashboard.canvas.elements.styleLabel')}
                  </span>

                  <ColorSwatchButton
                    color={el.style.fill}
                    onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'fill', c) })}
                    ariaLabel={withIndex(t('dashboard.canvas.elements.fillAria'), idx)}
                    testId={`canvas-element-fill-${idx}`}
                  />
                  <ColorSwatchButton
                    color={el.style.stroke}
                    onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'stroke', c) })}
                    ariaLabel={withIndex(t('dashboard.canvas.elements.strokeAria'), idx)}
                    testId={`canvas-element-stroke-${idx}`}
                  />
                  <ColorSwatchButton
                    color={el.style.textColor}
                    onChange={(c) => replaceAt(idx, { ...el, style: setStyleField(el.style, 'textColor', c) })}
                    ariaLabel={withIndex(t('dashboard.canvas.elements.textColorAria'), idx)}
                    testId={`canvas-element-text-color-${idx}`}
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

                  {/* 표시 여부는 3지 선택이다 — 체크박스로는 "미지정"과 "숨김"을 구분할 수
                      없고, 그 둘은 뜻이 다르다(규칙 표의 같은 컨트롤과 동일한 판단). */}
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
                </div>

                {/* 4행: 문구 템플릿 · 소수 자리 · 단위. 토큰 3종을 옆에 적어 사용자가
                    명세를 찾아보지 않아도 되게 한다. */}
                <div className="flex w-full flex-wrap items-center gap-1.5">
                  <span className={GROUP_LABEL_CLASS}>
                    {t('dashboard.canvas.elements.textLabel')}
                  </span>
                  <input
                    type="text"
                    value={el.text ?? ''}
                    onChange={(e) =>
                      replaceAt(idx, setElementField(el, 'text', optionalText(e.target.value)))
                    }
                    placeholder={t('dashboard.canvas.elements.textPlaceholder')}
                    aria-label={withIndex(t('dashboard.canvas.elements.textAria'), idx)}
                    data-testid={`canvas-element-text-${idx}`}
                    className={cn(INPUT_CLASS, 'min-w-16 flex-1')}
                  />
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
                <p
                  className="px-1 text-[10px] leading-tight text-(--color-text-muted)"
                  data-testid={`canvas-element-token-help-${idx}`}
                >
                  {t('dashboard.canvas.elements.tokenHelp')}
                </p>

                {/* 5행: 바인딩 · 요소 트윈 덮어쓰기. 바인딩이 없으면 정적 도형이며,
                    규칙은 평가되지 않는다(그래서 아래 표가 읽기 전용으로 잠긴다). */}
                <div className="flex w-full flex-wrap items-center gap-1.5">
                  <span className={GROUP_LABEL_CLASS}>
                    {t('dashboard.canvas.elements.bindingLabel')}
                  </span>
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

                  <span className={GROUP_LABEL_CLASS}>
                    {t('dashboard.canvas.elements.tweenLabel')}
                  </span>
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
                </div>

                {/* 6행: 조건 규칙 표. 별도 컴포넌트다(§위험 R4) — 여기서는 붙이기만 한다. */}
                <div className="rounded border border-(--color-border-default) p-1.5">
                  <CanvasRuleTableEditor
                    rules={el.rules}
                    onChange={(rules: RuleRow[] | undefined) =>
                      replaceAt(idx, setElementField(el, 'rules', rules))
                    }
                    disabled={unbound}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 추가 — 종류마다 버튼 하나. 선택 상자 + 추가 버튼으로 두면 "무엇을 추가할지" 가
          컴포넌트 상태가 되는데, 그 상태는 저장되지도 쓰이지도 않는다. */}
      <div className="flex flex-wrap items-center gap-1.5">
        <Plus className="h-3 w-3 text-blue-500" />
        {ELEMENT_KINDS.map((k) => (
          <button
            key={k}
            type="button"
            onClick={() => addElement(k)}
            className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] font-medium text-blue-500 hover:text-blue-600"
            data-testid={`canvas-element-add-${k}`}
          >
            {t(KIND_LABEL_KEY[k])}
          </button>
        ))}
      </div>
    </div>
  );
}
