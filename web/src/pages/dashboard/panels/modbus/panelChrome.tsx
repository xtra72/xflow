// SPEC-MODBUS-012 M1/M2: MODBUS Gateway 패널 공용 프레임/안내 UI.
//
// 6종 패널이 동일한 카드 프레임과 미설정/원격/빈/에러 안내 상태를 공유한다.
// 기존 대시보드 패널(SingleDevicePanel/AgentPanel)의 스타일 토큰을 그대로 재사용한다.

import type { ReactNode } from 'react';
import { Server } from 'lucide-react';

/** 패널 카드 프레임 + 헤더. */
export function ModbusPanelFrame({
  title,
  icon,
  children,
}: {
  title: string;
  icon?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="mb-3 flex shrink-0 items-center gap-2">
        {icon ?? <Server className="h-4 w-4 shrink-0 text-(--color-text-muted)" />}
        <h3 className="truncate text-sm font-semibold text-(--color-text-primary)">{title}</h3>
      </div>
      {children}
    </div>
  );
}

/** 중앙 정렬 안내 문구(미설정/원격/빈/에러 상태 공용). */
export function ModbusNotice({ icon, message }: { icon?: ReactNode; message: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 py-6 text-center">
      {icon}
      <p className="text-xs text-(--color-text-muted)">{message}</p>
    </div>
  );
}

/** 로딩 스피너(기존 패널 스피너 토큰 재사용). */
export function ModbusSpinner() {
  return (
    <div className="flex flex-1 items-center justify-center py-6">
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
    </div>
  );
}
