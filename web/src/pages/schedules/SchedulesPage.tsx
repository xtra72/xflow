// SPEC-SCHEDULE-VIEW-001 M5: 스케줄 뷰 페이지 셸.
//
// 2탭 헤더(관리 / 실행 로그)를 제공한다. '관리' 탭은 에이전트별 교차-플로우 스케줄
// 관리를 완전 구현하고(ScheduleManagementTab), '실행 로그' 탭은 correlation_id 병합
// 로그 테이블을 렌더한다(ScheduleLogTab, M6). 전체 인증 사용자 접근(RD-5, AC-11/AC-17)
// — 추가 role 게이팅 없음. AgentListPage 레이아웃 관례(space-y-6, 상단 액션/헤더)를 따른다.

import { useState } from 'react';
import { CalendarClock, ScrollText } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

import ScheduleManagementTab from './ScheduleManagementTab';
import ScheduleLogTab from './ScheduleLogTab';

type ScheduleTab = 'manage' | 'logs';

const TABS: { key: ScheduleTab; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { key: 'manage', label: '관리', icon: CalendarClock },
  { key: 'logs', label: '실행 로그', icon: ScrollText },
];

export default function SchedulesPage() {
  const [tab, setTab] = useState<ScheduleTab>('manage');

  return (
    <div className="space-y-6" data-testid="schedules-page">
      {/* 헤더 */}
      <div>
        <h1 className="text-xl font-bold text-(--color-text-primary)">스케줄</h1>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          에이전트별 예약 스케줄을 관리하고 실행 로그를 확인합니다.
        </p>
      </div>

      {/* 탭 헤더 */}
      <div
        className="flex gap-1 border-b border-(--color-border-default)"
        role="tablist"
        aria-label="스케줄 뷰 탭"
      >
        {TABS.map(({ key, label, icon: Icon }) => {
          const active = tab === key;
          return (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={active}
              data-testid={`schedules-tab-${key}`}
              onClick={() => setTab(key)}
              className={cn(
                '-mb-px inline-flex items-center gap-1.5 border-b-2 px-4 py-2 text-sm font-medium transition-colors',
                active
                  ? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-400'
                  : 'border-transparent text-(--color-text-muted) hover:text-(--color-text-secondary)',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          );
        })}
      </div>

      {/* 탭 콘텐츠 */}
      {tab === 'manage' ? <ScheduleManagementTab /> : <ScheduleLogTab />}
    </div>
  );
}
