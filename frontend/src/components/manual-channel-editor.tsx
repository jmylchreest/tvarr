'use client';

import React, { useCallback, useMemo } from 'react';
import { InlineEditTable, ColumnDef } from '@/components/shared';

/**
 * ManualChannelEditor
 *
 * A tabular editor for defining manual stream channels using InlineEditTable.
 * Replaces the previous card-based editor with a more compact table view.
 *
 * Validation rules:
 * - channel_name: non-empty (trimmed)
 * - stream_url: must start with http://, https://, or rtsp://
 * - tvg_logo: empty OR starts with @logo: OR http(s)://
 * - channel_number: optional; if present must be unique among non-empty
 *
 * Parent integration:
 *  <ManualChannelEditor
 *     value={manualChannels}
 *     onChange={setManualChannels}
 *     onValidityChange={setManualChannelsValid}
 *  />
 */

export interface ManualChannelInput {
  channel_number?: number;
  channel_name: string;
  stream_url: string;
  group_title?: string;
  tvg_id?: string;
  tvg_name?: string;
  tvg_logo?: string;
  epg_id?: string;
}

export interface ManualChannelEditorProps {
  value: ManualChannelInput[];
  onChange: (rows: ManualChannelInput[]) => void;
  onValidityChange?: (valid: boolean) => void;
  disabled?: boolean;
  minRequired?: number;
  className?: string;
}

// Internal type with id for InlineEditTable
interface ManualChannelRow extends ManualChannelInput {
  _id: string;
}

// Validation functions
const validateChannelName = (value: string | undefined): string | undefined => {
  if (!value || !value.trim()) {
    return 'Name is required';
  }
  return undefined;
};

const validateStreamUrl = (value: string | undefined): string | undefined => {
  if (!value || !/^(https?|rtsp):\/\//.test(value)) {
    return 'Must start with http://, https://, or rtsp://';
  }
  return undefined;
};

const validateTvgLogo = (value: string | undefined): string | undefined => {
  if (!value || value.trim() === '') {
    return undefined; // Empty is valid
  }
  if (value.startsWith('@logo:') || /^https?:\/\//.test(value)) {
    return undefined;
  }
  return 'Invalid format: use @logo:token or URL';
};

// Resolve @logo:token or URL to a displayable image URL
const resolveLogoUrl = (value: string): string | undefined => {
  if (!value || value.trim() === '') {
    return undefined;
  }
  // @logo:ULID format - resolve to /logos/{ULID}.png endpoint
  if (value.startsWith('@logo:')) {
    const ulid = value.substring(6); // Remove '@logo:' prefix
    if (ulid) {
      return `/logos/${ulid}.png`;
    }
    return undefined;
  }
  // HTTP(S) URL - use directly
  if (/^https?:\/\//.test(value)) {
    return value;
  }
  return undefined;
};

// Create validator for channel_number that checks for duplicates
const createChannelNumberValidator = (allRows: ManualChannelRow[]) => {
  return (value: number | undefined, row: ManualChannelRow): string | undefined => {
    if (value === undefined || value === null) {
      return undefined; // Optional field
    }
    // Count occurrences of this channel number
    const count = allRows.filter(
      (r) => r.channel_number === value && r._id !== row._id
    ).length;
    if (count > 0) {
      return 'Duplicate channel number';
    }
    return undefined;
  };
};

export const ManualChannelEditor: React.FC<ManualChannelEditorProps> = ({
  value,
  onChange,
  onValidityChange,
  disabled = false,
  minRequired = 1,
  className,
}) => {
  // Convert input to internal format with stable IDs
  const internalRows = useMemo((): ManualChannelRow[] => {
    return value.map((row, index) => ({
      ...row,
      _id: `channel-${index}-${row.channel_name || 'new'}-${Date.now()}`,
    }));
  }, [value]);

  // Column definitions with validation
  const columns = useMemo((): ColumnDef<ManualChannelRow>[] => {
    const channelNumberValidator = createChannelNumberValidator(internalRows);

    return [
      {
        id: 'channel_number',
        header: '#',
        accessorKey: 'channel_number',
        width: '70px',
        minWidth: '70px',
        type: 'number',
        placeholder: '#',
        defaultVisible: true,
        validate: channelNumberValidator,
      },
      {
        id: 'channel_name',
        header: 'Name',
        accessorKey: 'channel_name',
        minWidth: '150px',
        required: true,
        type: 'text',
        placeholder: 'Channel name',
        validate: validateChannelName,
      },
      {
        id: 'stream_url',
        header: 'Stream URL',
        accessorKey: 'stream_url',
        minWidth: '250px',
        required: true,
        type: 'url',
        placeholder: 'http://example.com/live.m3u8',
        validate: validateStreamUrl,
      },
      {
        id: 'tvg_logo',
        header: 'Logo',
        accessorKey: 'tvg_logo',
        minWidth: '220px',
        type: 'image',
        placeholder: '@logo:token or URL',
        validate: validateTvgLogo,
        resolveImageUrl: resolveLogoUrl,
      },
      {
        id: 'group_title',
        header: 'Group',
        accessorKey: 'group_title',
        minWidth: '120px',
        type: 'text',
        placeholder: 'Group',
        defaultVisible: false,
      },
      {
        id: 'tvg_id',
        header: 'TVG ID',
        accessorKey: 'tvg_id',
        minWidth: '100px',
        type: 'text',
        placeholder: 'TVG ID',
        defaultVisible: false,
      },
      {
        id: 'tvg_name',
        header: 'TVG Name',
        accessorKey: 'tvg_name',
        minWidth: '120px',
        type: 'text',
        placeholder: 'TVG Name',
        defaultVisible: false,
      },
      {
        id: 'epg_id',
        header: 'EPG ID',
        accessorKey: 'epg_id',
        minWidth: '100px',
        type: 'text',
        placeholder: 'EPG ID',
        defaultVisible: false,
      },
    ];
  }, [internalRows]);

  // Create empty row
  const createEmpty = useCallback((): ManualChannelRow => {
    return {
      _id: `channel-new-${Date.now()}-${Math.random().toString(36).substring(7)}`,
      channel_name: '',
      stream_url: '',
    };
  }, []);

  // Get row ID
  const getRowId = useCallback((row: ManualChannelRow): string => {
    return row._id;
  }, []);

  // Handle data changes - strip internal _id before passing to parent
  const handleChange = useCallback(
    (data: ManualChannelRow[]) => {
      const cleaned: ManualChannelInput[] = data.map(({ _id, ...rest }) => rest);
      onChange(cleaned);
    },
    [onChange]
  );

  // Handle validity changes
  const handleValidityChange = useCallback(
    (isValid: boolean) => {
      onValidityChange?.(isValid);
    },
    [onValidityChange]
  );

  return (
    <InlineEditTable
      columns={columns}
      data={internalRows}
      onChange={handleChange}
      createEmpty={createEmpty}
      getRowId={getRowId}
      onValidityChange={handleValidityChange}
      isLoading={false}
      canAdd={!disabled}
      canRemove={!disabled}
      canReorder={false}
      minRows={minRequired}
      className={className}
      emptyState={{
        title: 'No channels defined',
        description: 'Add channels to this manual source',
      }}
    />
  );
};

export default ManualChannelEditor;
