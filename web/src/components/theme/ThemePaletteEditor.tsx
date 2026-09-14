// 테마 팔레트 편집기 — 설정 → 테마 탭에서만 쓰인다.
//
// 라이트(day) / 다크(night) 팔레트를 각각 컬러 테이블로 편집한다. 편집은 곧바로
// 스토어에 반영되고 useTheme 이 인라인 CSS 변수로 적용하므로 별도의 미리보기
// 배선 없이 화면 전체가 즉시 바뀐다(저장/취소 단계가 없다 — 되돌리기는 초기화).

import { useRef, useState } from 'react';
import { Download, Upload, RotateCcw } from 'lucide-react';

import {
  PRESETS,
  PRESET_IDS,
  TOKEN_CATEGORIES,
  parseThemeFile,
  resolveTokens,
  serializeThemeFile,
  type PresetId,
} from '@/lib/theme/tokens';
import { useTranslation } from '@/lib/i18n';
import ColorPicker from '@/components/common/colorpicker/ColorPicker';
import { useUIStore } from '@/stores/uiStore';
import { ThemePreview } from './ThemePreview';
import { tokensOfPart } from './themePreviewParts';
import { cn } from '@/lib/utils/cn';

interface ThemePaletteEditorProps {
  /** 현재 화면에 적용 중인 팔레트 — 탭 초기 선택과 '적용 중' 배지에 쓴다. */
  activePreset: PresetId;
}

/** 프리셋 식별자 -> 탭 라벨 i18n 키 */
const PRESET_LABEL_KEY: Record<PresetId, string> = {
  day: 'settings.themeLight',
  night: 'settings.themeDark',
};

