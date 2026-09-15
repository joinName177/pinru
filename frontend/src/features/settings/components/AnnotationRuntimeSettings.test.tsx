import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { AnnotationRuntimeSettings } from './AnnotationRuntimeSettings';

const config = vi.hoisted(() => ({
  getAnnotationSettings: vi.fn(),
  saveAnnotationSettings: vi.fn(),
}));
vi.mock('../../../api/config', () => config);

it('loads and saves the global review engine and container API key', async () => {
  config.getAnnotationSettings.mockResolvedValue({ reviewEngine: 'codex', hasContainerApiKey: true });
  config.saveAnnotationSettings.mockResolvedValue(undefined);
  render(<AnnotationRuntimeSettings />);

  expect(await screen.findByDisplayValue('Codex CLI')).toBeInTheDocument();
  expect(screen.getByText('容器 API Key 已保存')).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('审核引擎'), { target: { value: 'deepseek' } });
  fireEvent.change(screen.getByLabelText('容器 API Key'), { target: { value: 'new-container-key' } });
  fireEvent.click(screen.getByRole('button', { name: '保存审核与容器设置' }));

  await waitFor(() => expect(config.saveAnnotationSettings).toHaveBeenCalledWith('deepseek', 'new-container-key'));
  expect(await screen.findByRole('status')).toHaveTextContent('已保存');
});
