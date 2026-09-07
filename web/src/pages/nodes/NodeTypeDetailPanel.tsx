// 노드 타입 상세 패널.
// 카드 아래에 확장되어 포트, 설정 필드, 설정 예제, 인스턴스 목록을 표시한다.

import { ArrowDownToLine, ArrowUpFromLine, AlertTriangle, Activity } from 'lucide-react';
import { Link } from 'react-router';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { useNodeTypeInstances } from '@/hooks/useNodeTypeInstances';
import { FieldHelp } from '@/components/property/FieldHelp';

import { NODE_TYPE_META } from './nodeTypeMeta';
import type { PortMeta } from './nodeTypeMeta';

interface NodeTypeDetailPanelProps {
  nodeType: string;
}

/** 포트 방향별 아이콘과 색상 */
const PORT_STYLES: Record<string, { icon: React.ReactNode; color: string }> = {
  input: {
    icon: <ArrowDownToLine className="h-3.5 w-3.5" />,
    color: 'text-blue-600 dark:text-blue-400',
  },
  output: {
    icon: <ArrowUpFromLine className="h-3.5 w-3.5" />,
    color: 'text-green-600 dark:text-green-400',
  },
  error: {
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
    color: 'text-red-600 dark:text-red-400',
  },
};

/** 포트 항목 렌더링. 설명은 포트 타이틀 뒤 `?` 도움말로 표시한다(인라인 텍스트 대신). */
function PortItem({ port }: { port: PortMeta }) {
  const style = PORT_STYLES[port.direction] ?? PORT_STYLES.output!;
  return (
    <div className="flex items-center gap-2 py-1">
      <span className={style!.color}>{style!.icon}</span>
      <span className="text-sm font-mono font-medium text-(--color-text-primary)">
        {port.name}
      </span>
      <span className="text-xs text-(--color-text-muted)">
        ({port.direction})
      </span>
      {port.description && (
        <FieldHelp
          text={port.description}
          describedById={`nodetype-port-desc-${port.direction}-${port.name}`}
        />
      )}
    </div>
  );
}

/** 노드 타입 상세 정보 패널 */
export default function NodeTypeDetailPanel({ nodeType }: NodeTypeDetailPanelProps) {
  const { t } = useTranslation();
  const meta = NODE_TYPE_META[nodeType];
  const { instances, isLoading: instancesLoading } = useNodeTypeInstances(nodeType);

  if (!meta) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        {t('nodes.detail.noMeta')}
      </div>
    );
  }

  return (
    <div className="space-y-5 rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-5">
      {/* 기능 설명 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          {t('nodes.detail.function')}
        </h4>
        <p className="text-sm text-(--color-text-muted) leading-relaxed">
          {meta.description}
        </p>
      </section>

      {/* 포트 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          {t('nodes.detail.ports')}
        </h4>
        <div className="space-y-0.5">
          {meta.ports.map((port) => (
            <PortItem key={`${port.direction}-${port.name}`} port={port} />
          ))}
        </div>
      </section>

      {/* 설정 필드 */}
      {meta.configFields.length > 0 && (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            {t('nodes.detail.configFields')}
          </h4>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colName')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colType')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colRequired')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colDefault')}
                  </th>
                  <th className="py-2 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colDescription')}
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
                        <span className="text-(--color-text-muted)">-</span>
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
      )}

      {/* 설정 예제 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          {t('nodes.detail.configExample')}
        </h4>
        <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950">
          {JSON.stringify(meta.configExample, null, 2)}
        </pre>
      </section>

      {/* 입력 메시지 예제 */}
      {meta.inputExamples && (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            {t('nodes.detail.inputFormat')}
          </h4>
          {Object.entries(meta.inputExamples).map(([label, example]) => (
            <div key={label} className="mb-3">
              <p className="mb-1 text-xs font-medium text-(--color-text-secondary)">
                {label}
              </p>
              <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950">
                {JSON.stringify(example, null, 2)}
              </pre>
            </div>
          ))}
        </section>
      )}

      {/* 출력 메시지 예제 */}
      {meta.outputExamples && (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            {t('nodes.detail.outputFormat')}
          </h4>
          {Object.entries(meta.outputExamples).map(([portName, example]) => (
            <div key={portName} className="mb-3">
              <p className="mb-1 text-xs font-medium text-(--color-text-secondary)">
                {portName}
              </p>
              <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950">
                {JSON.stringify(example, null, 2)}
              </pre>
            </div>
          ))}
        </section>
      )}

      {/* 인스턴스 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2 flex items-center gap-2">
          <Activity className="h-4 w-4" />
          {t('nodes.detail.instances')}
        </h4>
        {instancesLoading ? (
          <div className="flex items-center gap-2 py-2">
            <div className="h-4 w-4 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
            <span className="text-sm text-(--color-text-muted)">{t('common.loading')}</span>
          </div>
        ) : instances.length === 0 ? (
          <p className="text-sm text-(--color-text-muted)">
            {t('nodes.detail.noInstances')}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colFlow')}
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colNode')}
                  </th>
                  <th className="py-2 pr-4 text-right font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colProcessed')}
                  </th>
                  <th className="py-2 text-right font-medium text-(--color-text-muted)">
                    {t('nodes.detail.colErrors')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {instances.map((inst) => (
                  <tr key={`${inst.flowId}-${inst.nodeId}`}>
                    <td className="py-2 pr-4">
                      <Link
                        to={`/editor/${inst.flowId}`}
                        className="text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                      >
                        {inst.flowName}
                      </Link>
                    </td>
                    <td className="py-2 pr-4 font-mono text-(--color-text-primary)">
                      {inst.nodeName}
                    </td>
                    <td className="py-2 pr-4 text-right text-(--color-text-secondary)">
                      {inst.processed.toLocaleString()}
                    </td>
                    <td className={cn(
                      'py-2 text-right',
                      inst.errors > 0
                        ? 'text-red-600 dark:text-red-400 font-medium'
                        : 'text-(--color-text-secondary)',
                    )}>
                      {inst.errors.toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
