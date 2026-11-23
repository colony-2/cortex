import { Button, DatePicker, Input, InputNumber, Space } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import type { ColumnType } from 'antd/es/table';
import type { FilterDropdownProps } from 'antd/es/table/interface';

// Keep old date-only URLs as inclusive, whole-day UTC ranges.
function dateBounds(filter: string) {
  if (filter.includes('/')) return filter.split('/');
  return [filter + 'T00:00', filter + 'T23:59'];
}

function pickerRange(filter?: React.Key): [Dayjs, Dayjs] | null {
  if (!filter) return null;
  const [from, through] = dateBounds(String(filter)).map((value) => dayjs(value));
  return from.isValid() && through?.isValid() ? [from, through] : null;
}

function inputFilter(label: string, type: 'text' | 'number' | 'date' | 'range') {
  return ({ selectedKeys, setSelectedKeys, confirm, clearFilters }: FilterDropdownProps) => (
    <Space direction="vertical" style={{ padding: 8 }} onKeyDown={(event) => event.stopPropagation()}>
      {type === 'range' ? (
        <DatePicker.RangePicker
          aria-label={label}
          placeholder={['From (UTC)', 'Through (UTC)']}
          format="YYYY-MM-DD HH:mm"
          showTime={{ format: 'HH:mm', defaultOpenValue: [dayjs().startOf('day'), dayjs().endOf('day')] }}
          needConfirm={false}
          getPopupContainer={(trigger) => trigger.parentElement || document.body}
          value={pickerRange(selectedKeys[0])}
          onChange={(dates) => {
            const [from, through] = dates || [];
            setSelectedKeys(
              from && through
                ? [from.format('YYYY-MM-DDTHH:mm') + '/' + through.format('YYYY-MM-DDTHH:mm')]
                : [],
            );
          }}
        />
      ) : type === 'date' ? (
        <DatePicker
          getPopupContainer={(trigger) => trigger.parentElement || document.body}
          aria-label={label}
          placeholder={label}
          value={selectedKeys[0] ? dayjs(String(selectedKeys[0])) : null}
          onChange={(date) => setSelectedKeys(date ? [date.format('YYYY-MM-DD')] : [])}
        />
      ) : type === 'number' ? (
        <InputNumber
          autoFocus
          aria-label={label}
          placeholder={label}
          min={0}
          style={{ width: 170 }}
          value={selectedKeys[0] !== undefined ? Number(selectedKeys[0]) : null}
          onChange={(number) => setSelectedKeys(number == null ? [] : [String(number)])}
          onPressEnter={() => confirm()}
        />
      ) : (
        <Input
          autoFocus
          aria-label={label}
          placeholder={label}
          value={String(selectedKeys[0] ?? '')}
          onChange={(event) => setSelectedKeys(event.target.value ? [event.target.value] : [])}
          onPressEnter={() => confirm()}
        />
      )}
      <Space>
        <Button size="small" type="primary" onClick={() => confirm()}>
          Apply
        </Button>
        <Button size="small" onClick={() => clearFilters?.({ confirm: true, closeDropdown: true })}>
          Reset
        </Button>
      </Space>
    </Space>
  );
}

export function textColumn<T>(
  title: string,
  key: string,
  value: (row: T) => string | undefined,
): ColumnType<T> {
  return {
    title,
    key,
    render: (_, row) => value(row) || '—',
    sorter: (a, b) => (value(a) || '').localeCompare(value(b) || ''),
    filterDropdown: inputFilter(title, 'text'),
    onFilter: (term, row) => !!value(row)?.toLowerCase().includes(String(term).toLowerCase()),
  };
}

export function dateColumn<T>(
  title: string,
  key: string,
  value: (row: T) => string | undefined,
  filter: 'date' | 'range' = 'date',
): ColumnType<T> {
  const time = (row: T) => {
    const raw = value(row);
    return raw && !raw.startsWith('0001-') ? Date.parse(raw) : NaN;
  };
  return {
    title,
    key,
    sorter: (a, b) => (time(a) || 0) - (time(b) || 0),
    filterDropdown: inputFilter(title + (filter === 'range' ? ' (UTC range)' : ' (UTC date)'), filter),
    onFilter: (selection, row) => {
      const timestamp = time(row);
      if (!Number.isFinite(timestamp)) return false;
      const iso = new Date(timestamp).toISOString();
      if (filter === 'date') return iso.startsWith(String(selection));
      const [from, through] = dateBounds(String(selection));
      const minute = iso.slice(0, 16);
      return minute >= from && minute <= through;
    },
    render: (_, row) =>
      Number.isFinite(time(row)) ? (
        <time dateTime={value(row)} title={value(row)}>
          {new Date(time(row)).toLocaleString()}
        </time>
      ) : (
        '—'
      ),
  };
}

export function numberColumn<T>(
  title: string,
  key: string,
  filterLabel: string,
  value: (row: T) => number | undefined,
): ColumnType<T> {
  return {
    title,
    key,
    sorter: (a, b) => (value(a) ?? -1) - (value(b) ?? -1),
    filterDropdown: inputFilter(filterLabel, 'number'),
    onFilter: (minimum, row) => (value(row) ?? -1) >= Number(minimum),
    render: (_, row) => value(row) ?? '—',
  };
}
