// 조건 규칙 표 편집기 (SPEC-CANVAS-001 T8 · REQ-04).
//
// 한 행은 `[비교 연산자][임계값] → [스타일·문구 패치]` 이고, 표의 **위에서부터 처음
// 일치한 행 하나만** 이긴다. 사용자가 이 화면에서 새로 배우는 개념은 그 한 줄뿐이며,
// 나머지 어휘(연산자·임계값·색 스와치·행 추가/삭제/순서)는 이미 쓰던 임계값 UI 와
// 같은 것이다 — 행 배치·컬러 스와치·↑↓🗑 묶음을 `AcControlThresholdsSection.tsx` 에서
// 그대로 따온 이유다(§위험 R6: "또 다른 문법" 으로 읽히지 않게 한다).
//
// 본문이 `PanelSettingsDialog.tsx`(8,200행+) 가 아니라 여기 있는 이유는 §위험 R4 다.
// 다이얼로그에는 마운트 지점만 두고, 규칙 표는 요소 편집기와도 분리된 컴포넌트로 둔다.
//
// 살아 있는 일치 표시는 `canvasRules.matchesRule` 을 **그대로** 부른다. 비교 규칙을
// 여기서 다시 구현하면 표시와 실제 렌더가 갈라질 수 있고, 그러면 사용자는 화면을
// 믿을 수 없다. 같은 술어를 쓰는 한 편집기와 캔버스는 절대 다른 말을 하지 않는다.
//
// @spec SPEC-CANVAS-001

import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react';

import { FieldHelp } from '@/components/property/FieldHelp';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import ColorPicker from '@/components/common/colorpicker/ColorPicker';
import type {
  ElementFontWeight,
  RuleOp,
  RuleRow,
  RuleValue,
  StylePatch,
} from './canvasConfig';
import { matchesRule } from './canvasRules';
// 백분율 환산 한 쌍(SPEC-CANVAS-012 M5) — 요소·그룹·연결선 칸과 **같은 함수**를 지난다.
import {
  OPACITY_PERCENT_MAX,
  opacityToPercentInput,
  percentInputToOpacity,
} from './opacityPercent';

/** 연산자 선택지. 표시 순서는 §명세 "비교 연산자 집합" 의 나열 순서를 따른다. */
const RULE_OPS: readonly RuleOp[] = ['gt', 'gte', 'lt', 'lte', 'eq', 'ne', 'between', 'nodata'];

/**
 * 연산자 라벨 키. 리터럴 맵으로 두는 이유는 `t()` 인자를 문자열 이어붙이기로 만들면
 * 어떤 키가 실제로 쓰이는지 검색으로 확인할 수 없기 때문이다.
 */
const OP_LABEL_KEY: Record<RuleOp, string> = {
  gt: 'dashboard.canvas.rules.opGt',
  gte: 'dashboard.canvas.rules.opGte',
  lt: 'dashboard.canvas.rules.opLt',
  lte: 'dashboard.canvas.rules.opLte',
  eq: 'dashboard.canvas.rules.opEq',
  ne: 'dashboard.canvas.rules.opNe',
  between: 'dashboard.canvas.rules.opBetween',
  nodata: 'dashboard.canvas.rules.opNodata',
};

/** 신규 행의 기본값. 패치는 **비워** 둔다 — 사용자가 고르지 않은 색을 넣지 않는다. */
const NEW_ROW: RuleRow = { op: 'gt', value: 0, patch: {} };

/** 입력 공통 클래스(기존 임계값 행과 같은 치수). */
const INPUT_CLASS =
  'min-w-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 ' +
  'text-xs text-(--color-text-primary) outline-none focus:border-blue-500 disabled:opacity-50';

/** 순서 이동·삭제 아이콘 버튼 공통 클래스. */
const ICON_BUTTON_CLASS =
  'shrink-0 text-(--color-text-muted) hover:text-(--color-text-secondary) disabled:opacity-30';

/**
 * 안내문 클래스 — 본문(`text-xs`)보다 한 단계 작은 부차 문구다.
 *
 * 9px 는 쓰지 않는다. 이 표와 요소 편집기만 9~10px 로 적혀 있어 같은 다이얼로그 안에서
 * 유독 작게 보였고, 주변 설정 절(`ChartPanelSections`)에는 9px 가 한 군데도 없다.
 */
