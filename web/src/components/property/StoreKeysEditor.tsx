// Store 에이전트의 정적 키 + data_type + field + 태그 목록 에디터.
//
// 각 행은 `{ key, data_type?, field?, tags }` 형상이며, 백엔드 SPEC-STORE-003
// v0.3.0 의 `StoreKeyObject` 와 1:1 매핑된다 (registration 필드 제외 — 이는 폼이 아닌
// 런타임 상태이므로 yaml 에는 저장되지 않는다).
//
// v0.7.0 진화 (M13):
//   - `data_type` 셀렉트 컬럼 (6종 enum). manual 모드에서 필수, auto 모드에서 선택.
//   - `field` 텍스트 컬럼 (정규식 `^[a-zA-Z0-9_-]+$`, 기본 placeholder `"unknown"`).
//   - 컬럼 너비 비율 키:data_type:field:태그 = `4:1.5:1.5:3`.
//   - 부모 (StoreConfigEditor) 가 `registrationType` prop 을 전달하여 검증 정책을 제어.
//   - `onValidityChange` 콜백으로 부모의 저장 버튼을 게이팅한다.
//
// 유효성 검증은 클라이언트 측에서 즉시 수행하며 인라인 에러로 표시한다.
// 부모는 `onValidityChange(false)` 수신 시 저장 버튼을 비활성화해야 한다.
//
// v0.7.0 진화 (Task 14, Phase F):
//   - 각 행의 키 입력 옆에 `MetadataChips` 의 `manual` 배지를 표시하여,
//     이 에디터가 yaml 정적 정의(=manual 등록)임을 시각적으로 명확히 한다.
//   - TsdbDataViewerModal 시리즈 행의 auto/manual 배지와 동일 컴포넌트를 사용해 UX 일관성 확보.
//
// @spec SPEC-WEB-005 v0.7.0 (M13, Task 14)
// @spec SPEC-STORE-003 v0.3.0

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
} from 'react';
import { Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import type { DataType, RegistrationSource } from '@/services/api/store';
import { MetadataChips } from './MetadataChips';
import {
  DATA_TYPE_OPTIONS,
  validateDataType,
  validateMetricType,
} from './storeKeysValidation';

// ---- 외부 값 타입 ----

/**
 * 저장되는 단일 엔트리. 백엔드 SPEC-STORE-003 v0.3.0 `StoreKeyObject` 와 매핑된다.
 *
 * v0.7.0 (M13) BREAKING:
 *   - `data_type?: DataType` 필드 추가 (manual 모드 필수, auto 모드 선택).
 *   - `field?: string` 필드 추가 (정규식 검증, 기본값 `"unknown"`).
 *
 * @spec SPEC-WEB-005 v0.7.0 (M13)
 * @spec SPEC-STORE-003 v0.3.0
 */
export interface StoreKeyEntry {
  key: string;
  data_type?: DataType;
  field?: string;
  tags: Record<string, string>;
}

// ---- Props ----

interface StoreKeysEditorProps {
  /** 배열이 아니거나 undefined 이면 빈 목록으로 취급한다 (설정이 비어있는 초기 상태 대응). */
  value: unknown;
  onChange: (next: StoreKeyEntry[]) => void;
  /**
   * 부모의 `registration_type` 값.
   * - `manual`: data_type 필수, 미입력 행 인라인 에러
   * - `auto`: data_type 선택 사항
   *
   * 기본값 `auto` (legacy fallback). 부모는 가능한 한 명시적으로 전달한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M13)
   */
  registrationType?: RegistrationSource;
  /**
   * 검증 결과 변경 시 호출되는 콜백.
   * 부모는 `valid=false` 수신 시 저장 버튼을 비활성화해야 한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M13)
   */
  onValidityChange?: (valid: boolean) => void;
  readOnly?: boolean;
}

// ---- 내부 행 타입 ----
// React 리스트 렌더링에서 안정적 `key` 를 보장하기 위해 내부 행마다 id 를 부여한다.
// 외부 값(StoreKeyEntry[]) 로 변환 시 id 는 제거된다.

interface InternalRow {
  id: string;
  key: string;
  data_type: string; // 빈 문자열 = unset
  field: string; // 빈 문자열 = default unknown 적용
  tags: { tagId: string; k: string; v: string }[];
}

// ---- 변환 유틸 ----

let rowCounter = 0;
function nextRowId(): string {
  return `row-${++rowCounter}-${Date.now()}`;
}

let tagCounter = 0;
function nextTagId(): string {
  return `tag-${++tagCounter}-${Date.now()}`;
}

/** 외부 value → 내부 rows 로 변환. 알 수 없는 형상은 빈 목록으로 처리한다. */
function toRows(value: unknown): InternalRow[] {
  if (!Array.isArray(value)) return [];
  const rows: InternalRow[] = [];
  for (const item of value) {
    if (!item || typeof item !== 'object') continue;
    const rec = item as Record<string, unknown>;
    const key = typeof rec.key === 'string' ? rec.key : '';
    const dataType =
      typeof rec.data_type === 'string' ? rec.data_type : '';
    const fieldName =
      typeof rec.field === 'string' ? rec.field : '';
    const rawTags =
      rec.tags && typeof rec.tags === 'object' && !Array.isArray(rec.tags)
        ? (rec.tags as Record<string, unknown>)
        : {};
    const tags = Object.entries(rawTags).map(([k, v]) => ({
      tagId: nextTagId(),
      k,
      v: typeof v === 'string' ? v : String(v ?? ''),
    }));
    rows.push({
      id: nextRowId(),
      key,
      data_type: dataType,
      field: fieldName,
      tags,
    });
  }
  return rows;
}

/** 내부 rows → 외부 StoreKeyEntry[] (id/tagId 제거). */
function toEntries(rows: InternalRow[]): StoreKeyEntry[] {
  return rows.map((row) => {
    const tagsObj: Record<string, string> = {};
    for (const t of row.tags) {
      // 빈 태그 키는 생략 (부분 입력 중 상태 대응).
      if (t.k === '') continue;
      tagsObj[t.k] = t.v;
    }
    const entry: StoreKeyEntry = { key: row.key, tags: tagsObj };
    // data_type / field 은 비어있을 때만 yaml 에서 생략한다.
    // 빈 문자열을 그대로 보내면 백엔드가 default 적용을 못할 수 있다.
    if (row.data_type !== '') {
      entry.data_type = row.data_type as DataType;
    }
    if (row.field !== '') {
      entry.field = row.field;
    }
    return entry;
  });
}

// ---- 검증 ----

/** 태그 키 허용 문자: 영문/숫자/언더스코어/하이픈. */
const TAG_KEY_REGEX = /^[a-zA-Z0-9_-]+$/;

function isValidTagKey(k: string): boolean {
  return k === '' || TAG_KEY_REGEX.test(k);
}

/** 중복된 Store 키 목록을 반환한다 (비어있는 키는 제외). */
function findDuplicateKeys(rows: InternalRow[]): Set<string> {
  const seen = new Map<string, number>();
  for (const row of rows) {
    if (row.key === '') continue;
    seen.set(row.key, (seen.get(row.key) ?? 0) + 1);
  }
  const dupes = new Set<string>();
  for (const [k, count] of seen) {
    if (count > 1) dupes.add(k);
  }
  return dupes;
}

// ---- 스타일 ----

const inputCls = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

// readOnly 스타일.
//
// 주의: input 의 readOnly attr 와 함께 사용한다. disabled 를 적용하면
// 브라우저가 텍스트를 흐리게(회색조로) 렌더링하여 다크모드에서 값이
// 거의 보이지 않는 가시성 회귀가 발생한다 (commit b4ad829 와 동일한 패턴).
// opacity 는 낮추지 않고 배경만 살짝 다르게 표시한다.
const readOnlyCls = 'cursor-not-allowed bg-(--color-bg-elevated)';

// 에러 상태 input 스타일 — amber 톤으로 인라인 경고 표시.
const errorInputCls =
  'border-amber-400 focus:border-amber-500 focus:ring-amber-400';

const chipCls = cn(
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5',
  'bg-(--color-bg-elevated) text-xs font-medium text-(--color-text-secondary)',
);

const chipWarnCls = cn(
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5',
  'bg-amber-100 text-xs font-medium text-amber-800',
  'dark:bg-amber-900/40 dark:text-amber-300',
);

// ---- 컴포넌트 ----

export function StoreKeysEditor({
  value,
  onChange,
  registrationType = 'auto',
  onValidityChange,
  readOnly,
}: StoreKeysEditorProps) {
  const { t } = useTranslation();
  // 내부 상태로 행을 관리해 안정적 key 를 보장한다.
  // 외부 value 가 완전히 다른 객체로 교체되면(새 에이전트 로드, 취소 등) 동기화한다.
  const lastExternalRef = useRef<unknown>(undefined);
  const [rows, setRows] = useState<InternalRow[]>(() => toRows(value));

  if (value !== lastExternalRef.current) {
    // 얕은 비교: 외부 엔트리 개수나 키/메타데이터가 다르면 전체 리셋.
    const externalEntries = toEntries(toRows(value));
    const internalEntries = toEntries(rows);
    const serialize = (list: StoreKeyEntry[]) =>
      JSON.stringify(
        list
          .map((e) => ({
            key: e.key,
            data_type: e.data_type ?? '',
            field: e.field ?? '',
            tags: { ...e.tags },
          }))
          .sort((a, b) => a.key.localeCompare(b.key)),
      );
    if (serialize(externalEntries) !== serialize(internalEntries)) {
      setRows(toRows(value));
    }
    lastExternalRef.current = value;
  }

  const emit = useCallback(
    (next: InternalRow[]) => {
      setRows(next);
      const entries = toEntries(next);
      lastExternalRef.current = value; // ref 는 동일한 외부 참조를 유지하여 루프 방지.
      onChange(entries);
    },
    [onChange, value],
  );

  // --- 행 추가/삭제 ---

  const handleAddRow = useCallback(() => {
    // 새 행의 data_type 기본값:
    //   - manual: unset (사용자가 명시 선택해야 함, 인라인 에러로 유도)
    //   - auto: unset 상태로 두되 백엔드가 추론/default 적용
    // SPEC M13 주: "auto 의 경우 'float' 기본값" 옵션이 spec 에 있으나,
    // UI 일관성을 위해 placeholder 로 시각적 'optional' 표기만 적용하고 실제 값은 unset.
    emit([
      ...rows,
      {
        id: nextRowId(),
        key: '',
        data_type: '',
        field: '',
        tags: [],
      },
    ]);
  }, [rows, emit]);

  const handleRemoveRow = useCallback(
    (rowId: string) => {
      emit(rows.filter((r) => r.id !== rowId));
    },
    [rows, emit],
  );

  // --- 키 / data_type / field 편집 ---

  const handleKeyChange = useCallback(
    (rowId: string, nextKey: string) => {
      emit(rows.map((r) => (r.id === rowId ? { ...r, key: nextKey } : r)));
    },
    [rows, emit],
  );

  const handleDataTypeChange = useCallback(
    (rowId: string, nextValue: string) => {
      emit(
        rows.map((r) => (r.id === rowId ? { ...r, data_type: nextValue } : r)),
      );
    },
    [rows, emit],
  );

  const handleMetricTypeChange = useCallback(
    (rowId: string, nextValue: string) => {
      emit(
        rows.map((r) =>
          r.id === rowId ? { ...r, field: nextValue } : r,
        ),
      );
    },
    [rows, emit],
  );

  // --- 태그 추가/삭제/편집 ---

  const handleRemoveTag = useCallback(
    (rowId: string, tagId: string) => {
      emit(
        rows.map((r) =>
          r.id === rowId
            ? { ...r, tags: r.tags.filter((t) => t.tagId !== tagId) }
            : r,
        ),
      );
    },
    [rows, emit],
  );

  const handleAddTagFromInput = useCallback(
    (rowId: string, keyInput: string, valInput: string) => {
      const k = keyInput.trim();
      const v = valInput.trim();
      if (k === '') return; // 키가 비어있으면 무시 (부모에 emit 하지 않음)
      emit(
        rows.map((r) =>
          r.id === rowId
            ? {
                ...r,
                tags: [...r.tags, { tagId: nextTagId(), k, v }],
              }
            : r,
        ),
      );
    },
    [rows, emit],
  );

  // --- 검증 결과 (소프트 경고 + 저장 차단) ---

  const duplicateKeys = useMemo(() => findDuplicateKeys(rows), [rows]);

  // 행별 data_type / field 검증 결과 (memoized).
  const rowValidations = useMemo(() => {
    return rows.map((row) => ({
      id: row.id,
      dataType: validateDataType(row.data_type, registrationType),
      fieldName: validateMetricType(row.field),
    }));
  }, [rows, registrationType]);

  // 전체 폼 유효성 — 모든 행의 data_type 과 field 이 통과해야 한다.
  // (중복 키는 soft warning 이므로 저장 차단에 포함하지 않는다 — 기존 동작 유지.)
  const allValid = useMemo(
    () =>
      rowValidations.every(
        (v) => v.dataType.valid && v.fieldName.valid,
      ),
    [rowValidations],
  );

  // 부모에 검증 상태 전파. allValid 변동 시에만 호출.
  useEffect(() => {
    onValidityChange?.(allValid);
  }, [allValid, onValidityChange]);

  // 컬럼 너비 비율 키:data_type:field:태그 = 4:1.5:1.5:3 (M13).
  // readOnly 모드에서는 행 삭제 컬럼 제거.
  return (
    <div className="space-y-2">
      <div className="overflow-x-auto rounded-md border border-(--color-border-default)">
        <table className="w-full table-fixed text-sm">
          <colgroup>
            <col style={{ width: '40%' }} />
            <col style={{ width: '15%' }} />
            <col style={{ width: '15%' }} />
            <col style={{ width: '30%' }} />
            {!readOnly && <col className="w-10" />}
          </colgroup>
          <thead>
            <tr className="bg-(--color-bg-primary)">
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.store.key')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.store.dataType')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.store.fieldName')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.store.tags')}
              </th>
              {!readOnly && <th className="px-2 py-1.5" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={readOnly ? 4 : 5}
                  className="px-2 py-4 text-center text-xs text-(--color-text-muted)"
                >
                  {t('property.store.empty')}
                </td>
              </tr>
            )}
            {rows.map((row, idx) => {
              const isDuplicate = duplicateKeys.has(row.key);
              const validation = rowValidations[idx];
              const dataTypeError = validation?.dataType.error;
              const fieldNameError = validation?.fieldName.error;
              return (
                <tr
                  key={row.id}
                  className="transition-colors hover:bg-(--color-bg-elevated)"
                >
                  {/* 키 입력 + manual 배지 (col 폭 40%, Task 14).
                      이 에디터의 모든 행은 yaml 에 정적 정의되므로 항상 manual 등록이다.
                      TsdbDataViewerModal 의 auto/manual 배지와 동일 컴포넌트로 시각 통일성 확보. */}
                  <td className="px-2 py-1.5 align-top">
                    <div className="flex items-center gap-1.5">
                      <input
                        type="text"
                        value={row.key}
                        // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를
                        // 흐리게 렌더링해 값이 보이지 않게 만든다 (commit b4ad829 참조).
                        readOnly={readOnly}
                        onChange={(e) => handleKeyChange(row.id, e.target.value)}
                        className={cn(
                          inputCls,
                          readOnly && readOnlyCls,
                          isDuplicate && errorInputCls,
                        )}
                        placeholder="indoor/1/temperature"
                        aria-invalid={isDuplicate || undefined}
                        aria-label={t('property.store.keyAria')}
                      />
                      <MetadataChips
                        registration="manual"
                        showAutoBadge
                        className="shrink-0"
                      />
                    </div>
                    {isDuplicate && (
                      <p className="mt-1 text-[10px] text-amber-600 dark:text-amber-400">
                        {t('property.store.duplicateKey')}
                      </p>
                    )}
                  </td>

                  {/* 데이터 타입 셀렉트 (col 폭 15%) */}
                  <td className="px-2 py-1.5 align-top">
                    <select
                      value={row.data_type}
                      disabled={readOnly}
                      onChange={(e) =>
                        handleDataTypeChange(row.id, e.target.value)
                      }
                      className={cn(
                        inputCls,
                        readOnly && readOnlyCls,
                        dataTypeError && errorInputCls,
                      )}
                      aria-invalid={Boolean(dataTypeError) || undefined}
                      aria-label={t('property.store.dataType')}
                    >
                      <option value="">
                        {registrationType === 'manual'
                          ? t('property.store.dataTypeSelectRequired')
                          : t('property.store.dataTypeSelect')}
                      </option>
                      {DATA_TYPE_OPTIONS.map((opt) => (
                        <option key={opt} value={opt}>
                          {opt}
                        </option>
                      ))}
                    </select>
                    {dataTypeError && (
                      <p className="mt-1 text-[10px] text-amber-600 dark:text-amber-400">
                        {dataTypeError}
                      </p>
                    )}
                  </td>

                  {/* 필드 입력 (col 폭 15%) */}
                  <td className="px-2 py-1.5 align-top">
                    <input
                      type="text"
                      value={row.field}
                      readOnly={readOnly}
                      onChange={(e) =>
                        handleMetricTypeChange(row.id, e.target.value)
                      }
                      className={cn(
                        inputCls,
                        readOnly && readOnlyCls,
                        fieldNameError && errorInputCls,
                      )}
                      placeholder={t('property.store.fieldNamePlaceholder')}
                      aria-invalid={Boolean(fieldNameError) || undefined}
                      aria-label={t('property.store.fieldName')}
                    />
                    {fieldNameError && (
                      <p className="mt-1 text-[10px] text-amber-600 dark:text-amber-400">
                        {fieldNameError}
                      </p>
                    )}
                  </td>

                  {/* 태그 칩 + 추가 폼 (col 폭 30%) */}
                  <td className="px-2 py-1.5 align-top">
                    <TagChipsEditor
                      tags={row.tags}
                      readOnly={readOnly}
                      onRemoveTag={(tagId) => handleRemoveTag(row.id, tagId)}
                      onAddTag={(k, v) => handleAddTagFromInput(row.id, k, v)}
                    />
                  </td>

                  {/* 행 삭제 */}
                  {!readOnly && (
                    <td className="px-2 py-1.5 align-top">
                      <button
                        type="button"
                        onClick={() => handleRemoveRow(row.id)}
                        className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        aria-label={t('property.store.deleteRowAria')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* 행 추가 버튼 */}
      {!readOnly && (
        <button
          type="button"
          onClick={handleAddRow}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.store.addRow')}
        </button>
      )}
    </div>
  );
}

// ---- 태그 칩 에디터 (행 내부) ----

/**
 * 단일 행의 태그 목록을 칩으로 표시하고, 하단에 "키=값" 입력 + 추가 버튼을 둔다.
 *
 * 추가 동작: 다음 세 경우에 모두 동일하게 chip 으로 commit 한다.
 *   1. "태그 추가" 버튼 클릭
 *   2. Enter 키 입력
 *   3. 태그 폼 외부로 포커스 이동 (blur with relatedTarget outside form)
 *
 * 3번이 중요한 이유: 사용자가 태그 키/값을 입력한 뒤 "추가" 버튼을 누르지 않고
 * 외부의 "저장" 버튼을 바로 누르면, pending 입력이 부모에 emit 되지 않아
 * 빈 tags 가 저장되는 회귀가 있었다. blur 자동 commit 으로 해당 케이스를 보전한다.
 *
 * 동작 규칙:
 *   - 태그 키가 비어있으면 (어떤 트리거든) no-op.
 *   - 유효하지 않은 키 문자는 chip 을 추가하되 경고 스타일(amber) 로 표시.
 */
function TagChipsEditor({
  tags,
  readOnly,
  onRemoveTag,
  onAddTag,
}: {
  tags: { tagId: string; k: string; v: string }[];
  readOnly?: boolean;
  onRemoveTag: (tagId: string) => void;
  onAddTag: (k: string, v: string) => void;
}) {
  const { t } = useTranslation();
  const [keyInput, setKeyInput] = useState('');
  const [valInput, setValInput] = useState('');
  // 폼 영역 ref — blur 시 relatedTarget 이 폼 안의 다른 요소인지 판별하는 데 사용.
  const formRef = useRef<HTMLDivElement | null>(null);

  const handleAddClick = useCallback(() => {
    if (keyInput.trim() === '') return;
    onAddTag(keyInput, valInput);
    setKeyInput('');
    setValInput('');
  }, [keyInput, valInput, onAddTag]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        handleAddClick();
      }
    },
    [handleAddClick],
  );

  // 폼 외부로 포커스가 이동하면 pending 입력을 자동 commit.
  // 폼 내부 (키 input ↔ 값 input ↔ 추가 버튼) 사이의 포커스 이동은 무시한다.
  const handleBlur = useCallback(
    (e: FocusEvent<HTMLInputElement>) => {
      const next = e.relatedTarget as Node | null;
      if (next && formRef.current && formRef.current.contains(next)) {
        return; // 폼 내부 이동: 무시
      }
      // 폼 외부로 이동 (또는 relatedTarget 이 null=document 등): pending 입력 commit.
      if (keyInput.trim() !== '') {
        handleAddClick();
      }
    },
    [keyInput, handleAddClick],
  );

  return (
    <div className="space-y-1.5">
      {/* 태그 칩 리스트 */}
      <div className="flex flex-wrap gap-1">
        {tags.length === 0 && (
          <span className="text-[11px] text-(--color-text-muted)">{t('property.store.tagsEmpty')}</span>
        )}
        {tags.map((tag) => {
          const valid = isValidTagKey(tag.k);
          return (
            <span key={tag.tagId} className={valid ? chipCls : chipWarnCls}>
              <span className="font-mono">
                {tag.k}={tag.v}
              </span>
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => onRemoveTag(tag.tagId)}
                  className="rounded-full p-0.5 hover:bg-black/10 dark:hover:bg-white/10"
                  aria-label={t('property.store.tagDeleteAria').replace('{key}', tag.k)}
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </span>
          );
        })}
      </div>

      {/* 태그 추가 폼 */}
      {!readOnly && (
        <div ref={formRef} className="flex items-center gap-1.5">
          <input
            type="text"
            value={keyInput}
            onChange={(e) => setKeyInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={handleBlur}
            placeholder={t('property.tag.keyPlaceholder')}
            className={cn(inputCls, 'max-w-[9rem]')}
            aria-label={t('property.tag.keyAria')}
          />
          <span className="text-(--color-text-muted)">=</span>
          <input
            type="text"
            value={valInput}
            onChange={(e) => setValInput(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={handleBlur}
            placeholder={t('property.tag.valuePlaceholder')}
            className={cn(inputCls, 'max-w-[9rem]')}
            aria-label={t('property.tag.valueAria')}
          />
          <button
            type="button"
            onClick={handleAddClick}
            disabled={keyInput.trim() === ''}
            className={cn(
              'inline-flex items-center gap-0.5 rounded-md border border-(--color-border-default) px-2 py-1 text-[11px] font-medium',
              'text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            <Plus className="h-3 w-3" />
            {t('property.tag.add')}
          </button>
        </div>
      )}
    </div>
  );
}
