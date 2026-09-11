import { describe, expect, it } from 'vitest';
import { fmt, fmtLatency, percentage } from './dashboard';

describe('dashboard number formatting', () => {
  it.each([
    [0, '0'], [12, '12'], [999, '999'], [1000, '1.0K'], [1250, '1.3K'],
    [999_000, '999.0K'], [1_000_000, '1.00M'], [1_234_567, '1.23M'],
    [1_000_000_000, '1.00B'], [2_244_680_000, '2.24B'],
  ])('formats %s as %s', (value, expected) => {
    expect(fmt(value)).toBe(expected);
  });
});

describe('dashboard latency formatting', () => {
  it.each([
    [0, '0ms'], [123.4, '123ms'], [123.5, '124ms'], [999, '999ms'],
    [1000, '1s'], [1999, '1s'], [59_999, '59s'], [60_000, '1m'],
    [61_000, '1m 1s'], [119_999, '1m 59s'], [120_000, '2m'], [3_661_000, '61m 1s'],
  ])('formats %s ms as %s', (value, expected) => {
    expect(fmtLatency(value)).toBe(expected);
  });
});

describe('dashboard percentages', () => {
  it('does not claim a success rate when there are no requests', () => {
    expect(percentage(0, 0)).toBe('—');
  });

  it.each([
    [0, 100, '0%'], [100, 100, '100%'], [1, 3, '33.33%'], [2, 3, '66.67%'],
    [1, 8, '12.5%'], [95, 100, '95%'], [120, 125, '96%'],
    [1, 10_000, '0.01%'], [9999, 10_000, '99.99%'],
  ])('formats %s of %s as %s without unnecessary zeroes', (count, total, expected) => {
    expect(percentage(count, total)).toBe(expected);
  });

  it('does not round rare failures to perfect success or rare successes to zero', () => {
    expect(percentage(1, 1_000_000)).toBe('<0.01%');
    expect(percentage(999_999, 1_000_000)).toBe('>99.99%');
  });
});
