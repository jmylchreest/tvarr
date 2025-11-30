'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState, memo } from 'react';

/**
 * ManualChannelEditor
 *
 * A responsive, validation-aware editor for defining manual stream channels
 * for a stream source with source_type === 'manual'.
 *
 * Design goals:
 * - All rows are always active (per current product decision).
 * - Require at least one valid row before allowing parent form submission.
 * - Minimize horizontal sprawl: primary fields always visible, advanced fields behind an expand toggle.
 * - Immediate validation feedback (no debounce needed at current scale).
 * - Duplicate channel_number detection.
 *
 * Validation rules (active rows only — all rows are active in this iteration):
 * - name: non-empty (trimmed)
 * - stream_url: must start with http:// or https://
 * - tvg_logo: empty OR starts with @logo: OR http(s)://
 * - channel_number: optional; if present must be unique among non-empty
 *
 * Parent integration:
 *  <ManualChannelEditor
 *     value={manualChannels}
 *     onChange={setManualChannels}
 *     onValidityChange={setManualChannelsValid}
 *  />
 *
 * Parent should disable submit when !manualChannelsValid.
 */

/* duplicate React import removed */
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

export interface ManualChannelInput {
  channel_number?: number;
  name: string;
  stream_url: string;
  group_title?: string;
  tvg_id?: string;
  tvg_name?: string;
  tvg_logo?: string;
  epg_id?: string;
  // is_active intentionally omitted/ignored (all rows active by policy)
}

interface RowUI extends ManualChannelInput {
  id: string; // stable identity for React key & focus retention
  _expanded?: boolean;
  _errors?: Partial<Record<'name' | 'stream_url' | 'tvg_logo' | 'channel_number', string>>;
  _focusVersion?: number; // increment when forcing re-focus
}

export interface ManualChannelEditorProps {
  value: ManualChannelInput[];
  onChange: (rows: ManualChannelInput[]) => void;
  onValidityChange?: (valid: boolean) => void;
  disabled?: boolean;
  minRequired?: number; // default 1
  className?: string;
}

