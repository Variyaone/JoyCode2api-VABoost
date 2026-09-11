// Date × hour activity from retained evidence. At most 31 × 24 cells mounted.
import { useEffect, useMemo, useRef, useState } from 'react';
import type { KeyboardEvent } from 'react';
import { Button, Segmented, Table, Tooltip } from 'antd';
import { api } from '../api';
import { useRefreshableResource } from '../hooks/useRefreshableResource';
import { calendarDay, calendarTime, heatColors, heatLevel } from '../utils/usageReport';
import type { UsageReport } from '../utils/usageReport';
import { activityCellLabel, activityRows, hourLabel } from '../utils/usageActivity';
import type { ActivityCell } from '../utils/usageActivity';
import ResourceStatus from './ResourceStatus';

export default function ActivityHeatmap({ report, autoRefresh, refreshKey }: {
  report: UsageReport; autoRefresh: boolean; refreshKey: number;
}) {
  const [page, setPage] = useState(0);
  const totalDays = Math.round((calendarTime(report.end) - calendarTime(report.start)) / 86_400_000) + 1;
  const pages = Math.ceil(totalDays / 31);
  const end = calendarTime(report.end) - page * 31 * 86_400_000;
  const from = calendarDay(Math.max(calendarTime(report.start), end - 30 * 86_400_000));
  const through = calendarDay(end);
  return <section className="jc-usage-section jc-hourly-section" aria-label="日期与小时活动热力图">
    <ActivityPage key={`${from}/${through}`} from={from} through={through} timezone={report.timezone} autoRefresh={autoRefresh} refreshKey={refreshKey} />
    {pages > 1 && <div className="jc-hourly-pagination">
      <Button size="small" disabled={page >= pages - 1} onClick={() => setPage(page + 1)}>更早日期</Button>
      <span>所选 {totalDays} 天，每页最多 31 天；当前 {from} 至 {through}</span>
      <Button size="small" disabled={page === 0} onClick={() => setPage(page - 1)}>较近日期</Button>
    </div>}
  </section>;
}

