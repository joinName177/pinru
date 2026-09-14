import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CustomProjectPickerModal } from './QuestionBankSyncCards';

function setup() {
  const onImport = vi.fn();
  render(<CustomProjectPickerModal open importing={false} promptDocGenerating={false} error="" promptDocError="" onClose={vi.fn()} onImport={onImport} scanResult={{ projectId: 'batch', projectName: 'Batch', rootPath: '/projects', prefixes: 'cyc', totalCount: 1, skippedCount: 0, candidates: [{ name: 'cyc-05', path: '/projects/cyc-05', questionId: 5, targetPath: '/target' }] }} />);
  fireEvent.click(screen.getByRole('checkbox'));
  return onImport;
}

describe('custom document quantities', () => {
  it('submits editable quantities while keeping four other types locked', () => {
    const onImport = setup();
    fireEvent.change(screen.getByRole('spinbutton', { name: '0-1代码生成' }), { target: { value: '0' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Feature迭代' }), { target: { value: '3' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Bug修复' }), { target: { value: '2' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: '一般' }), { target: { value: '4' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: '困难' }), { target: { value: '5' } });
    for (const name of ['代码理解', '工程化', '代码测试', '代码重构']) {
      expect(screen.getByRole('spinbutton', { name })).toBeDisabled();
      expect(screen.getByRole('spinbutton', { name })).toHaveValue(1);
    }
    fireEvent.click(screen.getByRole('button', { name: '导入并生成文档' }));
    expect(onImport).toHaveBeenCalledWith(['cyc-05'], { codeGen: 0, feature: 3, bugFix: 2, general: 4, difficult: 5 });
  });

  it('blocks empty, fractional and negative quantities', () => {
    const onImport = setup();
    for (const value of ['', '-1', '1.5']) {
      fireEvent.change(screen.getByRole('spinbutton', { name: 'Bug修复' }), { target: { value } });
      expect(screen.getByRole('button', { name: '导入并生成文档' })).toBeDisabled();
    }
    expect(onImport).not.toHaveBeenCalled();
  });

  it('requires difficulty quantities to equal the generated task total', () => {
    setup();
    fireEvent.change(screen.getByRole('spinbutton', { name: '困难' }), { target: { value: '9' } });
    expect(screen.getByText(/难度数量合计 19 题，与题型总数 20 题不一致/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '导入并生成文档' })).toBeDisabled();
  });
});
