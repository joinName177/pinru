import { describe, expect, it } from 'vitest';
import { hasCustomPromptEntries } from './useQuestionBank';

describe('hasCustomPromptEntries', () => {
  it('recognizes prompt items under a spaced Bug fix heading', () => {
    expect(
      hasCustomPromptEntries(`## Bug 修复

1. 【困难】统一预览和最终创建的相似度结论。
2. 【困难】修复拖动后的步骤编号。`),
    ).toBe(true);
  });

  it('does not treat a document without a supported heading as prompt-ready', () => {
    expect(hasCustomPromptEntries('1. 【困难】这条提示词没有分类标题。')).toBe(false);
  });
});
