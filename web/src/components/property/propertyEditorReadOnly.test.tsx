// Property editor readOnly 가시성 회귀 테스트.
//
// 버그: text/number/datetime input 에 disabled 속성이 붙고 readOnlyCls 가
//       opacity-60 + bg-gray-* 조합이라 다크모드에서 값이 거의 안 보였다.
//
// 수정: text/number/datetime/textarea 등 readOnly attr 를 지원하는 input 은
//       disabled 대신 readOnly attr 를 사용하고, readOnlyCls 는
//       'cursor-not-allowed bg-(--color-bg-elevated)' 로 단순화한다.
//       select/checkbox/button 은 readOnly attr 미지원이므로 disabled 유지.
//
// 이 테스트는 9개 property editor 컴포넌트에 동일한 패턴이 적용되었는지를
// 한꺼번에 검증한다 (commit b4ad829 / 309966e 와 동일 패턴).

import { describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';

import { TriggerScheduleEditor } from './TriggerScheduleEditor';
import { BridgeHttpConfig } from './BridgeHttpConfig';
import { BridgeMqttConfig } from './BridgeMqttConfig';
import { BridgeModbusConfig } from './BridgeModbusConfig';
import { KeyValueMapEditor } from './KeyValueMapEditor';
import { RegisterMapEditor } from './RegisterMapEditor';
import { StringListEditor } from './StringListEditor';
import { TransformPipelineEditor } from './TransformPipelineEditor';
import { TypedKeyValueMapEditor } from './TypedKeyValueMapEditor';

// 텍스트성 input 이 readOnly attr 를 사용하는지 확인하는 헬퍼.
// readOnly === true 이고 disabled === false 여야 한다.
function expectTextInputsReadOnly(container: HTMLElement, types: string[]) {
  for (const type of types) {
    const inputs = container.querySelectorAll<HTMLInputElement>(`input[type="${type}"]`);
    for (const input of Array.from(inputs)) {
      expect(input.readOnly).toBe(true);
      expect(input.disabled).toBe(false);
    }
  }
}

describe('Property editors readOnly visibility (commit b4ad829 / 309966e 패턴)', () => {
  it('TriggerScheduleEditor: interval/cron/once input 은 readOnly attr 사용', () => {
    const value = [
      { type: 'interval', value: '5s' },
      { type: 'cron', value: '0 */5 * * *' },
    ];
    const { container } = render(
      <TriggerScheduleEditor value={value} onChange={vi.fn()} readOnly />,
    );
    // text input (interval, cron) 은 readOnly
    expectTextInputsReadOnly(container, ['text']);
    // select (스케줄 타입) 는 disabled 유지
    const selects = container.querySelectorAll<HTMLSelectElement>('select');
    for (const sel of Array.from(selects)) {
      expect(sel.disabled).toBe(true);
    }
  });

  it('BridgeHttpConfig: url_template (text), timeout_ms (number) 는 readOnly attr 사용', () => {
    const { container } = render(
      <BridgeHttpConfig
        data={{ content_type: 'application/json', url_template: '/api/x', timeout_ms: 5000 }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expectTextInputsReadOnly(container, ['text', 'number']);
    // select (Content-Type) 는 disabled 유지
    const selects = container.querySelectorAll<HTMLSelectElement>('select');
    expect(selects.length).toBeGreaterThan(0);
    for (const sel of Array.from(selects)) {
      expect(sel.disabled).toBe(true);
    }
  });

  it('BridgeMqttConfig: publish_topic (text) 는 readOnly attr 사용, checkbox/select 는 disabled', () => {
    const { container } = render(
      <BridgeMqttConfig
        data={{ default_qos: '1', default_retained: true, publish_topic: 'devices/x' }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expectTextInputsReadOnly(container, ['text']);
    // checkbox 는 disabled 유지
    const cb = container.querySelector<HTMLInputElement>('input[type="checkbox"]');
    expect(cb).not.toBeNull();
    expect(cb!.disabled).toBe(true);
    // select 도 disabled 유지
    const select = container.querySelector<HTMLSelectElement>('select');
    expect(select).not.toBeNull();
    expect(select!.disabled).toBe(true);
  });

  it('BridgeModbusConfig: unit_id (number) 는 readOnly attr 사용', () => {
    const { container } = render(
      <BridgeModbusConfig
        data={{ unit_id: 1, register_map: {} }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expectTextInputsReadOnly(container, ['number']);
  });

  it('KeyValueMapEditor: key/value (text) 는 readOnly attr 사용', () => {
    const { container } = render(
      <KeyValueMapEditor
        value={{ alpha: '1', beta: '2' }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expectTextInputsReadOnly(container, ['text']);
  });

  it('RegisterMapEditor: startAddress/count (number) 는 readOnly attr 사용, select 는 disabled', () => {
    const value = {
      holding_registers: [{ start_address: 0, count: 10, data_type: 'uint16' }],
    };
    const { container } = render(
      <RegisterMapEditor value={value} onChange={vi.fn()} readOnly />,
    );
    expectTextInputsReadOnly(container, ['number']);
    // select (areaType / dataType) 는 disabled
    const selects = container.querySelectorAll<HTMLSelectElement>('select');
    for (const sel of Array.from(selects)) {
      expect(sel.disabled).toBe(true);
    }
  });

  it('StringListEditor: 행 input (text) 은 readOnly attr 사용', () => {
    const { container } = render(
      <StringListEditor value={['topic/a', 'topic/b']} onChange={vi.fn()} readOnly />,
    );
    expectTextInputsReadOnly(container, ['text']);
  });

  it('TransformPipelineEditor: fieldName/fieldValue (text) 는 readOnly attr 사용, select 는 disabled', () => {
    const value = [{ select: '{ params: { area: $.area } }' }];
    const { container } = render(
      <TransformPipelineEditor value={value} onChange={vi.fn()} readOnly />,
    );
    expectTextInputsReadOnly(container, ['text']);
    // select (모드) 는 disabled
    const select = container.querySelector<HTMLSelectElement>('select');
    expect(select).not.toBeNull();
    expect(select!.disabled).toBe(true);
  });

  it('TypedKeyValueMapEditor: key (text) 는 readOnly attr 사용, select 는 disabled', () => {
    const { container } = render(
      <TypedKeyValueMapEditor
        value={{ greet: 'hello', count: 7 }}
        onChange={vi.fn()}
        readOnly
      />,
    );
    expectTextInputsReadOnly(container, ['text', 'number']);
    // select (타입) 는 disabled
    const selects = container.querySelectorAll<HTMLSelectElement>('select');
    expect(selects.length).toBeGreaterThan(0);
    for (const sel of Array.from(selects)) {
      expect(sel.disabled).toBe(true);
    }
  });
});
