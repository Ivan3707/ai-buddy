import { describe, expect, it } from 'vitest';
import { stageScale } from './useStageScale';

describe('stageScale', () => {
  it('на кадре макета масштаб 1', () => expect(stageScale(1920, 1080)).toBe(1));
  it('вписывает кадр по меньшей стороне', () => {
    expect(stageScale(1366, 768)).toBeCloseTo(0.711, 2);
    expect(stageScale(2560, 1080)).toBe(1);
  });
  it('ограничивает масштаб', () => {
    expect(stageScale(300, 200)).toBe(0.5);
    expect(stageScale(7680, 4320)).toBe(2.5);
  });
});
