// 노드 디스플레이 해상도 오버라이드 섹션 (SPEC-REMOTE-001 M12).
//
// 개요(overview) 탭에 임베드되어 선택 노드의 EFFECTIVE 해상도와 그 출처를 보여주고,
// 관리자가 노드 config 편집/재시작 없이 오버라이드를 SET/CLEAR 할 수 있게 한다.
//
// 책임:
//   - EFFECTIVE 해상도(W×H) + 출처 표시: "오버라이드" / "노드 보고" / "기본값".
//   - 프리셋 드롭다운(+직접 입력) 또는 W×H number 입력으로 오버라이드 SET
//     (useSetNodeDisplay) → 양의 정수 검증 → 성공/실패 토스트(editError 매핑).
//   - 오버라이드가 설정되어 있으면 "오버라이드 해제"(useClearNodeDisplay).
//
// 게이팅: 부모(NodeOverview)가 server 모드(enabled)에서만 렌더하며, 본 컴포넌트는
// 추가로 admin 일 때만 SET/CLEAR 컨트롤을 노출한다(읽기 표시는 항상). 실제 권한은
// 백엔드가 검증한다.
//
// 캔버스 연동: SET/CLEAR 성공 시 훅이 노드-상세 쿼리(['remote','nodes',id,'detail'])를
// 무효화하므로, 같은 캐시를 공유하는 고정 캔버스(DashboardCanvas)가 새 EFFECTIVE
// 해상도로 재렌더된다 — 캔버스 코드 변경 없음(REQ-M03).

import { useEffect, useMemo, useState } from 'react';
import { Loader2, Monitor } from 'lucide-react';

import { useClearNodeDisplay, useSetNodeDisplay } from '@/hooks/useRemote';
import { useAuth } from '@/hooks/useAuth';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { useUIStore } from '@/stores/uiStore';
import type { NodeDetail } from '@/types/remote';

// 미보고/오버라이드 없음일 때의 폴백 해상도(REQ-M03, NodeDashboard 와 동일 값).
const FALLBACK_DISPLAY_WIDTH = 1920;
const FALLBACK_DISPLAY_HEIGHT = 1080;

// 직접 입력 프리셋 식별자(드롭다운에서 선택 시 number 입력을 노출).
const CUSTOM_PRESET = 'custom';

/** EFFECTIVE 해상도의 출처. */
type ResolutionSource = 'override' | 'reported' | 'fallback';

/** 흔히 쓰는 키오스크/터치스크린 프리셋 (REQ-M12). value 는 "WxH" 토큰. */
const PRESETS: ReadonlyArray<{ value: string; width: number; height: number }> = [
  { value: '2560x1440', width: 2560, height: 1440 },
  { value: '1920x1080', width: 1920, height: 1080 },
  { value: '1280x1024', width: 1280, height: 1024 },
  { value: '1280x720', width: 1280, height: 720 },
  { value: '1024x768', width: 1024, height: 768 },
  { value: '800x480', width: 800, height: 480 },
];

/**
 * 입력 문자열을 양의 정수로 파싱한다. 정수가 아니거나(소수/공백/문자/음수/0)
 * 양수가 아니면 null 을 반환한다(parseInt 의 절삭을 피해 엄격히 검증).
 */
function parsePositiveInt(raw: string): number | null {
  const trimmed = raw.trim();
  // 1 이상의 정수만 허용(선행 0 포함 숫자열). 소수점/부호/지수 표기는 거부한다.
  if (!/^\d+$/.test(trimmed)) return null;
  const value = Number(trimmed);
  return Number.isInteger(value) && value > 0 ? value : null;
}

/** detail 로부터 EFFECTIVE 해상도의 출처를 계산한다. */
function resolveSource(detail: NodeDetail): ResolutionSource {
  if (detail.display_override_width > 0 && detail.display_override_height > 0) {
    return 'override';
  }
  if (detail.display_reported_width > 0 && detail.display_reported_height > 0) {
    return 'reported';
  }
  return 'fallback';
}

interface NodeDisplayResolutionSectionProps {
  /** 대상 노드 식별자. */
  instanceId: string;
  /** 노드 상세(EFFECTIVE + 오버라이드 + 보고 해상도 포함). */
  detail: NodeDetail;
}

/**
 * 디스플레이 해상도 섹션 — EFFECTIVE 표시 + 출처 + (admin) 오버라이드 SET/CLEAR.
 */
