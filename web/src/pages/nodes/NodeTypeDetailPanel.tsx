// 노드 타입 상세 패널.
// 카드 아래에 확장되어 포트, 설정 필드, 설정 예제, 인스턴스 목록을 표시한다.

import { ArrowDownToLine, ArrowUpFromLine, AlertTriangle, Activity } from 'lucide-react';
import { Link } from 'react-router';

import { cn } from '@/lib/utils/cn';
import { useNodeTypeInstances } from '@/hooks/useNodeTypeInstances';

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

/** 포트 항목 렌더링 */
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
      <span className="text-xs text-(--color-text-muted)">
        - {port.description}
      </span>
    </div>
  );
}

/** 노드 타입 상세 정보 패널 */
export default function NodeTypeDetailPanel({ nodeType }: NodeTypeDetailPanelProps) {
  const meta = NODE_TYPE_META[nodeType];
  const { instances, isLoading: instancesLoading } = useNodeTypeInstances(nodeType);

  if (!meta) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        상세 정보가 없습니다.
      </div>
    );
  }

  return (
    <div className="space-y-5 rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-5">
      {/* 기능 설명 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          기능
        </h4>
        <p className="text-sm text-(--color-text-muted) leading-relaxed">
          {meta.description}
        </p>
      </section>

      {/* 포트 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          포트
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
            설정 필드
          </h4>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    이름
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    타입
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    필수
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    기본값
                  </th>
                  <th className="py-2 text-left font-medium text-(--color-text-muted)">
                    설명
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
      )}

      {/* 설정 예제 */}
      <section>
        <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
          설정 예제
        </h4>
        <pre className="overflow-x-auto rounded-md bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950">
          {JSON.stringify(meta.configExample, null, 2)}
        </pre>
      </section>

      {/* 입력 메시지 예제 */}
      {meta.inputExamples && (
        <section>
          <h4 className="text-sm font-semibold text-(--color-text-primary) mb-2">
            입력 메시지 형식
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
            출력 메시지 형식
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
          현재 인스턴스
        </h4>
        {instancesLoading ? (
          <div className="flex items-center gap-2 py-2">
            <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
            <span className="text-sm text-(--color-text-muted)">로딩 중...</span>
          </div>
        ) : instances.length === 0 ? (
          <p className="text-sm text-(--color-text-muted)">
            실행 중인 플로우에서 사용 중인 인스턴스가 없습니다.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    플로우
                  </th>
                  <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">
                    노드
                  </th>
                  <th className="py-2 pr-4 text-right font-medium text-(--color-text-muted)">
                    처리
                  </th>
                  <th className="py-2 text-right font-medium text-(--color-text-muted)">
                    에러
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
