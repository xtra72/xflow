// 에이전트 타입 상세 패널.
// 카드 아래에 확장되어 설정 필드, 설정 예제를 표시한다.

import { useTranslation } from '@/lib/i18n';

import { AGENT_TYPE_META } from './agentTypeMeta';

interface AgentTypeDetailPanelProps {
  agentType: string;
}

/** 에이전트 타입 상세 정보 패널 */
export default function AgentTypeDetailPanel({ agentType }: AgentTypeDetailPanelProps) {
  const { t } = useTranslation();
  const meta = AGENT_TYPE_META[agentType];

  if (!meta) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        {t('agents.types.noDetail')}
      </div>
    );
  }

  return (
    <div className="space-y-5 rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-5">
      {/* 기능 설명 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          {t('agents.types.featureSection')}
        </h4>
        <p className="text-sm text-(--color-text-muted) leading-relaxed">
          {meta.description}
        </p>
      </section>

      {/* 설정 필드 */}
      {meta.configFields.length > 0 ? (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            {t('agents.types.configFields')}
          </h4>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('agents.types.colName')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('agents.types.colType')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('agents.types.colRequired')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('agents.types.colDefault')}
                  </th>
                  <th className="py-2 text-left font-medium text-(--color-text-muted)">
                    {t('agents.types.colDescription')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {meta.configFields.map((field) => (
                  <tr key={field.name}>
                    <td className="py-2 pr-4 font-mono text-(--color-text-primary)">
                      {field.name}
                    </td>
                    <td className="py-2 pr-4 text-(--color-text-muted)">
                      {field.type}
                    </td>
                    <td className="py-2 pr-4">
                      {field.required ? (
                        <span className="text-red-600 dark:text-red-400 font-medium">Y</span>
                      ) : (
                        <span className="text-gray-400">-</span>
                      )}
                    </td>
                    <td className="py-2 pr-4 font-mono text-(--color-text-muted)">
                      {field.default ?? '-'}
                    </td>
                    <td className="py-2 text-(--color-text-muted)">
                      {field.description}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            {t('agents.types.configFields')}
          </h4>
          <p className="text-sm text-(--color-text-muted)">
            {t('agents.types.configFieldsEmpty')}
          </p>
        </section>
      )}

      {/* 설정 예제 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          {t('agents.types.configExample')}
        </h4>
        <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950">
          {JSON.stringify(meta.configExample, null, 2)}
        </pre>
      </section>
    </div>
  );
}