export const ManualChannelEditor: React.FC<ManualChannelEditorProps> = ({
  value,
  onChange,
  onValidityChange,
  disabled = false,
  minRequired = 1,
  className,
}) => {
  const [rows, setRows] = useState<RowUI[]>(
    value.length
      ? value.map((r) => ({
          ...r,
          id: crypto.randomUUID(),
          _expanded: false,
          _errors: {},
        }))
      : [
          {
            id: crypto.randomUUID(),
            name: '',
            stream_url: '',
            _expanded: true,
            _errors: {},
          },
        ]
  );

  // Keep internal rows synced if parent replaces the value wholesale
  useEffect(() => {
    // Basic shallow comparison to avoid resetting user expansion states unnecessarily
    if (value.length === 0 && rows.length === 1 && !rows[0].name && !rows[0].stream_url) return;
    if (value.length !== rows.length) {
      setRows(
        value.map((r, i) => ({
          ...r,
          id: (r as any).id ?? crypto.randomUUID(),
          _expanded: rows[i]?._expanded ?? false,
          _errors: rows[i]?._errors ?? {},
        }))
      );
    }
  }, [value]); // eslint-disable-line react-hooks/exhaustive-deps

  // Detect duplicates for channel_number
  const duplicateMap = useMemo(() => {
    const counts = new Map<number, number>();
    rows.forEach((r) => {
      if (r.channel_number != null) {
        counts.set(r.channel_number, (counts.get(r.channel_number) ?? 0) + 1);
      }
    });
    return counts;
  }, [rows]);

  // Lightweight validators to avoid reconstructing every row object each keystroke.
  // We validate only the changed row inline; global validity recalculated from current state.
  const validateSingle = (row: RowUI, duplicateMapLocal: Map<number, number>): RowUI => {
    const errors: RowUI['_errors'] = {};

    if (!row.name.trim()) {
      errors.name = 'Name required';
    }
    if (!/^https?:\/\//.test(row.stream_url)) {
      errors.stream_url = 'Must start http(s)://';
    }
    if (
      row.tvg_logo &&
      !(
        row.tvg_logo.startsWith('@logo:') ||
        /^https?:\/\//.test(row.tvg_logo) ||
        row.tvg_logo.trim() === ''
      )
    ) {
      errors.tvg_logo = 'Invalid (@logo:token or URL)';
    }
    if (
      row.channel_number != null &&
      duplicateMapLocal.get(row.channel_number) !== undefined &&
      duplicateMapLocal.get(row.channel_number)! > 1
    ) {
      errors.channel_number = 'Duplicate number';
    }
    // Mutate errors only; preserve object reference to keep cursor position stable.
    row._errors = errors;
    return row;
  };

  const recomputeAllValidity = useCallback(
    (next: RowUI[]) => {
      // Rebuild duplicate map once
      const dup = new Map<number, number>();
      next.forEach((r) => {
        if (r.channel_number != null) {
          dup.set(r.channel_number, (dup.get(r.channel_number) ?? 0) + 1);
        }
      });
      // Validate each row in place (no new objects)
      next.forEach((r) => validateSingle(r, dup));
      const allValid = next.every((r) => !r._errors || Object.keys(r._errors!).length === 0);
      const nonEmpty = next.length >= minRequired;
      onValidityChange?.(allValid && nonEmpty);
    },
    [minRequired, onValidityChange]
  );

  // Initial validity computation
  useEffect(() => {
    recomputeAllValidity(rows);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Parent updates deferred until explicit Apply Click to prevent focus loss.
  // We only recompute validity locally on each keystroke.
  const emitImmediateToParent = (nextRows: RowUI[]) => {
    onChange(nextRows.map(({ _expanded, _errors, id, ...rest }) => rest));
  };

  const updateRow = (index: number, patch: Partial<RowUI>) => {
    setRows((prev) => {
      const next = [...prev];
      Object.assign(next[index], patch); // preserve ref for focus
      // Revalidate only this row & global validity (no parent emit here)
      const dup = new Map<number, number>();
      next.forEach((r) => {
        if (r.channel_number != null) {
          dup.set(r.channel_number, (dup.get(r.channel_number) ?? 0) + 1);
        }
      });
      validateSingle(next[index], dup);
      recomputeAllValidity(next);
      return next;
    });
  };

  const addRow = () => {
    setRows((prev) => {
      const next = [
        ...prev,
        {
          id: crypto.randomUUID(),
          name: '',
          stream_url: '',
          _expanded: true,
          _errors: {},
        },
      ];
      recomputeAllValidity(next);
      return next;
    });
  };

  const removeRow = (index: number) => {
    setRows((prev) => {
      if (prev.length === 1) {
        const single = [
          {
            id: crypto.randomUUID(),
            name: '',
            stream_url: '',
            _expanded: true,
            _errors: {},
          },
        ];
        recomputeAllValidity(single);
        return single;
      }
      const next = prev.filter((_, i) => i !== index);
      recomputeAllValidity(next);
      return next;
    });
  };

  const toggleExpandAll = (expand: boolean) => {
    setRows((prev) => {
      prev.forEach((r) => {
        r._expanded = expand;
      });
      // No need to recompute validity (expansion does not affect validation)
      return [...prev];
    });
  };

  // --- Row Render ---
  // Extracted row component with memoization and explicit input refs for stable focus
  const RowBase: React.FC<{
    row: RowUI;
    index: number;
    disabled?: boolean;
    onChange: (index: number, patch: Partial<RowUI>) => void;
    onRemove: (index: number) => void;
  }> = ({ row, index, disabled, onChange, onRemove }) => {
    const hasErrors = row._errors && Object.keys(row._errors).length > 0;

    // Individual refs for focus retention
    const nameRef = useRef<HTMLInputElement | null>(null);
    const urlRef = useRef<HTMLInputElement | null>(null);
    const logoRef = useRef<HTMLInputElement | null>(null);

    // When _focusVersion changes, try to restore focus to last active element
    // Removed automatic refocus on each render to prevent focus loss loops

    const handleFieldChange = (
      patch: Partial<RowUI>,
      fieldRef: React.RefObject<HTMLInputElement | HTMLInputElement | null>
    ) => {
      // Mark a focus version so we can attempt re-focus after parent state updates
      onChange(index, { ...patch, _focusVersion: (row._focusVersion || 0) + 1 });
      // Defer focus restoration very slightly
      // Simple defer without forcing focus when element not mounted
      if (fieldRef.current) {
        requestAnimationFrame(() => {
          fieldRef.current && fieldRef.current.focus();
        });
      }
    };

    return (
      <div
        className={cn(
          'border rounded-md p-3 space-y-2 bg-background transition-colors',
          hasErrors && 'border-destructive/70'
        )}
      >
        <div className="flex flex-wrap items-center gap-2">
          <Input
            type="number"
            placeholder="#"
            className={cn('w-20', row._errors?.channel_number && 'border-destructive')}
            value={row.channel_number ?? ''}
            onChange={(e) =>
              handleFieldChange(
                {
                  channel_number: e.target.value ? parseInt(e.target.value, 10) : undefined,
                },
                urlRef // keep current active; numeric rarely needs refocus
              )
            }
            disabled={disabled}
          />

          <Input
            ref={nameRef}
            placeholder="Name"
            className={cn('min-w-[10rem] flex-1', row._errors?.name && 'border-destructive')}
            value={row.name}
            onChange={(e) => handleFieldChange({ name: e.target.value }, nameRef)}
            disabled={disabled}
          />

          <Input
            ref={urlRef}
            placeholder="Stream URL (e.g. http://example.com/live.m3u8)"
            className={cn(
              'min-w-[16rem] flex-[2]',
              row._errors?.stream_url && 'border-destructive'
            )}
            value={row.stream_url}
            onChange={(e) => handleFieldChange({ stream_url: e.target.value }, urlRef)}
            disabled={disabled}
          />

          <Input
            ref={logoRef}
            placeholder="Logo (e.g. @logo:token or https://example.com/logo.png)"
            className={cn('min-w-[14rem]', row._errors?.tvg_logo && 'border-destructive')}
            value={row.tvg_logo || ''}
            onChange={(e) => handleFieldChange({ tvg_logo: e.target.value || undefined }, logoRef)}
            disabled={disabled}
          />

          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => onChange(index, { _expanded: !row._expanded })}
            disabled={disabled}
            className="text-xs"
          >
            {row._expanded ? 'Hide' : 'Advanced'}
          </Button>

          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => onRemove(index)}
            disabled={disabled}
            className="text-destructive"
          >
            Remove
          </Button>

          <Badge
            variant={hasErrors ? 'destructive' : 'outline'}
            className="ml-auto px-2 py-0.5 text-[10px] font-medium"
          >
            {hasErrors ? 'Invalid' : 'OK'}
          </Badge>
        </div>

        {row._expanded && (
          <div className="grid md:grid-cols-3 gap-2 pt-1">
            <Input
              placeholder="Group"
              value={row.group_title || ''}
              onChange={(e) =>
                handleFieldChange({ group_title: e.target.value || undefined }, nameRef)
              }
              disabled={disabled}
            />
            <Input
              placeholder="TVG ID"
              value={row.tvg_id || ''}
              onChange={(e) => handleFieldChange({ tvg_id: e.target.value || undefined }, nameRef)}
              disabled={disabled}
            />
            <Input
              placeholder="TVG Name"
              value={row.tvg_name || ''}
              onChange={(e) =>
                handleFieldChange({ tvg_name: e.target.value || undefined }, nameRef)
              }
              disabled={disabled}
            />
            <Input
              placeholder="EPG ID"
              value={row.epg_id || ''}
              onChange={(e) => handleFieldChange({ epg_id: e.target.value || undefined }, nameRef)}
              disabled={disabled}
              className="md:col-span-3"
            />

            {row._errors && Object.values(row._errors).length > 0 && (
              <div className="text-xs text-destructive md:col-span-3 flex flex-wrap gap-2">
                {Object.entries(row._errors).map(([k, v]) => (
                  <span key={k}>{v}</span>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    );
  };
  const MemoRow = memo(RowBase);

  return (
    <div className={cn('space-y-4', className)}>
      {/* Header / Controls */}
      <div className="flex flex-wrap items-center gap-3">
        <h4 className="font-medium">Manual Channels</h4>
        {(() => {
          const totalRows = rows.length;
          const validRowsCount = rows.filter(
            (r) => !r._errors || Object.keys(r._errors!).length === 0
          ).length;
          const allRowsValid = validRowsCount === totalRows && totalRows >= minRequired;
          return (
            <Badge
              variant={allRowsValid ? 'default' : 'destructive'}
              className="text-xs"
              title={
                allRowsValid ? 'All rows valid' : 'All rows must be valid & need at least one row'
              }
            >
              {validRowsCount} / {totalRows} valid (need ≥ {minRequired})
            </Badge>
          );
        })()}
        <Button
          type="button"
          size="sm"
          variant="default"
          disabled={
            rows.length === 0 || rows.some((r) => r._errors && Object.keys(r._errors!).length > 0)
          }
          onClick={() => emitImmediateToParent(rows)}
        >
          Apply Changes
        </Button>

        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => toggleExpandAll(true)}
          disabled={disabled}
        >
          Expand All
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => toggleExpandAll(false)}
          disabled={disabled}
        >
          Collapse All
        </Button>
        <Button type="button" size="sm" onClick={addRow} disabled={disabled}>
          Add Channel
        </Button>
      </div>

      {/* Rows */}
      <div className="space-y-3">
        {rows.map((row, i) => (
          <MemoRow
            key={row.id}
            row={row}
            index={i}
            disabled={disabled}
            onChange={updateRow}
            onRemove={removeRow}
          />
        ))}
      </div>

      {/* Future enhancement hooks (import/export/paste) could be placed here */}
    </div>
  );
};

export default ManualChannelEditor;
