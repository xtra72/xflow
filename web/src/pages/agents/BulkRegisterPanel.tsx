// 일괄 등록 패널 (역사 / 위치 / 디바이스 공용).
//
// textarea + 제출 + 파싱 도움말 + 실패 목록. best-effort 실행 결과의 실패 행은
// formatFailure 로 변환해 인라인 표시한다. (SPEC-XSFM-001 Wave 2 — 원래
// XsfmStationsTab 내부 로컬 컴포넌트였으나 디바이스 탭 일괄 등록과 공유하기 위해 추출.)

import { ListPlus } from 'lucide-react';

import type { BulkFailure } from '@/hooks/useStation';

const textareaCls =
  'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)';

export default function BulkRegisterPanel({
  placeholder,
  formatHint,
  value,
  onChange,
  onSubmit,
  submitting,
  submitLabel,
  failures,
  formatFailure,
}: {
  placeholder: string;
  formatHint: string;
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  submitting: boolean;
  submitLabel: string;
  failures: BulkFailure[] | null;
  formatFailure: (f: BulkFailure) => string;
}) {
  return (
    <div className="space-y-2">
      <textarea
        rows={5}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className={textareaCls}
      />
      <p className="text-[11px] text-(--color-text-muted)">{formatHint}</p>
      {failures && failures.length > 0 && (
        <ul className="space-y-0.5 rounded-md border border-red-200 bg-red-50 p-2 dark:border-red-900/40 dark:bg-red-900/20">
          {failures.map((f) => (
            <li key={f.line} className="text-[11px] text-red-600 dark:text-red-400">
              {formatFailure(f)}
            </li>
          ))}
        </ul>
      )}
      <div className="flex justify-end">
        <button
          type="button"
          onClick={onSubmit}
          disabled={submitting || value.trim() === ''}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <ListPlus className="h-3.5 w-3.5" />
          {submitLabel}
        </button>
      </div>
    </div>
  );
}
