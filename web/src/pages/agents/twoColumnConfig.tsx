// 2열 설정 레이아웃 (HVACR 외 에이전트) — 공유 모듈.
//
// AgentDetailPanel(설정 탭)과 CreateAgentModal(생성 모달)이 함께 사용한다.
// HVACR 에이전트(lg_hvacr01 / century_hvacr01)는 상세 패널에서
// FourQuadrantConfigLayout 으로 분기된다. samsung_hvacr01 은 생성 팝업에서
// 연결|운영 2분할을 쓰도록 아래 TWO_COL_CONFIG 에 포함되지만, 상세 패널은
// HVACR_QUADRANT_AGENT_TYPES 체크가 우선하므로 여전히 4-분면을 사용한다.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { FormField } from '@/components/property/FormField';
import type { ConfigSchema } from '@/types/node';
import { isFieldVisible } from '@/types/node';
import { TWO_COL_CONFIG } from './twoColumnConfigMap';

export function TwoColumnConfigLayout({
  data, schema, onChange, readOnly, agentType, logLevel,
}: {
  nodeId: string;
  data: Record<string, unknown>;
  schema: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
  agentType: string;
  logLevel?: { agentId: string; value: string; updating: boolean; onChangeLevel: (v: string) => void };
}) {
  const { t } = useTranslation();
  const colConfig = TWO_COL_CONFIG[agentType];
  if (!colConfig) return null;

  // 로그 섹션(section: 'logging') 필드가 있으면 별도 '로그' 그룹으로 분리하고 로그 레벨
  // 셀렉터를 그 그룹 상단에 둔다(HVACR 4-분면의 logging 섹션과 동형). 로그 섹션 필드가
  // 없는 에이전트는 기존 2열 동작을 유지한다(로그 레벨은 우측 컬럼 하단).
  const leftFields = schema.fields.filter((f) => colConfig.left.has(f.name) && f.section !== 'logging');
  const rightFields = schema.fields.filter((f) => !colConfig.left.has(f.name) && f.section !== 'logging');

  const filterVisible = (fields: typeof schema.fields) =>
    fields.filter((f) => isFieldVisible(f, data as Record<string, unknown>));

  const loggingFields = filterVisible(schema.fields.filter((f) => f.section === 'logging'));
  const hasLoggingGroup = loggingFields.length > 0;

  const handleChange = (fieldName: string, value: unknown) => {
    onChange({ ...data, [fieldName]: value });
  };

  // 로그 레벨 셀렉터(로컬 타깃에서만 logLevel 전달). 로그 그룹 유무에 따라 그룹 상단 또는
  // 우측 컬럼 하단에 배치한다.
  const logLevelSelect = logLevel ? (
    <div className="space-y-1">
      <label
        htmlFor={`agent-log-${logLevel.agentId}`}
        className="block text-xs font-medium text-(--color-text-secondary)"
      >
        {t('agents.detail.config.logLevel')}
      </label>
      <select
        id={`agent-log-${logLevel.agentId}`}
        value={logLevel.value}
        onChange={(e) => logLevel.onChangeLevel(e.target.value)}
        disabled={logLevel.updating}
        className={cn(
          'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
          'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
          'bg-(--color-bg-surface) text-(--color-text-primary)',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        <option value="">{t('agents.detail.config.logLevelDefault')}</option>
        <option value="debug">DEBUG</option>
        <option value="info">INFO</option>
        <option value="warn">WARN</option>
        <option value="error">ERROR</option>
      </select>
    </div>
  ) : null;

  return (
    <div className="grid grid-cols-2 gap-4">
      {/* 좌측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{t(colConfig.leftLabelKey)}</h4>
        {filterVisible(leftFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
            formData={data}
          />
        ))}
      </div>
      {/* 우측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{t(colConfig.rightLabelKey)}</h4>
        {filterVisible(rightFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
            formData={data}
          />
        ))}
        {/* 로그 그룹 (section: 'logging' 필드 보유 에이전트: serial / tcp-server 등) 은
            우측 컬럼 하단에 별도 '로그' 그룹으로 렌더한다. 로그 레벨을 상단에, 그 아래
            로그 토글(송/수신 프레임 로그 등)을 둔다. 로그 섹션 필드가 없으면 기존처럼
            로그 레벨만 우측 하단에 렌더한다. */}
        {hasLoggingGroup ? (
          <div className="space-y-3 border-t border-(--color-border-default) pt-3">
            <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{t('agents.detail.config.logging')}</h4>
            {logLevelSelect}
            {loggingFields.map((field) => (
              <FormField
                key={field.name}
                field={field}
                value={data[field.name]}
                onChange={(v) => handleChange(field.name, v)}
                readOnly={readOnly}
                formData={data}
              />
            ))}
          </div>
        ) : (
          logLevelSelect
        )}
      </div>
    </div>
  );
}