function ActivityPage({ from, through, timezone, autoRefresh, refreshKey }: {
  from: string; through: string; timezone: string; autoRefresh: boolean; refreshKey: number;
}) {
  const resource = useRefreshableResource(async signal => {
    const data = await api.getUsageActivity(from, through, signal);
    if (data.from !== from || data.through !== through || data.source !== 'request_logs' || !Array.isArray(data.days)) throw new Error('小时记录范围不匹配');
    return data;
  }, { autoRefresh });
  const lastRefresh = useRef(refreshKey);
  useEffect(() => {
    if (refreshKey !== lastRefresh.current) { lastRefresh.current = refreshKey; void resource.refresh(); }
  }, [refreshKey, resource.refresh]);
  const [mode, setMode] = useState<'requests' | 'tokens'>('requests');
  const [table, setTable] = useState(false);
  const [focused, setFocused] = useState(0);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const grid = useRef<HTMLDivElement>(null);
  const rows = useMemo(() => resource.data ? activityRows(resource.data) : [], [resource.data]);
  const cells = useMemo(() => rows.flatMap(row => row.cells), [rows]);
  const max = cells.reduce((value, cell) => Math.max(value, cell[mode] ?? 0), 0);
  const unit = mode === 'requests' ? '次请求' : 'Token';
  const partial = resource.data?.days.some(day => day.coverage !== 'matched');

  function move(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === 'ArrowRight') next++;
    else if (event.key === 'ArrowLeft') next--;
    else if (event.key === 'ArrowDown') next += 24;
    else if (event.key === 'ArrowUp') next -= 24;
    else if (event.key === 'Home') next -= next % 24;
    else if (event.key === 'End') next += 23 - next % 24;
    else return;
    event.preventDefault();
    next = Math.max(0, Math.min(cells.length - 1, next));
    setFocused(next);
    grid.current?.querySelector<HTMLButtonElement>(`[data-cell-index="${next}"]`)?.focus();
  }

  return <>
    <div className="jc-usage-section-head"><div><h2>使用时段</h2><p className="jc-usage-caption">横轴为小时，纵轴为日期。每个格子是一小时的已记录使用量。</p></div>
      <div className="jc-hourly-controls"><Segmented aria-label="热力图指标" value={mode} onChange={value => setMode(value as 'requests' | 'tokens')} options={[{ value: 'requests', label: '请求次数' }, { value: 'tokens', label: 'Token' }]} /><Button type="text" size="small" onClick={() => setTable(!table)}>{table ? '显示热力图' : '查看数据表'}</Button></div>
    </div>
    <ResourceStatus label="时段记录" resource={resource} />
    <p className="jc-usage-caption">{from} 至 {through}，{resource.data?.timezone ?? timezone}。按请求完成后的日志记录时间归入小时，不代表连续在线时长。{partial ? '斜纹表示小时日志不足；不从每日总量推算小时，也不把缺失当零。' : ''}</p>
    {resource.initialLoading && <div className="jc-hourly-placeholder" role="status">正在读取日期 × 小时记录…</div>}
    {!resource.initialLoading && !resource.data && <div className="jc-hourly-placeholder">暂未读取到小时记录。其他统计仍可查看，可使用上方重试。</div>}
    {resource.data && (table ? <Table<ActivityCell> rowKey={cell => `${cell.date}/${cell.hour}`} size="small" pagination={{ pageSize: 24, showSizeChanger: false }} scroll={{ x: 560 }} dataSource={cells}
      columns={[
        { title: '日期', dataIndex: 'date' }, { title: '时段', render: (_, cell) => hourLabel(cell.hour) },
        { title: '请求次数', render: (_, cell) => cell.requests?.toLocaleString() ?? (cell.state === 'future' ? '尚未发生' : '记录不足') },
        { title: '输入 Token', render: (_, cell) => cell.input?.toLocaleString() ?? '记录不足' },
        { title: '输出 Token', render: (_, cell) => cell.output?.toLocaleString() ?? '记录不足' },
        { title: '记录范围', render: (_, cell) => cell.state === 'partial' ? '部分现存记录' : cell.state === 'unknown' ? '记录不足' : cell.state === 'future' ? '尚未发生' : '与账本记录相符' },
      ]} /> : <>
      <div className="jc-hourly-scroll" aria-label="日期与小时矩阵，方向键浏览，可横向滚动">
        <div className="jc-hourly-grid" ref={grid}>
          <span className="jc-hourly-date jc-hourly-corner">日期 / 小时</span>
          {Array.from({ length: 24 }, (_, hour) => <span className="jc-hourly-hour" key={hour}>{String(hour).padStart(2, '0')}</span>)}
          {rows.map((row, rowIndex) => <div className="jc-hourly-row" key={row.date}>
            <span className="jc-hourly-date">{row.date.slice(5)} <small>{['日','一','二','三','四','五','六'][new Date(calendarTime(row.date)).getUTCDay()]}</small></span>
            {row.cells.map((cell, hour) => {
              const index = rowIndex * 24 + hour;
              return <button type="button" key={hour} className="jc-hour-cell" data-cell-index={index} data-state={cell.state}
                aria-label={activityCellLabel(cell)} title={activityCellLabel(cell)} tabIndex={focused === index ? 0 : -1}
                onKeyDown={event => move(event, index)} onFocus={() => { setFocused(index); setSelectedIndex(index); }} onMouseEnter={() => setSelectedIndex(index)} onClick={() => setSelectedIndex(index)}
                style={cell[mode] !== null ? { backgroundColor: heatColors[heatLevel(cell[mode] ?? 0, max)] } : undefined} />;
            })}
          </div>)}
        </div>
      </div>
      <div className="jc-hourly-readout" role="status">{selectedIndex !== null && cells[selectedIndex] ? activityCellLabel(cells[selectedIndex]) : '悬停、点击或使用方向键查看具体日期与时段。'}</div>
      <div className="jc-heat-legend"><span>0</span>{heatColors.map(color => <span key={color} className="jc-heat-swatch" style={{ background: color }} />)}<span>{max.toLocaleString()} {unit}</span><Tooltip title="原始日志无法完整对应每日账本；已有数值仅是现存部分。"><span className="jc-heat-swatch jc-heat-unknown" /></Tooltip><span>记录不足</span><span className="jc-hourly-future-key" /><span>尚未发生</span></div>
    </>)}
  </>;
}