const HINT_CLASS = 'px-1 text-[11px] leading-tight text-(--color-text-muted)';

/** 순번 배지. 11px 두 자리가 들어가야 하므로 `h-4 w-4`(16px) 로는 좁다. */
const ORDER_BADGE_CLASS =
  'flex h-5 min-w-5 shrink-0 items-center justify-center rounded px-0.5 ' +
  'bg-(--color-bg-elevated) text-[11px] tabular-nums text-(--color-text-muted)';

export interface CanvasRuleTableEditorProps {
  /** 편집 대상 규칙 표. 미지정과 빈 표는 같은 상태다(파서가 빈 표를 미지정으로 접는다). */
  rules: RuleRow[] | undefined;
  /**
   * 변경 통지. 마지막 행을 지우면 `[]` 가 아니라 **`undefined`** 를 보낸다 —
   * `parseCanvasConfig` 가 빈 표를 미지정으로 정규화하므로, `[]` 를 그대로 저장하면
   * 저장 왕복 한 번에 값이 바뀌어 config 가 안정적으로 돌지 않는다.
   */
  onChange: (rules: RuleRow[] | undefined) => void;
  /** 요소의 살아 있는 바인딩 값. 일치 표시에만 쓴다. */
  currentValue?: number | null;
  /** 요소에 바인딩이 없어 규칙이 평가되지 않는 상태. 읽기 전용으로 그린다. */
  disabled?: boolean;
}

/** 연산자별 임계값 칸 수. `nodata` 는 값을 쓰지 않는다(§명세). */
function valueArity(op: RuleOp): 0 | 1 | 2 {
  if (op === 'nodata') return 0;
  return op === 'between' ? 2 : 1;
}

/**
 * 연산자를 바꿀 때 임계값의 **형상**만 맞춘다.
 *
 * 스칼라 ↔ 구간 전환에서 사용자가 적어 둔 수를 최대한 살린다(구간으로 갈 때는 같은
 * 값 두 개, 스칼라로 돌아올 때는 하한). `nodata` 는 값을 쓰지 않으므로 파서와 같은
 * 정규화(0)를 그대로 따른다 — 여기서만 다른 값을 남기면 저장 왕복에 값이 바뀐다.
 * 패치는 어느 경로로도 건드리지 않는다.
 */
function coerceValue(op: RuleOp, prev: RuleValue): RuleValue {
  if (op === 'nodata') return 0;
  if (op === 'between') return Array.isArray(prev) ? prev : [prev, prev];
  return Array.isArray(prev) ? prev[0] : prev;
}

/** 임계값 입력 파싱. 빈 칸·비수치는 0 으로 본다(기존 임계값 행과 같은 규율). */
function parseThreshold(raw: string): number {
  const n = Number(raw.trim());
  return Number.isFinite(n) ? n : 0;
}