export function NodeDisplayResolutionSection({
  instanceId,
  detail,
}: NodeDisplayResolutionSectionProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const { user, authEnabled } = useAuth();
  // admin 판단: authEnabled=false(dev 단일 사용자) → admin 간주, 그 외 role 검증.
  // UI 노출 제어용이며 실제 권한은 백엔드가 검증한다(SystemStatusPage 와 동일 패턴).
  const isAdmin = !authEnabled || user?.role === 'admin';

  const setDisplay = useSetNodeDisplay();
  const clearDisplay = useClearNodeDisplay();

  const source = resolveSource(detail);
  const hasOverride = source === 'override';

  // EFFECTIVE 표시값(폴백 출처면 폴백 크기를 보여 캔버스와 일치시킨다).
  const effectiveWidth =
    detail.display_width > 0 ? detail.display_width : FALLBACK_DISPLAY_WIDTH;
  const effectiveHeight =
    detail.display_height > 0 ? detail.display_height : FALLBACK_DISPLAY_HEIGHT;

  // 프리셋 선택값. 현재 EFFECTIVE 가 프리셋과 일치하면 그 값으로 초기화한다.
  const matchingPreset = useMemo(
    () =>
      PRESETS.find(
        (p) => p.width === effectiveWidth && p.height === effectiveHeight,
      ),
    [effectiveWidth, effectiveHeight],
  );

  const [preset, setPreset] = useState<string>(
    matchingPreset ? matchingPreset.value : CUSTOM_PRESET,
  );
  const [widthInput, setWidthInput] = useState<string>(String(effectiveWidth));
  const [heightInput, setHeightInput] = useState<string>(String(effectiveHeight));
  const [validationError, setValidationError] = useState<string | null>(null);

  // 노드/EFFECTIVE 가 바뀌면(다른 노드 선택, 외부 갱신) 입력 폼을 동기화한다.
  useEffect(() => {
    setPreset(matchingPreset ? matchingPreset.value : CUSTOM_PRESET);
    setWidthInput(String(effectiveWidth));
    setHeightInput(String(effectiveHeight));
    setValidationError(null);
  }, [matchingPreset, effectiveWidth, effectiveHeight]);

  const pending = setDisplay.isPending || clearDisplay.isPending;
  const showCustomInputs = preset === CUSTOM_PRESET;

  const handlePresetChange = (value: string): void => {
    setValidationError(null);
    setPreset(value);
    if (value !== CUSTOM_PRESET) {
      const p = PRESETS.find((x) => x.value === value);
      if (p) {
        setWidthInput(String(p.width));
        setHeightInput(String(p.height));
      }
    }
  };

  const handleSave = (): void => {
    setValidationError(null);
    const width = parsePositiveInt(widthInput);
    const height = parsePositiveInt(heightInput);
    if (width === null || height === null) {
      setValidationError(t('remote.display.errorInvalid'));
      return;
    }
    setDisplay.mutate(
      { instanceID: instanceId, width, height },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: t('remote.display.toast.set') });
        },
        onError: (err) => {
          addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
        },
      },
    );
  };

  const handleClear = (): void => {
    setValidationError(null);
    clearDisplay.mutate(instanceId, {
      onSuccess: () => {
        addNotification({ type: 'success', message: t('remote.display.toast.cleared') });
      },
      onError: (err) => {
        addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
      },
    });
  };

  return (
    <section
      aria-labelledby="node-display-heading"
      data-testid="node-display-section"
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
    >
      <div className="mb-3 flex items-center gap-2">
        <Monitor className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
        <h2
          id="node-display-heading"
          className="text-sm font-semibold text-(--color-text-primary)"
        >
          {t('remote.display.title')}
        </h2>
      </div>

      {/* EFFECTIVE 해상도 + 출처 */}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-2.5 sm:grid-cols-3">
        <div>
          <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.display.effective')}
          </dt>
          <dd
            data-testid="display-effective"
            className="mt-1 text-sm font-medium text-(--color-text-primary)"
          >
            {effectiveWidth} × {effectiveHeight}
          </dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.display.sourceLabel')}
          </dt>
          <dd
            data-testid="display-source"
            data-source={source}
            className="mt-1 text-sm text-(--color-text-secondary)"
          >
            {t(`remote.display.source.${source}`)}
          </dd>
        </div>
      </dl>

      {/* admin 컨트롤: 오버라이드 SET (프리셋/직접 입력) + CLEAR */}
      {isAdmin && (
        <div
          className="mt-4 border-t border-(--color-border-default) pt-4"
          data-testid="display-controls"
        >
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-40">
              <label
                htmlFor="display-preset"
                className="block text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
              >
                {t('remote.display.presetLabel')}
              </label>
              <select
                id="display-preset"
                value={preset}
                onChange={(e) => handlePresetChange(e.target.value)}
                disabled={pending}
                data-testid="display-preset"
                className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
              >
                {PRESETS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.width} × {p.height}
                  </option>
                ))}
                <option value={CUSTOM_PRESET}>{t('remote.display.presetCustom')}</option>
              </select>
            </div>

            {showCustomInputs && (
              <>
                <div className="w-28">
                  <label
                    htmlFor="display-width"
                    className="block text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                  >
                    {t('remote.display.width')}
                  </label>
                  <input
                    id="display-width"
                    type="number"
                    min={1}
                    step={1}
                    value={widthInput}
                    onChange={(e) => setWidthInput(e.target.value)}
                    disabled={pending}
                    data-testid="display-width-input"
                    className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
                  />
                </div>
                <span className="pb-2 text-sm text-(--color-text-muted)" aria-hidden="true">
                  ×
                </span>
                <div className="w-28">
                  <label
                    htmlFor="display-height"
                    className="block text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                  >
                    {t('remote.display.height')}
                  </label>
                  <input
                    id="display-height"
                    type="number"
                    min={1}
                    step={1}
                    value={heightInput}
                    onChange={(e) => setHeightInput(e.target.value)}
                    disabled={pending}
                    data-testid="display-height-input"
                    className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
                  />
                </div>
              </>
            )}

            <button
              type="button"
              onClick={handleSave}
              disabled={pending}
              data-testid="display-save-button"
              className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {setDisplay.isPending && (
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
              )}
              {t('remote.display.save')}
            </button>

            {hasOverride && (
              <button
                type="button"
                onClick={handleClear}
                disabled={pending}
                data-testid="display-clear-button"
                className="inline-flex items-center gap-2 rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
              >
                {clearDisplay.isPending && (
                  <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
                )}
                {t('remote.display.clear')}
              </button>
            )}
          </div>

          {validationError && (
            <p
              className="mt-2 text-sm text-red-600 dark:text-red-400"
              data-testid="display-validation-error"
              role="alert"
            >
              {validationError}
            </p>
          )}

          <p className="mt-2 text-xs text-(--color-text-muted)">
            {t('remote.display.hint')}
          </p>
        </div>
      )}
    </section>
  );
}
