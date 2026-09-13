import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { TableStatusBadge } from './TableStatusBadge';

describe('TableStatusBadge',()=>{
  it('shows a running review without claiming completion',()=>{
    render(<TableStatusBadge progress={{state:'running',prepared:1,reviewed:1,total:3,message:'第 2 轮'}} />);
    expect(screen.getByText('制表中 · 已复审 1/3 轮')).toBeInTheDocument();
    expect(screen.queryByText('已制表')).not.toBeInTheDocument();
  });
  it('keeps partial results visible after failure',()=>{
    render(<TableStatusBadge progress={{state:'error',prepared:1,reviewed:2,total:3,error:'执行超时'}} />);
    expect(screen.getByText('制表失败 · 已复审 2/3 轮')).toHaveAttribute('title',expect.stringContaining('可导出 1 轮'));
  });
  it('does not label missing evidence as ready even when every round was reviewed',()=>{
    render(<TableStatusBadge progress={{state:'needs_evidence',prepared:0,reviewed:1,total:1}} />);
    expect(screen.getByText('待补证据 · 已复审 1/1 轮')).toBeInTheDocument();
  });
});