/** 패치 수치 입력 파싱. **빈 칸은 미지정**이며 0 이 아니다(아래 `setOrDelete` 참조). */
function parseOptionalNumber(raw: string): number | undefined {
  const v = raw.trim();
  if (v === '') return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

/**
 * 패치의 키 하나를 갈아끼우되, **미지정이면 키 자체를 지운다**.
 *
 * 이 함수가 이 파일에서 가장 중요한 규율이다. 빈 칸을 `''` 나 `0` 으로 직렬화하면
 * 규칙이 일치하는 순간 요소의 기본 스타일이 사용자가 지정한 적 없는 값으로 덮인다
 * (`evaluateRules` 는 `undefined` 가 아닌 값을 전부 덮어쓴다). 키가 없어야만
 * "이 속성은 패치하지 않음" 이 된다.
 */
function setOrDelete<K extends keyof StylePatch>(
  patch: StylePatch,
  key: K,
  value: StylePatch[K],
): StylePatch {
  const next: StylePatch = { ...patch };
  if (value === undefined) delete next[key];
  else next[key] = value;
  return next;
}

/** `{index}` 자리를 1-기반 행 번호로 채운다(기존 임계값 aria 문구 규약). */
function withIndex(label: string, idx: number): string {
  return label.replace('{index}', String(idx + 1));
}

/**
 * 패치 색 칸.
 *
 * 공용 `ColorSwatchButton` 을 그대로 쓰되(새 컬러 픽커를 만들지 않는다), 그 컴포넌트는
 * `disabled` 축을 갖고 있지 않다. 읽기 전용에서 팝오버가 열리면 사용자가 고칠 수 없는
 * 값을 고르는 화면이 되므로, 그때는 색만 보여 주는 잠긴 버튼으로 바꿔 그린다.
 */
function PatchColorField({
  color,
  onChange,
  ariaLabel,
  testId,
  disabled,
}: {
  color: string | undefined;
  onChange: (next: string | undefined) => void;
  ariaLabel: string;
  testId: string;
  disabled: boolean;
}) {
  if (disabled) {
    return (
      <button
        type="button"
        disabled
        aria-label={ariaLabel}
        data-testid={testId}
        className={cn(
          'h-3.5 w-3.5 rounded-full border border-(--color-border-default) opacity-50',
          !color && 'bg-(--color-bg-surface)',
        )}
        style={color ? { backgroundColor: color } : undefined}
      />
    );
  }
  return (
    <ColorPicker
      alpha
      clearable
      value={color}
      onChange={onChange}
      ariaLabel={ariaLabel}
      testId={testId}
    />
  );
}

/**
 * 조건 규칙 표 편집기.
 *
 * 제어 컴포넌트다 — 내부 상태를 들지 않고 매 편집마다 새 배열을 `onChange` 로 올린다.
 */
export default function CanvasRuleTableEditor({
  rules,
  onChange,
  currentValue,
  disabled = false,
}: CanvasRuleTableEditorProps) {
  const { t } = useTranslation();
  const rows: RuleRow[] = Array.isArray(rules) ? rules : [];

  /**
   * 행별 일치 여부. 바인딩이 없으면(=`disabled`) 캔버스가 규칙을 아예 평가하지
   * 않으므로 표시도 하지 않는다 — 화면에 없는 일치를 편집기가 보여 주면 안 된다.
   */
  const matched = rows.map((row) => (disabled ? false : matchesRule(currentValue, row)));
  /** 첫 일치 행. 이 한 줄이 first-match-wins 의 전부다. */
  const winnerIndex = matched.indexOf(true);

  /** 빈 표는 미지정으로 접어서 올린다(위 `onChange` 주석 참조). */
  const emit = (next: RuleRow[]): void => onChange(next.length > 0 ? next : undefined);

  const updateRow = (idx: number, patch: Partial<RuleRow>): void => {
    emit(rows.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  };

  // 아래 세 편집기는 행을 인덱스로 되찾지 않고 **행 자체**를 받는다. 호출부가
  // `rows.map((row, idx) => ...)` 안이라 행은 이미 손에 있고, 되찾으면 절대 참이 될 수
  // 없는 결측 분기가 하나 생긴다 — 도달할 수 없는 방어는 방어가 아니라 죽은 코드다.

  const updatePatch = <K extends keyof StylePatch>(
    row: RuleRow,
    idx: number,
    key: K,
    value: StylePatch[K],
  ): void => {
    updateRow(idx, { patch: setOrDelete(row.patch, key, value) });
  };

  const changeOp = (row: RuleRow, idx: number, op: RuleOp): void => {
    updateRow(idx, { op, value: coerceValue(op, row.value) });
  };

  /**
   * `between` 의 한쪽 끝만 고친다. 저술 순서는 그대로 둔다(정렬은 평가기의 몫).
   *
   * 값이 튜플이 아닌 경우까지 받는 이유는 편집 도중의 행 때문이다 — 연산자만 먼저
   * `between` 으로 바뀌고 값이 아직 스칼라인 상태가 화면에 존재할 수 있으며, 그때
   * 예외로 죽는 대신 그 값 하나를 양 끝으로 벌린다(`canvasRules` 와 같은 규율).
   */
  const changeBound = (row: RuleRow, idx: number, side: 0 | 1, raw: string): void => {
    const cur = Array.isArray(row.value) ? row.value : ([row.value, row.value] as const);
    const next: [number, number] = [cur[0], cur[1]];
    next[side] = parseThreshold(raw);
    updateRow(idx, { value: next });
  };

  const addRow = (): void => emit([...rows, { ...NEW_ROW, patch: {} }]);

  const removeRow = (idx: number): void => emit(rows.filter((_, i) => i !== idx));

  /** 순서 이동 — 우선순위가 곧 순서이므로 부가 기능이 아니라 일급 동작이다. */
  const moveRow = (idx: number, delta: -1 | 1): void => {
    const target = idx + delta;
    if (target < 0 || target >= rows.length) return;
    const next = rows.slice();
    const [moved] = next.splice(idx, 1);
    if (moved !== undefined) next.splice(target, 0, moved);
    emit(next);
  };

  return (
    <div className="space-y-2" data-testid="canvas-rule-table">
      {/* 열 이름 — 명세의 `[조건] → [패치]` 표기를 그대로 화면에 옮긴다.
          이 표에서 새로 배우는 개념(위에서부터 처음 일치하는 행 하나)은 그 열 이름 뒤
          `?` 에 담는다 — 줄로 깔면 요소를 펼칠 때마다 표 위에 안내문이 한 줄씩 선다. */}
      <div className="flex items-center gap-1 px-1 text-xs font-medium text-(--color-text-muted)">
        <span>{t('dashboard.canvas.rules.headerCondition')}</span>
        <span aria-hidden="true">→</span>
        <span>{t('dashboard.canvas.rules.headerPatch')}</span>
        <FieldHelp
          text={t('dashboard.canvas.rules.firstMatchWins')}
          testId="canvas-rule-order-help"
        />
      </div>

      {disabled && (
        <p
          className="rounded border border-(--color-border-default) px-2 py-1 text-[11px] leading-tight text-(--color-text-muted)"
          data-testid="canvas-rule-disabled-hint"
        >
          {t('dashboard.canvas.rules.disabledHint')}
        </p>
      )}

      {rows.length === 0 ? (
        <p className={cn(HINT_CLASS, 'leading-normal')} data-testid="canvas-rule-empty">
          {t('dashboard.canvas.rules.empty')}
        </p>
      ) : (
        <div className="space-y-1.5">
          {rows.map((row, idx) => {
            const isWinner = idx === winnerIndex;
            const isSuperseded = matched[idx] === true && !isWinner;
            const state = isWinner ? 'winner' : isSuperseded ? 'superseded' : 'none';
            const arity = valueArity(row.op);
            const low = Array.isArray(row.value) ? row.value[0] : row.value;
            const high = Array.isArray(row.value) ? row.value[1] : row.value;

            return (
              <div
                key={idx}
                data-testid={`canvas-rule-row-${idx}`}
                data-match={state}
                className={cn(
                  'space-y-1 rounded-md border p-1.5',
                  isWinner
                    ? 'border-blue-500 bg-blue-50/40 dark:bg-blue-900/20'
                    : 'border-(--color-border-default)',
                  // 가려진 행은 흐리게 — 실제 렌더에서 이 행의 패치는 쓰이지 않는다.
                  isSuperseded && 'opacity-60',
                )}
              >
                {/* 1행: 순번 · 조건 · 일치 표시 · 순서/삭제 */}
                <div className="flex w-full items-center gap-1.5">
                  <span
                    className={ORDER_BADGE_CLASS}
                    data-testid={`canvas-rule-order-${idx}`}
                  >
                    {idx + 1}
                  </span>

                  <select
                    value={row.op}
                    disabled={disabled}
                    onChange={(e) => changeOp(row, idx, e.target.value as RuleOp)}
                    aria-label={withIndex(t('dashboard.canvas.rules.opAria'), idx)}
                      data-testid={`canvas-rule-op-${idx}`}
                    className={cn(INPUT_CLASS, 'shrink-0')}
                  >
                    {RULE_OPS.map((op) => (
                      <option key={op} value={op}>
                        {t(OP_LABEL_KEY[op])}
                      </option>
                    ))}
                  </select>

                  {arity === 1 && (
                    <input
                      type="number"
                      step="any"
                      value={low}
                      disabled={disabled}
                      onChange={(e) => updateRow(idx, { value: parseThreshold(e.target.value) })}
                      aria-label={withIndex(t('dashboard.canvas.rules.valueAria'), idx)}
                      data-testid={`canvas-rule-value-${idx}`}
                      className={cn(INPUT_CLASS, 'flex-1 text-center tabular-nums')}
                    />
                  )}
                  {arity === 2 && (
                    <>
                      <input
                        type="number"
                        step="any"
                        value={low}
                        disabled={disabled}
                        onChange={(e) => changeBound(row, idx, 0, e.target.value)}
                        aria-label={withIndex(t('dashboard.canvas.rules.valueLowAria'), idx)}
                      data-testid={`canvas-rule-value-low-${idx}`}
                        className={cn(INPUT_CLASS, 'flex-1 text-center tabular-nums')}
                      />
                      <span className="shrink-0 text-[11px] text-(--color-text-muted)">~</span>
                      <input
                        type="number"
                        step="any"
                        value={high}
                        disabled={disabled}
                        onChange={(e) => changeBound(row, idx, 1, e.target.value)}
                        aria-label={withIndex(t('dashboard.canvas.rules.valueHighAria'), idx)}
                      data-testid={`canvas-rule-value-high-${idx}`}
                        className={cn(INPUT_CLASS, 'flex-1 text-center tabular-nums')}
                      />
                    </>
                  )}
                  {/* `nodata` 는 임계값이 없다 — 빈 자리를 남겨 행 높이를 맞춘다. */}
                  {arity === 0 && <span className="flex-1" />}

                  {isWinner && (
                    <span
                      className="shrink-0 rounded bg-blue-600 px-1 py-px text-[11px] font-medium text-white"
                      data-testid={`canvas-rule-badge-${idx}`}
                    >
                      {t('dashboard.canvas.rules.matchWinner')}
                    </span>
                  )}
                  {isSuperseded && (
                    <span
                      className="shrink-0 rounded bg-(--color-bg-elevated) px-1 py-px text-[11px] font-medium text-(--color-text-muted) line-through"
                      title={t('dashboard.canvas.rules.matchSupersededHint')}
                      data-testid={`canvas-rule-badge-${idx}`}
                    >
                      {t('dashboard.canvas.rules.matchSuperseded')}
                    </span>
                  )}

                  <button
                    type="button"
                    onClick={() => moveRow(idx, -1)}
                    disabled={disabled || idx === 0}
                    className={ICON_BUTTON_CLASS}
                    aria-label={withIndex(t('dashboard.canvas.rules.moveUpAria'), idx)}
                    data-testid={`canvas-rule-move-up-${idx}`}
                  >
                    <ChevronUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => moveRow(idx, 1)}
                    disabled={disabled || idx === rows.length - 1}
                    className={ICON_BUTTON_CLASS}
                    aria-label={withIndex(t('dashboard.canvas.rules.moveDownAria'), idx)}
                    data-testid={`canvas-rule-move-down-${idx}`}
                  >
                    <ChevronDown className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => removeRow(idx)}
                    disabled={disabled}
                    className="shrink-0 text-(--color-text-muted) hover:text-red-500 disabled:opacity-30"
                    aria-label={withIndex(t('dashboard.canvas.rules.deleteAria'), idx)}
                    data-testid={`canvas-rule-delete-${idx}`}
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>

                {/* 2행: 패치. 비운 칸은 패치하지 않는다(= 기본 스타일 유지). */}
                <div className="flex w-full flex-wrap items-center gap-1.5 pl-5">
                  <span aria-hidden="true" className="shrink-0 text-[11px] text-(--color-text-muted)">
                    →
                  </span>

                  <PatchColorField
                    color={row.patch.fill}
                    onChange={(c) => updatePatch(row, idx, 'fill', c)}
                    ariaLabel={withIndex(t('dashboard.canvas.rules.fillAria'), idx)}
                    testId={`canvas-rule-fill-${idx}`}
                    disabled={disabled}
                  />
                  <PatchColorField
                    color={row.patch.stroke}
                    onChange={(c) => updatePatch(row, idx, 'stroke', c)}
                    ariaLabel={withIndex(t('dashboard.canvas.rules.strokeAria'), idx)}
                    testId={`canvas-rule-stroke-${idx}`}
                    disabled={disabled}
                  />
                  <PatchColorField
                    color={row.patch.textColor}
                    onChange={(c) => updatePatch(row, idx, 'textColor', c)}
                    ariaLabel={withIndex(t('dashboard.canvas.rules.textColorAria'), idx)}
                    testId={`canvas-rule-text-color-${idx}`}
                    disabled={disabled}
                  />

                  <input
                    type="number"
                    step="any"
                    min={0}
                    value={row.patch.strokeWidth ?? ''}
                    disabled={disabled}
                    onChange={(e) =>
                      updatePatch(row, idx, 'strokeWidth', parseOptionalNumber(e.target.value))
                    }
                    placeholder={t('dashboard.canvas.rules.strokeWidthPlaceholder')}
                    aria-label={withIndex(t('dashboard.canvas.rules.strokeWidthAria'), idx)}
                      data-testid={`canvas-rule-stroke-width-${idx}`}
                    className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                  />
                  {/* 012 M5 — 칸은 백분율, 저장은 0..1 그대로다. 요소·그룹·연결선 칸과
                      **같은 함수 한 쌍**을 지난다(AC-28).

                      이 칸은 종전에 0..1 로 죄지 **않았다** — `1.5` 가 저장에 남을 수 있었고
                      그 값은 `resolveAlpha` 가 1 로 죄어 그렸다. 이제 칸이 그려지는 수를
                      보이고(100) 쓸 때도 죈다. */}
                  <input
                    type="number"
                    step={1}
                    min={0}
                    max={OPACITY_PERCENT_MAX}
                    value={opacityToPercentInput(row.patch.opacity)}
                    disabled={disabled}
                    onChange={(e) =>
                      updatePatch(row, idx, 'opacity', percentInputToOpacity(e.target.value))
                    }
                    placeholder={t('dashboard.canvas.rules.opacityPlaceholder')}
                    aria-label={withIndex(t('dashboard.canvas.rules.opacityAria'), idx)}
                      data-testid={`canvas-rule-opacity-${idx}`}
                    className={cn(INPUT_CLASS, 'w-12 text-center tabular-nums')}
                  />

                  <select
                    value={row.patch.fontWeight ?? ''}
                    disabled={disabled}
                    onChange={(e) =>
                      updatePatch(
                        row,
                        idx,
                        'fontWeight',
                        e.target.value === '' ? undefined : (e.target.value as ElementFontWeight),
                      )
                    }
                    aria-label={withIndex(t('dashboard.canvas.rules.fontWeightAria'), idx)}
                      data-testid={`canvas-rule-font-weight-${idx}`}
                    className={cn(INPUT_CLASS, 'shrink-0')}
                  >
                    <option value="">{t('dashboard.canvas.rules.unset')}</option>
                    <option value="normal">{t('dashboard.canvas.rules.fontWeightNormal')}</option>
                    <option value="bold">{t('dashboard.canvas.rules.fontWeightBold')}</option>
                  </select>

                  {/*
                    표시 여부는 체크박스가 아니라 3지 선택이다 — 체크박스로는 "미지정" 과
                    "숨김" 을 구분할 수 없고, 그 둘은 뜻이 다르다(미지정은 기본 스타일 유지).
                  */}
                  <select
                    value={row.patch.visible === undefined ? '' : row.patch.visible ? 'show' : 'hide'}
                    disabled={disabled}
                    onChange={(e) =>
                      updatePatch(
                        row,
                        idx,
                        'visible',
                        e.target.value === '' ? undefined : e.target.value === 'show',
                      )
                    }
                    aria-label={withIndex(t('dashboard.canvas.rules.visibleAria'), idx)}
                      data-testid={`canvas-rule-visible-${idx}`}
                    className={cn(INPUT_CLASS, 'shrink-0')}
                  >
                    <option value="">{t('dashboard.canvas.rules.unset')}</option>
                    <option value="show">{t('dashboard.canvas.rules.visibleShow')}</option>
                    <option value="hide">{t('dashboard.canvas.rules.visibleHide')}</option>
                  </select>

                  <input
                    type="text"
                    value={row.patch.text ?? ''}
                    disabled={disabled}
                    onChange={(e) =>
                      updatePatch(
                        row,
                        idx,
                        'text',
                        e.target.value === '' ? undefined : e.target.value,
                      )
                    }
                    placeholder={t('dashboard.canvas.rules.textPlaceholder')}
                    aria-label={withIndex(t('dashboard.canvas.rules.textAria'), idx)}
                      data-testid={`canvas-rule-text-${idx}`}
                    className={cn(INPUT_CLASS, 'min-w-16 flex-1')}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}

      <button
        type="button"
        onClick={addRow}
        disabled={disabled}
        className="flex items-center gap-1 px-1 text-xs font-medium text-blue-500 hover:text-blue-600 disabled:opacity-30"
        data-testid="canvas-rule-add"
      >
        <Plus className="h-3 w-3" />
        {t('dashboard.canvas.rules.addRule')}
      </button>
    </div>
  );
}
