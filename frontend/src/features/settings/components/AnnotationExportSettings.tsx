import { useEffect, useState } from 'react';
import { getConfig, setConfig } from '../../../api/config';

export function AnnotationExportSettings() {
  const [directory, setDirectory] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  useEffect(() => {
    let current = true;
    getConfig('annotation_export_directory').then((value) => { if (current) setDirectory(value); })
      .catch((error) => { if (current) setMessage(String(error)); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, []);
  async function save() {
    const value = directory.trim();
    if (value && !value.startsWith('/')) { setMessage('请填写本机绝对路径，例如 /Users/你的用户名/Documents/PinRu导出'); return; }
    setSaving(true);
    try {
      await setConfig('annotation_export_directory', value);
      setMessage('已保存，所有项目和单题导出统一使用此目录');
    } catch (error) { setMessage(String(error)); }
    finally { setSaving(false); }
  }
  return <section className="mb-6 rounded-2xl border border-stone-200 bg-white p-5 dark:border-stone-800 dark:bg-stone-900">
    <h2 className="text-base font-semibold text-stone-800 dark:text-stone-100">Excel 统一导出位置</h2>
    <p className="mt-2 text-sm text-stone-500">所有项目的一键导出、单题导出及批次导出共用此目录。每次生成独立文件夹，保存 Excel 和轨迹附件。</p>
    <label className="mt-4 block text-sm text-stone-600 dark:text-stone-300">导出目录（本机绝对路径）
      <input className="mt-2 w-full rounded-xl border border-stone-200 bg-transparent px-3 py-2 dark:border-stone-700" value={directory} disabled={loading || saving} onChange={(event) => setDirectory(event.target.value)} placeholder="留空使用默认目录：~/.pinru/annotation/exports" />
    </label>
    <button className="mt-3 rounded-xl bg-slate-800 px-4 py-2 text-sm text-white disabled:opacity-40 dark:bg-slate-200 dark:text-slate-900" disabled={loading || saving} onClick={() => void save()}>{saving ? '保存中…' : '保存导出位置'}</button>
    {message && <p role="status" className="mt-2 text-sm text-stone-500">{message}</p>}
  </section>;
}
