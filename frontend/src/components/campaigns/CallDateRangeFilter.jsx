import { useRef, useState } from 'react';
import { DatePicker } from 'antd';
import enUS from 'antd/locale/en_US';
import dayjs from 'dayjs';
import customParseFormat from 'dayjs/plugin/customParseFormat';

dayjs.extend(customParseFormat);
const FORMAT = 'DD/MM/YYYY, hh:mm A';
const endLocale = { ...enUS, lang: { ...enUS.lang, ok: 'Apply' } };
const readRange = (from, to) => [from ? dayjs(from) : null, to ? dayjs(to) : null];

function RangeEditor({ from, to, onChange }) {
  const [draft, setDraft] = useState(() => readRange(from, to));
  const [openField, setOpenField] = useState(null);
  const [error, setError] = useState('');
  const endPicker = useRef(null);

  const update = (index, value) => {
    setError('');
    setDraft(current => current.map((date, position) => position === index ? value : date));
  };
  const confirmStart = date => {
    if (!date?.isValid()) return;
    if (date.isAfter(dayjs())) { setError('Start date cannot be in the future.'); return; }
    setError('');
    setDraft(current => [date, current[1]?.isBefore(date) ? null : current[1]]);
    setOpenField('end');
    requestAnimationFrame(() => endPicker.current?.focus());
  };
  const applyEnd = date => {
    if (!draft[0]?.isValid() || !date?.isValid()) return;
    const end = date.isAfter(dayjs()) ? dayjs() : date;
    if (draft[0].isAfter(end)) { setError('End date must be on or after the start date.'); return; }
    setError('');
    setOpenField(null);
    onChange(draft[0].format('YYYY-MM-DDTHH:mm:ss'), end.format('YYYY-MM-DDTHH:mm:ss'));
    endPicker.current?.blur();
  };
  const confirmWithEnter = (event, preventDefault, confirm) => {
    if (event.key !== 'Enter') return;
    preventDefault?.();
    event.preventDefault();
    event.stopPropagation();
    const date = dayjs(event.target.value, FORMAT, true);
    if (date.isValid()) confirm(date);
  };
  const onOpenChange = field => open => {
    setOpenField(current => open ? field : current === field ? null : current);
  };
  const common = {
    format: FORMAT,
    allowClear: false,
    needConfirm: true,
    previewValue: false,
    maxDate: dayjs().endOf('day'),
    className: 'agent-report-range-picker',
    classNames: { popup: { root: 'agent-report-range-popup' } },
  };

  return <div className="call-date-range-filter" role="group" aria-label="Filter by call date and time">
    <DatePicker
      {...common}
      value={draft[0]}
      open={openField === 'start'}
      onOpenChange={onOpenChange('start')}
      placeholder="dd/mm/yyyy, --:-- --"
      aria-label="Calls made from"
      showTime={{ format: 'hh:mm A', use12Hours: true, defaultOpenValue: dayjs().startOf('day') }}
      onCalendarChange={value => update(0, value)}
      onChange={value => update(0, value)}
      onOk={confirmStart}
      onKeyDown={(event, preventDefault) => confirmWithEnter(event, preventDefault, confirmStart)}
    />
    <span className="call-date-range-separator">to</span>
    <DatePicker
      {...common}
      ref={endPicker}
      value={draft[1]}
      open={openField === 'end'}
      onOpenChange={onOpenChange('end')}
      placeholder="dd/mm/yyyy, --:-- --"
      aria-label="Calls made to"
      locale={endLocale}
      minDate={draft[0]?.startOf('day')}
      showTime={{ format: 'hh:mm A', use12Hours: true, defaultOpenValue: dayjs().endOf('day') }}
      onCalendarChange={value => update(1, value)}
      onChange={value => update(1, value)}
      onOk={applyEnd}
      onKeyDown={(event, preventDefault) => confirmWithEnter(event, preventDefault, applyEnd)}
    />
    <button type="button" className="btn-secondary" disabled={!from && !to && !draft[0] && !draft[1]} onClick={() => {
      setDraft([null, null]); setOpenField(null); setError(''); onChange('', '');
    }}>Clear</button>
    {error && <span role="alert" style={{ color: 'var(--transcript-red, #ef4444)', fontSize: 12 }}>{error}</span>}
  </div>;
}

export default function CallDateRangeFilter(props) {
  return <RangeEditor key={`${props.from}|${props.to}`} {...props} />;
}
