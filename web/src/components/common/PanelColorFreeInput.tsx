// 패널 색상 자유 입력 — 네이티브 색상 피커 + 16진 입력칸.
//
// 프리셋 스와치 옆에 붙는 **한 벌짜리 부품**이다. 프리셋 목록과 초기화 버튼은
// 각 화면이 그대로 소유하고(모양이 이미 다르다), 새로 생긴 자유 입력만 여기로
// 모은다 — 같은 설정을 고치는 자유 입력이 두 벌로 갈라지는 것을 막는 것이
// 이 파일의 존재 이유다.
//
// 값 형식 판정은 `normalizePanelColor` 한 곳에만 있다. 화면은 형식을 모른다.

import { useState } from 'react';
import { Pipette } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { normalizePanelColor } from '@/pages/dashboard/panelColorPresets';

/** 값이 없을 때 네이티브 피커가 열어 보일 색. 저장되지는 않는다. */
const FALLBACK = '#3b82f6';

export default function PanelColorFreeInput({
  value,
  onChange,
}: {
  value: string | undefined;
  onChange: (color: string) => void;
}) {
  const { t } = useTranslation();
  // 입력칸은 초안을 따로 쥔다. 치는 도중(`#ab`)에도 막지 않되, 형식이 맞는
  // 순간에만 바깥으로 내보낸다 — 설정에는 완성된 값만 들어간다.
  const [draft, setDraft] = useState(value ?? '');
  const [synced, setSynced] = useState(value);

  // 프리셋 클릭·초기화처럼 바깥에서 값이 바뀌면 입력칸도 따라가야 한다.
  if (value !== synced) {
    setSynced(value);
    setDraft(value ?? '');
  }

  const commit = (next: string) => {
    setDraft(next);
    const normalized = normalizePanelColor(next);
    if (normalized) onChange(normalized);
  };

  return (
    <div className="flex items-center gap-1.5" data-testid="panel-color-free">
      <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md bg-(--color-bg-elevated) transition-colors hover:bg-(--color-border-default)">
        <Pipette className="h-3.5 w-3.5 text-(--color-text-muted)" />
        <input
          type="color"
          data-testid="panel-color-native"
          aria-label={t('panel.color.custom')}
          value={value ?? FALLBACK}
          onChange={(e) => {
            const normalized = normalizePanelColor(e.target.value);
            if (normalized) onChange(normalized);
          }}
          className="absolute inset-0 cursor-pointer opacity-0"
        />
      </label>
      <input
        type="text"
        data-testid="panel-color-hex"
        aria-label={t('panel.color.hex')}
        value={draft}
        spellCheck={false}
        placeholder={FALLBACK}
        maxLength={7}
        onChange={(e) => commit(e.target.value)}
        // 형식이 안 맞는 채로 두고 떠나면 실제 저장값으로 되돌린다 —
        // 화면의 글자와 적용된 색이 어긋난 채 남지 않도록.
        onBlur={() => setDraft(value ?? '')}
        className="h-6 w-20 rounded-md bg-(--color-bg-elevated) px-1.5 text-center font-mono text-[11px] text-(--color-text-secondary) outline-none focus:ring-1 focus:ring-(--color-interactive-primary)"
      />
    </div>
  );
}