export function ThemePaletteEditor({ activePreset }: ThemePaletteEditorProps) {
  const { t } = useTranslation();
  const themeOverrides = useUIStore((s) => s.themeOverrides);
  const setThemeToken = useUIStore((s) => s.setThemeToken);
  const setThemeOverrides = useUIStore((s) => s.setThemeOverrides);
  const resetThemeOverrides = useUIStore((s) => s.resetThemeOverrides);
  const addNotification = useUIStore((s) => s.addNotification);

  /** 편집 대상 팔레트. 기본은 지금 적용 중인 쪽. */
  const [target, setTarget] = useState<PresetId>(activePreset);
  /**
   * 커서가 스쳐 가는 토큰. **기본으로는 아무 일도 하지 않는다.**
   *
   * 커서만 지나가도 미리보기가 따라 움직이면, 표를 훑어보는 동안 화면이 쉬지 않고
   * 바뀐다 — 읽으려던 사람이 멀미를 한다. 그래서 따라가기는 `followCursor` 를 켠
   * 사람만 쓴다.
   */
  const [hovered, setHovered] = useState<string | undefined>(undefined);
  /** 커서를 따라갈 것인가. 기본은 **끔** — 고르는 것은 누르는 일이다. */
  const [followCursor, setFollowCursor] = useState(false);
  /** 표에서 눌러 고른 토큰. */
  const [selectedToken, setSelectedToken] = useState<string | undefined>(undefined);
  /** 미리보기에서 고른 조각. 그 조각이 쓰는 토큰 행들이 표에서 함께 표시된다. */
  const [selectedPart, setSelectedPart] = useState<string | undefined>(undefined);

  /**
   * 지금 미리보기가 가리키는 토큰.
   *
   * 커서 따라가기를 켰을 때만 커서가 우선한다. 끄면 누른 것만 남는다.
   */
  const activeToken = (followCursor ? hovered : undefined) ?? selectedToken;

  /** 표에서 표시할 토큰들 — 조각을 골랐으면 그 조각이 쓰는 색 전부. */
  const selectedTokens = new Set(
    selectedPart !== undefined
      ? tokensOfPart(selectedPart)
      : selectedToken === undefined
        ? []
        : [selectedToken],
  );

  /**
   * 표에서 행을 눌러 고른다.
   *
   * 미리보기의 조각 선택과는 **서로를 지운다** — 한 화면에 "이 색" 과 "이 조각" 이
   * 동시에 고정되어 있으면 무엇을 고친 것인지 읽히지 않는다.
   */
  const pickToken = (cssVar: string): void => {
    setSelectedToken(cssVar);
    setSelectedPart(undefined);
  };
  /** 행 스크롤용 — 조각을 고르면 첫 토큰 행을 화면 안으로 데려온다. */
  const rowRefs = useRef(new Map<string, HTMLTableRowElement>());

  /**
   * 미리보기에서 조각을 골랐을 때.
   *
   * 표를 그 조각의 첫 토큰 행으로 데려간다. 표가 24행이라 고른 조각의 색이 화면 밖에
   * 있으면 "골랐다" 는 사실이 아무 데도 보이지 않는다.
   */
  const pickPart = (partId: string): void => {
    setSelectedPart(partId);
    setSelectedToken(undefined);
    setHovered(undefined);
    const first = tokensOfPart(partId)[0];
    if (first === undefined) return;
    const row = rowRefs.current.get(first);
    // jsdom 에는 `scrollIntoView` 가 없다. 없으면 스크롤만 건너뛴다 — 선택 자체는 선다.
    row?.scrollIntoView?.({ block: 'nearest' });
  };
  const fileInputRef = useRef<HTMLInputElement>(null);

  const overrides = themeOverrides[target] ?? {};
  const resolved = resolveTokens(target, overrides);
  const changedCount = Object.keys(overrides).length;

  /** 현재 팔레트를 .json 파일로 내려받는다. */
  function handleExport() {
    const blob = new Blob([serializeThemeFile(target, resolved)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `xflow-theme-${target}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  /** 선택한 파일을 읽어 현재 대상 팔레트에 적용한다. */
  async function handleImportFile(file: File) {
    const parsed = parseThemeFile(await file.text());

    if (!parsed.ok) {
      addNotification({ type: 'error', message: t(`settings.palette.importError.${parsed.error}`) });
      return;
    }

    // 파일에 기록된 preset 과 무관하게 "지금 보고 있는" 팔레트에 적용한다.
    // 라이트 팔레트를 다크 쪽으로 옮겨보는 사용도 막을 이유가 없다.
    setThemeOverrides(target, parsed.tokens);
    addNotification({
      type: parsed.ignored > 0 ? 'warning' : 'success',
      message:
        parsed.ignored > 0
          ? `${t('settings.palette.imported')} (${t('settings.palette.ignored')}: ${parsed.ignored})`
          : t('settings.palette.imported'),
    });
  }

  return (
    <div className="mt-6 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface)">
      {/* 헤더: 대상 팔레트 탭 + 액션 */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-(--color-border-default) p-4">
        <div className="flex items-center gap-2">
          {PRESET_IDS.map((preset) => {
            const isTarget = preset === target;
            const overriddenCount = Object.keys(themeOverrides[preset] ?? {}).length;
            return (
              <button
                key={preset}
                type="button"
                onClick={() => setTarget(preset)}
                aria-pressed={isTarget}
                data-testid={`palette-tab-${preset}`}
                className={cn(
                  'rounded-md border px-3 py-1.5 text-sm font-medium transition-colors',
                  isTarget
                    ? 'border-(--color-interactive-primary) bg-(--color-interactive-muted) text-(--color-interactive-active)'
                    : 'border-(--color-border-default) text-(--color-text-secondary) hover:border-(--color-border-strong)',
                )}
              >
                {t(PRESET_LABEL_KEY[preset])}
                {overriddenCount > 0 && (
                  <span className="ml-1.5 text-xs opacity-70">({overriddenCount})</span>
                )}
                {preset === activePreset && (
                  <span className="ml-1.5 text-xs font-normal opacity-70">
                    · {t('settings.palette.active')}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={handleExport}
            data-testid="palette-export"
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) px-3 py-1.5 text-sm text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
          >
            <Download className="h-4 w-4" aria-hidden="true" />
            {t('settings.palette.export')}
          </button>

          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) px-3 py-1.5 text-sm text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
          >
            <Upload className="h-4 w-4" aria-hidden="true" />
            {t('settings.palette.import')}
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="application/json,.json"
            className="hidden"
            data-testid="theme-import-input"
            onChange={(e) => {
              const file = e.target.files?.[0];
              // 같은 파일을 연속으로 고를 수 있도록 값을 비운다.
              e.target.value = '';
              if (file) void handleImportFile(file);
            }}
          />

          <button
            type="button"
            onClick={() => resetThemeOverrides(target)}
            disabled={changedCount === 0}
            data-testid="palette-reset"
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) px-3 py-1.5 text-sm text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong) disabled:cursor-not-allowed disabled:opacity-40"
          >
            <RotateCcw className="h-4 w-4" aria-hidden="true" />
            {t('settings.palette.reset')}
          </button>
        </div>
      </div>

      {/* 컬러 테이블 — 카테고리를 tbody 로 묶어 하나의 표로 그린다.
          카테고리마다 표를 따로 두면 각자 폭을 계산해 컬럼이 어긋난다. */}
      <div className="flex flex-col gap-4 p-4 lg:flex-row lg:items-start">
      <div className="min-w-0 flex-1 overflow-x-auto lg:basis-1/2">
        <table className="w-full min-w-[560px] table-fixed border-collapse text-sm">
          {/* 컬럼 폭 고정 — 모든 카테고리가 같은 격자를 쓴다 */}
          <colgroup>
            <col />
            <col className="w-20" />
            <col className="w-36" />
            <col className="w-24" />
          </colgroup>

          <thead>
            <tr className="text-left text-xs text-(--color-text-muted)">
              <th className="pb-2 font-medium">{t('settings.palette.colToken')}</th>
              <th className="pb-2 font-medium">{t('settings.palette.colColor')}</th>
              <th className="pb-2 font-medium">{t('settings.palette.colValue')}</th>
              <th className="pb-2" />
            </tr>
          </thead>

          {TOKEN_CATEGORIES.map((category) => (
            <tbody key={category.category}>
              <tr>
                <th
                  colSpan={4}
                  className="border-t border-(--color-border-subtle) pt-4 pb-2 text-left text-sm font-semibold text-(--color-text-secondary)"
                >
                  {category.label}
                </th>
              </tr>

              {category.tokens.map((token) => {
                const value = resolved[token.cssVar] ?? '';
                const isOverridden = overrides[token.cssVar] !== undefined;
                return (
                  <tr
                    key={token.cssVar}
                    ref={(el) => {
                      if (el === null) rowRefs.current.delete(token.cssVar);
                      else rowRefs.current.set(token.cssVar, el);
                    }}
                    data-token={token.cssVar}
                    data-lit={activeToken === token.cssVar ? 'true' : 'false'}
                    data-selected={selectedTokens.has(token.cssVar) ? 'true' : 'false'}
                    // 누르면 고른다. 포커스도 같은 일을 한다 — 키보드로 이 행에 온
                    // 것은 마우스로 누른 것과 같은 뜻이다.
                    onClick={() => pickToken(token.cssVar)}
                    onFocus={() => pickToken(token.cssVar)}
                    // 커서는 따라가기를 켠 사람에게만 뜻이 있다.
                    onMouseEnter={() => setHovered(token.cssVar)}
                    onMouseLeave={() => setHovered(undefined)}
                    className={cn(
                      'align-middle',
                      // 고른 조각이 쓰는 색은 여럿이므로 행 여럿이 함께 표시된다.
                      selectedTokens.has(token.cssVar) && 'bg-(--color-interactive-muted)',
                      activeToken === token.cssVar && 'bg-(--color-interactive-muted)',
                    )}
                  >
                    <td className="py-1.5 pr-3">
                      <div className="truncate text-(--color-text-primary)">{token.label}</div>
                      <div className="truncate text-xs text-(--color-text-muted)">{token.hint}</div>
                    </td>

                    <td className="py-1.5 pr-3">
                      {/* 알파를 켜지 않는다 — `isValidHexColor` 가 여덟 자리를 거부해
                          `normalizeOverrides` 가 저장 자체를 버린다(감사표 27행). */}
                      <ColorPicker
                        value={value === '' ? undefined : value}
                        onChange={(c) => {
                          if (c !== undefined) setThemeToken(target, token.cssVar, c);
                        }}
                        ariaLabel={token.label}
                      />
                    </td>

                    <td className="py-1.5 pr-3">
                      <input
                        type="text"
                        value={value}
                        spellCheck={false}
                        onChange={(e) => {
                          const next = e.target.value;
                          // 입력 도중(#ab 등)에도 막지 않되, 완성된 헥스일 때만 반영한다.
                          if (/^#[0-9a-fA-F]{6}$/.test(next)) {
                            setThemeToken(target, token.cssVar, next);
                          }
                        }}
                        className="w-full rounded border border-(--color-border-default) bg-(--color-bg-sunken) px-2 py-1 font-mono text-xs text-(--color-text-primary)"
                      />
                    </td>

                    <td className="py-1.5 text-right">
                      {isOverridden && (
                        <button
                          type="button"
                          onClick={() =>
                            setThemeToken(target, token.cssVar, PRESETS[target][token.cssVar] ?? '')
                          }
                          className="text-xs text-(--color-interactive-primary) hover:underline"
                        >
                          {t('settings.palette.resetToken')}
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          ))}
        </table>
      </div>

        {/* 미리보기 — **편집 중인** 프리셋을 그린다. 표에서 고치는 색이 실제 화면에서
            어떻게 보일지는 이 자리 말고는 볼 곳이 없다: 비활성 프리셋을 고치면 바깥
            화면은 아무것도 바뀌지 않기 때문이다. */}
        <aside className="w-full min-w-0 flex-1 lg:basis-1/2 lg:sticky lg:top-4">
          <div className="mb-2 flex items-center justify-between gap-2">
            <h3 className="text-xs font-medium text-(--color-text-muted)">
              {t('settings.palette.preview')}
            </h3>
            {/* 기본은 끔. 커서만 지나가도 미리보기가 따라 움직이면 표를 훑어보는
                동안 화면이 쉬지 않고 바뀐다 — 읽으려던 사람이 멀미를 한다. */}
            <label className="flex cursor-pointer items-center gap-1.5 text-xs text-(--color-text-muted)">
              <input
                type="checkbox"
                checked={followCursor}
                data-testid="palette-follow-cursor"
                onChange={(e) => setFollowCursor(e.target.checked)}
                className="h-3.5 w-3.5 cursor-pointer accent-(--color-interactive-primary)"
              />
              {t('settings.palette.followCursor')}
            </label>
          </div>
          <ThemePreview
            target={target}
            overrides={overrides}
            activeToken={activeToken}
            selectedPart={selectedPart}
            onPickPart={pickPart}
            onClearSelection={() => {
              setSelectedPart(undefined);
              setSelectedToken(undefined);
              setHovered(undefined);
            }}
          />
        </aside>
      </div>

    </div>
  );
}
