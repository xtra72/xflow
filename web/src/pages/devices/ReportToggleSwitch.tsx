// 디바이스별 상태 전송(report_enabled) on/off 토글 스위치.
//
// Samsung HVACR / LGAP 등 디바이스별 report 게이트를 지원하는 에이전트에서 사용한다.
// 전역 디바이스 목록(DeviceListPage)과 에이전트 상세의 디바이스 탭(DevicesTab)이
// 동일한 UI/동작을 공유하도록 공용 컴포넌트로 분리했다.

import { cn } from '@/lib/utils/cn';
import type { TranslationFn } from '@/lib/i18n';

export function ReportToggleSwitch({
  enabled,
  onToggle,
  t,
}: {
  enabled: boolean;
  onToggle: (next: boolean) => void;
  t: TranslationFn;
}) {
  const label = enabled ? t('devices.list.reportOn') : t('devices.list.reportOff');
  return (
    <button
      type="button"
      role="switch"
      aria-checked={enabled}
      aria-label={t('devices.list.reportToggle')}
      title={`${t('devices.list.reportToggle')}: ${label}`}
      onClick={(e) => {
        e.stopPropagation();
        onToggle(!enabled);
      }}
      className={cn(
        'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2',
        enabled ? 'bg-green-500' : 'bg-(--color-border-strong)',
      )}
    >
      <span
        className={cn(
          'pointer-events-none inline-block h-3.5 w-3.5 rounded-full bg-(--color-bg-surface) shadow-sm ring-0 transition-transform duration-200',
          enabled ? 'translate-x-4' : 'translate-x-0.5',
        )}
      />
    </button>
  );
}

export default ReportToggleSwitch;
