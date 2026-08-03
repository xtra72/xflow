// 디바이스 추가 다이얼로그.
// Samsung HVACR-01, Modbus 에이전트에 대한 디바이스 추가 폼을 제공한다.

import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Trash2, X } from 'lucide-react';

import { useAgents, useExecAgent } from '@/hooks/useAgent';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';

/** 레지스터 영역 라벨 */
const MODBUS_AREA_LABELS: Record<string, string> = {
  coils: 'Coils (FC01/05)',
  discrete_inputs: 'Discrete Inputs (FC02)',
  holding_registers: 'Holding Registers (FC03/06)',
  input_registers: 'Input Registers (FC04)',
};
const MODBUS_AREA_ORDER = ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'] as const;

type RegBlock = { start: string; count: string };

const DEFAULT_REG_AREAS: Record<string, RegBlock[]> = {
  holding_registers: [{ start: '0', count: '100' }],
  input_registers: [],
  coils: [],
  discrete_inputs: [],
};

export default function AddDeviceDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const { data: agentsData } = useAgents();
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  // 디바이스 추가 지원 에이전트 필터링 (samsung_hvacr01).
  // modbus-gateway 의 디바이스는 이제 config.devices(디바이스 탭 + PUT /agents/{id}/config)로
  // 관리한다(SPEC-MODBUS-008). 전역 add_device exec 경로에서는 제외한다.
  const supportedAgents = useMemo(() => {
    const agents = agentsData?.data ?? [];
    return agents.filter((a) => a.type === 'samsung_hvacr01');
  }, [agentsData]);

  const [selectedAgentId, setSelectedAgentId] = useState('');

  // 선택된 에이전트 타입 결정
  const selectedAgentType = useMemo(() => {
    if (!selectedAgentId) return null;
    return supportedAgents.find((a) => a.id === selectedAgentId)?.type ?? null;
  }, [selectedAgentId, supportedAgents]);

  // --- Samsung HVACR-01 폼 상태 ---
  const [samsungHvacr01Address, setSamsungHvacr01Address] = useState('');
  const [samsungHvacr01DeviceId, setSamsungHvacr01DeviceId] = useState('');
  const [samsungHvacr01DeviceType, setSamsungHvacr01DeviceType] = useState('');
  // 상태 전송(report) on/off. 기본 on(true). off 면 이 디바이스는 상태 메시지를 보내지 않는다.
  const [samsungHvacr01ReportEnabled, setSamsungHvacr01ReportEnabled] = useState(true);

  // --- Modbus 폼 상태 ---
  const [modbusUnitId, setModbusUnitId] = useState('');
  const [modbusName, setModbusName] = useState('');
  const [modbusRegAreas, setModbusRegAreas] = useState<Record<string, RegBlock[]>>({ ...DEFAULT_REG_AREAS });
  const [expandedAreas, setExpandedAreas] = useState<Record<string, boolean>>({});

  function resetForm() {
    setSamsungHvacr01Address('');
    setSamsungHvacr01DeviceId('');
    setSamsungHvacr01DeviceType('');
    setSamsungHvacr01ReportEnabled(true);
    setModbusUnitId('');
    setModbusName('');
    setModbusRegAreas({
      holding_registers: [{ start: '0', count: '100' }],
      input_registers: [],
      coils: [],
      discrete_inputs: [],
    });
    setExpandedAreas({});
  }

  function handleSubmit() {
    if (!selectedAgentId) return;

    if (selectedAgentType === 'samsung_hvacr01') {
      if (!samsungHvacr01Address.trim()) return;
      execAgent.mutate(
        {
          id: selectedAgentId,
          req: {
            command: 'add_device',
            params: {
              address: samsungHvacr01Address.trim(),
              ...(samsungHvacr01DeviceId.trim() && { device_id: samsungHvacr01DeviceId.trim() }),
              ...(samsungHvacr01DeviceType && { device_type: samsungHvacr01DeviceType }),
              report_enabled: samsungHvacr01ReportEnabled,
            },
          },
        },
        {
          onSuccess: () => {
            addNotification({ type: 'success', message: t('devices.add.successAdded') });
            onClose();
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `${t('devices.add.addFailedPrefix')}${err instanceof Error ? err.message : t('devices.add.unknownError')}` });
          },
        },
      );
    } else if (selectedAgentType === 'modbus-gateway') {
      const unitId = parseInt(modbusUnitId, 10);
      if (isNaN(unitId) || unitId < 1 || unitId > 247) {
        addNotification({ type: 'error', message: t('devices.add.unitIdRange') });
        return;
      }

      const params: Record<string, unknown> = { unit_id: unitId };
      if (modbusName.trim()) params.name = modbusName.trim();

      // 레지스터 맵 조립 (다중 블록 지원)
      const regMap: Record<string, unknown> = {};
      for (const [area, blocks] of Object.entries(modbusRegAreas)) {
        if (!blocks || blocks.length === 0) continue;
        const parsed: { start_address: number; count: number }[] = [];
        for (const blk of blocks) {
          const start = parseInt(blk.start, 10);
          const cnt = parseInt(blk.count, 10);
          if (isNaN(start) || isNaN(cnt) || cnt <= 0) {
            addNotification({ type: 'error', message: t('devices.add.invalidAddressCount').replace('{area}', MODBUS_AREA_LABELS[area] ?? area) });
            return;
          }
          parsed.push({ start_address: start, count: cnt });
        }
        regMap[area] = parsed.length === 1 ? parsed[0] : parsed;
      }
      if (Object.keys(regMap).length === 0) {
        addNotification({ type: 'error', message: t('devices.add.atLeastOneArea') });
        return;
      }
      params.register_map = regMap;

      execAgent.mutate(
        { id: selectedAgentId, req: { command: 'add_device', params } },
        {
          onSuccess: (res) => {
            const result = res as { result?: { success?: boolean; error?: string } };
            if (result?.result?.success === false) {
              addNotification({ type: 'error', message: result.result.error ?? t('devices.add.addFailed') });
            } else {
              addNotification({ type: 'success', message: t('devices.add.modbusSuccess').replace('{unit}', String(unitId)) });
              onClose();
            }
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `${t('devices.add.addFailedPrefix')}${err instanceof Error ? err.message : t('devices.add.unknownError')}` });
          },
        },
      );
    }
  }

  // 제출 버튼 활성화 조건
  const canSubmit = (() => {
    if (!selectedAgentId || execAgent.isPending) return false;
    if (selectedAgentType === 'samsung_hvacr01') return !!samsungHvacr01Address.trim();
    if (selectedAgentType === 'modbus-gateway') return !!modbusUnitId.trim();
    return false;
  })();

  return (
    <>
      {/* 오버레이 */}
      <div
        className="fixed inset-0 z-40 bg-black/30 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* 다이얼로그 */}
      <div className="fixed inset-x-0 top-1/2 z-50 mx-auto w-full max-w-lg -translate-y-1/2 rounded-xl border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-2xl">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-(--color-text-primary)">{t('devices.add.title')}</h3>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
          {/* 에이전트 선택 */}
          <div>
            <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
              {t('devices.add.agent')}
            </label>
            <select
              value={selectedAgentId}
              onChange={(e) => {
                setSelectedAgentId(e.target.value);
                resetForm();
              }}
              className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
            >
              <option value="">{t('devices.add.agentPlaceholder')}</option>
              {supportedAgents.map((a) => {
                const typeLabel = a.type === 'modbus-gateway' ? 'Modbus' : 'Samsung HVACR-01';
                return (
                  <option key={a.id} value={a.id}>
                    {a.name} [{typeLabel}] ({a.status === 'running' ? t('devices.add.agentRunning') : t('devices.add.agentStopped')})
                  </option>
                );
              })}
            </select>
            {supportedAgents.length === 0 && (
              <p className="mt-1 text-xs text-gray-500">{t('devices.add.noSupportedAgent')}</p>
            )}
          </div>

          {/* ===== Samsung HVACR-01 폼 ===== */}
          {selectedAgentType === 'samsung_hvacr01' && (
            <>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  {t('devices.add.deviceAddress')}
                </label>
                <input
                  type="text"
                  placeholder={t('devices.add.deviceAddressPlaceholder')}
                  value={samsungHvacr01Address}
                  onChange={(e) => setSamsungHvacr01Address(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  {t('devices.add.deviceIdOptional')}
                </label>
                <input
                  type="text"
                  placeholder={t('devices.add.deviceIdPlaceholder')}
                  value={samsungHvacr01DeviceId}
                  onChange={(e) => setSamsungHvacr01DeviceId(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                />
              </div>
              <div>
                <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                  {t('devices.add.deviceType')}
                </label>
                <select
                  value={samsungHvacr01DeviceType}
                  onChange={(e) => setSamsungHvacr01DeviceType(e.target.value)}
                  className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                >
                  <option value="">{t('devices.add.autoDetect')}</option>
                  <option value="HVACR.IDU">{t('devices.add.indoor')}</option>
                  <option value="HVACR.ODU">{t('devices.add.outdoor')}</option>
                </select>
              </div>
              {/* 상태 전송 on/off. off 면 이 디바이스는 노드로 상태 메시지를 보내지 않는다. */}
              <div className="flex items-center justify-between rounded-md border border-(--color-border-default) px-3 py-2">
                <div className="mr-3 min-w-0">
                  <p className="text-sm font-medium text-(--color-text-secondary)">
                    {t('devices.add.reportEnabled')}
                  </p>
                  <p className="text-xs text-(--color-text-muted)">
                    {t('devices.add.reportEnabledDesc')}
                  </p>
                </div>
                <button
                  type="button"
                  role="switch"
                  aria-checked={samsungHvacr01ReportEnabled}
                  aria-label={t('devices.add.reportEnabled')}
                  onClick={() => setSamsungHvacr01ReportEnabled((v) => !v)}
                  className={cn(
                    'relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2',
                    samsungHvacr01ReportEnabled ? 'bg-green-500' : 'bg-gray-300 dark:bg-gray-600',
                  )}
                >
                  <span
                    className={cn(
                      'pointer-events-none inline-block h-4 w-4 rounded-full bg-white shadow-sm ring-0 transition-transform duration-200',
                      samsungHvacr01ReportEnabled ? 'translate-x-5' : 'translate-x-0.5',
                    )}
                  />
                </button>
              </div>
              <p className="text-xs text-(--color-text-muted)">
                {t('devices.add.dynamicNotice')}
              </p>
            </>
          )}

          {/* ===== Modbus 폼 ===== */}
          {selectedAgentType === 'modbus-gateway' && (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                    {t('devices.add.unitId')}
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={247}
                    placeholder="1"
                    value={modbusUnitId}
                    onChange={(e) => setModbusUnitId(e.target.value)}
                    className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
                <div>
                  <label className="mb-1 block text-sm font-medium text-(--color-text-secondary)">
                    {t('devices.add.nameOptional')}
                  </label>
                  <input
                    type="text"
                    placeholder="Device-1"
                    value={modbusName}
                    onChange={(e) => setModbusName(e.target.value)}
                    className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                  />
                </div>
              </div>

              {/* 레지스터 맵 설정 */}
              <div>
                <label className="mb-2 block text-sm font-medium text-(--color-text-secondary)">
                  {t('devices.add.registerMap')}
                </label>
                <div className="space-y-2">
                  {MODBUS_AREA_ORDER.map((area) => {
                    const blocks = modbusRegAreas[area] ?? [];
                    const enabled = blocks.length > 0;
                    const expanded = expandedAreas[area] ?? false;

                    return (
                      <div key={area} className="rounded-md border border-(--color-border-default)">
                        {/* 영역 헤더 */}
                        <div className="flex items-center gap-2 px-3 py-2">
                          <input
                            type="checkbox"
                            checked={enabled}
                            onChange={(e) => {
                              const checked = e.target.checked;
                              setModbusRegAreas((prev) => ({
                                ...prev,
                                [area]: checked
                                  ? [{ start: '0', count: area.startsWith('coil') || area.startsWith('discrete') ? '8' : '100' }]
                                  : [],
                              }));
                              if (checked) setExpandedAreas((prev) => ({ ...prev, [area]: true }));
                            }}
                            className="h-3.5 w-3.5 rounded border-gray-300"
                          />
                          <button
                            type="button"
                            onClick={() => enabled && setExpandedAreas((prev) => ({ ...prev, [area]: !prev[area] }))}
                            className="flex flex-1 items-center gap-1 text-left text-xs font-medium text-(--color-text-secondary)"
                            disabled={!enabled}
                          >
                            {enabled && (expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />)}
                            {MODBUS_AREA_LABELS[area]}
                            {enabled && <span className="ml-auto text-[10px] text-gray-400">{blocks.length}{t('devices.add.blocksUnit')}</span>}
                          </button>
                        </div>

                        {/* 블록 목록 */}
                        {enabled && expanded && (
                          <div className="border-t border-(--color-border-default) px-3 pb-2 pt-1">
                            {blocks.map((blk, idx) => (
                              <div key={idx} className="mt-1 flex items-center gap-2">
                                <span className="w-6 text-right text-[10px] text-gray-400">#{idx + 1}</span>
                                <input
                                  type="number"
                                  min={0}
                                  placeholder={t('devices.add.startPlaceholder')}
                                  value={blk.start}
                                  onChange={(e) => {
                                    const v = e.target.value;
                                    setModbusRegAreas((prev) => {
                                      const arr = [...(prev[area] ?? [])];
                                      arr[idx] = { start: v, count: arr[idx]?.count ?? '0' };
                                      return { ...prev, [area]: arr };
                                    });
                                  }}
                                  className="w-20 rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                                />
                                <span className="text-[10px] text-gray-400">~</span>
                                <input
                                  type="number"
                                  min={1}
                                  placeholder={t('devices.add.countPlaceholder')}
                                  value={blk.count}
                                  onChange={(e) => {
                                    const v = e.target.value;
                                    setModbusRegAreas((prev) => {
                                      const arr = [...(prev[area] ?? [])];
                                      arr[idx] = { start: arr[idx]?.start ?? '0', count: v };
                                      return { ...prev, [area]: arr };
                                    });
                                  }}
                                  className="w-20 rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-white"
                                />
                                <span className="text-[10px] text-gray-400">{t('devices.add.countUnit')}</span>
                                {blocks.length > 1 && (
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setModbusRegAreas((prev) => {
                                        const arr = (prev[area] ?? []).filter((_, i) => i !== idx);
                                        return { ...prev, [area]: arr };
                                      });
                                    }}
                                    className="rounded p-0.5 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                                  >
                                    <Trash2 className="h-3 w-3" />
                                  </button>
                                )}
                              </div>
                            ))}
                            <button
                              type="button"
                              onClick={() => {
                                const defCount = area.startsWith('coil') || area.startsWith('discrete') ? '8' : '100';
                                const lastBlk = blocks[blocks.length - 1];
                                const nextStart = lastBlk ? String(parseInt(lastBlk.start, 10) + parseInt(lastBlk.count, 10)) : '0';
                                setModbusRegAreas((prev) => ({
                                  ...prev,
                                  [area]: [...(prev[area] ?? []), { start: nextStart, count: defCount }],
                                }));
                              }}
                              className="mt-1 text-[10px] font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400"
                            >
                              {t('devices.add.addBlock')}
                            </button>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>

              <p className="text-xs text-(--color-text-muted)">
                {t('devices.add.dynamicNotice')}
              </p>
            </>
          )}
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-(--color-text-secondary) hover:bg-gray-50 dark:border-gray-600 dark:hover:bg-gray-700"
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
          >
            {execAgent.isPending ? t('devices.add.submitting') : t('devices.add.submit')}
          </button>
        </div>
      </div>
    </>
  );
}
