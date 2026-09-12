import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { it, expect, vi } from 'vitest';
import { AnnotationExportSettings } from './AnnotationExportSettings';
const config = vi.hoisted(() => ({ getConfig: vi.fn(), setConfig: vi.fn() }));
vi.mock('../../../api/config', () => config);

it('loads and saves the single shared export directory and rejects relative paths', async () => {
  config.getConfig.mockResolvedValue('/exports/old');
  config.setConfig.mockResolvedValue(undefined);
  render(<AnnotationExportSettings />);
  const field = await screen.findByDisplayValue('/exports/old');
  fireEvent.change(field, { target: { value: '/exports/new' } });
  fireEvent.click(screen.getByRole('button', { name: '保存导出位置' }));
  await waitFor(() => expect(config.setConfig).toHaveBeenCalledWith('annotation_export_directory', '/exports/new'));
  await screen.findByText('已保存，所有项目和单题导出统一使用此目录');
  config.setConfig.mockClear();
  fireEvent.change(field, { target: { value: 'relative/path' } });
  fireEvent.click(screen.getByRole('button', { name: '保存导出位置' }));
  expect(config.setConfig).not.toHaveBeenCalled();
  expect(screen.getByRole('status')).toHaveTextContent('请填写本机绝对路径');
});
