// 시스템 리소스(CPU, 메모리) 사용량 위젯.
// 모니터 메트릭 데이터를 받아 미니 차트와 수치로 표시한다.

import { Cpu, HardDrive } from 'lucide-react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';

import { formatPercent } from '@/lib/utils/format';

interface ResourceWidgetProps {
  metrics: Record<string, unknown> | undefined;
}

/**
 * 메트릭 데이터에서 숫자 값을 안전하게 추출한다.
 * 값이 없거나 숫자가 아니면 null을 반환한다.
 */
function extractNumber(value: unknown): number | null {
  if (typeof value === 'number' && !Number.isNaN(value)) return value;
  if (typeof value === 'string') {
    const parsed = parseFloat(value);
    return Number.isNaN(parsed) ? null : parsed;
  }
  return null;
}

/**
 * 단일 리소스 게이지 표시 컴포넌트.
 * 현재 사용률을 큰 숫자와 미니 면적 차트로 보여준다.
 */
function ResourceGauge({
  icon,
  label,
  value,
  color,
}: {
  icon: React.ReactNode;
  label: string;
  value: number | null;
  color: string;
}) {
  // 미니 차트용 더미 히스토리 데이터 (실제 히스토리가 없으므로 현재 값 기반으로 생성)
  const chartData = value !== null
    ? [
        { v: Math.max(0, value - 0.08) },
        { v: Math.max(0, value - 0.03) },
        { v: value },
        { v: Math.max(0, value - 0.02) },
        { v: value },
      ]
    : [];

  return (
    <div className="flex flex-col items-center gap-2 rounded-md border border-gray-200 p-4 dark:border-gray-700">
      <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400">
        {icon}
        <span className="text-sm font-medium">{label}</span>
      </div>

      {value !== null ? (
        <>
          <span className="text-3xl font-bold text-gray-900 dark:text-white">
            {formatPercent(value)}
          </span>
          {/* 미니 면적 차트 */}
          <div className="h-10 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <Area
                  type="monotone"
                  dataKey="v"
                  stroke={color}
                  fill={color}
                  fillOpacity={0.2}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </>
      ) : (
        <span className="text-sm text-gray-400 dark:text-gray-500">
          데이터 없음
        </span>
      )}
    </div>
  );
}

/** 시스템 리소스 개요를 표시하는 대시보드 위젯 */
export default function ResourceWidget({ metrics }: ResourceWidgetProps) {
  // 메트릭에서 CPU, 메모리 사용률 추출
  // 서버 메트릭 키 형식에 맞춰 여러 경로를 시도한다
  const cpuValue = metrics
    ? extractNumber(metrics['cpu_usage_percent'] ?? metrics['cpu_usage'] ?? metrics['cpu'])
    : null;
  const memValue = metrics
    ? extractNumber(metrics['memory_usage_percent'] ?? metrics['memory_usage'] ?? metrics['memory'])
    : null;

  // 서버가 0~100 범위로 보내는 경우 0~1 비율로 정규화
  const cpuNormalized = cpuValue !== null ? (cpuValue > 1 ? cpuValue / 100 : cpuValue) : null;
  const memNormalized = memValue !== null ? (memValue > 1 ? memValue / 100 : memValue) : null;

  return (
    <div className="rounded-lg bg-white p-6 shadow dark:bg-gray-800">
      <h3 className="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
        시스템 리소스
      </h3>
      <div className="grid grid-cols-2 gap-4">
        <ResourceGauge
          icon={<Cpu className="h-4 w-4" />}
          label="CPU"
          value={cpuNormalized}
          color="#3b82f6"
        />
        <ResourceGauge
          icon={<HardDrive className="h-4 w-4" />}
          label="메모리"
          value={memNormalized}
          color="#8b5cf6"
        />
      </div>
    </div>
  );
}
