'use client';

import React, { useState, useCallback, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Checkbox } from '@/components/ui/checkbox';
import { Search, Plus, ChevronLeft, ChevronRight, Trash2, CheckSquare, XSquare, GripVertical } from 'lucide-react';
import { EmptyState } from '../feedback/EmptyState';
import { SkeletonList } from '../feedback/SkeletonTable';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

export interface BulkAction {
  id: string;
  label: string;
  icon?: React.ReactNode;
  variant?: 'default' | 'destructive' | 'outline';
  onClick: (selectedIds: string[]) => void | Promise<void>;
}

export type MasterItemStatus = 'default' | 'active' | 'warning' | 'error' | 'success';

export interface MasterItem {
  id: string;
  title: string;
  subtitle?: string;
  badge?: React.ReactNode;
  icon?: React.ReactNode;
  /** Whether the item is enabled (affects visual styling) */
  enabled?: boolean;
  /** Status for collapsed view styling (color/animation) */
  status?: MasterItemStatus;
  /** Whether this item should show animation (e.g., sparkle when transcoding) */
  animate?: boolean;
}

export interface MasterDetailLayoutProps<T extends MasterItem> {
  /** Items to display in the master list */
  items: T[];
  /** Currently selected item ID */
  selectedId?: string | null;
  /** Callback when an item is selected */
  onSelect: (item: T | null) => void;
  /** Callback when add button is clicked */
  onAdd?: () => void;
  /** Custom header action element (replaces default add button) */
  headerAction?: React.ReactNode;
  /** Loading state */
  isLoading?: boolean;
  /** Title for the master panel */
  title?: string;
  /** Add button label */
  addLabel?: string;
  /** Search placeholder */
  searchPlaceholder?: string;
  /** Empty state configuration */
  emptyState?: {
    title: string;
    description?: string;
    actionLabel?: string;
  };
  /** Detail panel content (render prop receives selected item) */
  children: (selectedItem: T | null) => React.ReactNode;
  /** Additional className for container */
  className?: string;
  /** Master panel width (default: 320px) */
  masterWidth?: number;
  /** Whether master panel can be collapsed */
  collapsible?: boolean;
  /** Custom filter function for search */
  filterFn?: (item: T, searchTerm: string) => boolean;
  /** Custom render function for master list items */
  renderItem?: (item: T, isSelected: boolean) => React.ReactNode;
  /** Enable multi-select mode with checkboxes */
  selectable?: boolean;
  /** Currently selected item IDs (for bulk selection) */
  selectedIds?: string[];
  /** Callback when selection changes */
  onSelectionChange?: (ids: string[]) => void;
  /** Bulk actions to show when items are selected */
  bulkActions?: BulkAction[];
  /** Enable drag/drop reordering */
  sortable?: boolean;
  /** Callback when items are reordered (receives new order of IDs) */
  onReorder?: (reorderedIds: string[]) => void | Promise<void>;
  /** Start with the master panel collapsed */
  defaultCollapsed?: boolean;
  /** localStorage key to persist collapsed state (e.g., 'transcoders-panel') */
  storageKey?: string;
  /** Force the detail panel to be visible on mobile (e.g., during create mode
   *  when selectedId is null but the detail panel has content to show) */
  showDetailPanel?: boolean;
}

/**
 * SortableItem - A wrapper component for sortable list items
 * Shows position number integrated with drag handle: ::1, ::2, etc.
 */
function SortableItem({
  id,
  children,
  disabled = false,
  position,
  totalDigits = 1,
}: {
  id: string;
  children: React.ReactNode;
  disabled?: boolean;
  /** Current position (1-indexed) */
  position?: number;
  /** Number of digits to pad to (for alignment) */
  totalDigits?: number;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    zIndex: isDragging ? 1 : 0,
  };

  // Format position with padding for alignment (e.g., "  1" when total is "199")
  const formattedPosition = position !== undefined
    ? String(position).padStart(totalDigits, '\u00A0') // Use non-breaking space for padding
    : '';

  return (
    <div ref={setNodeRef} style={style} className="flex items-center gap-1 group">
      <button
        {...attributes}
        {...listeners}
        className={cn(
          'flex-shrink-0 cursor-grab active:cursor-grabbing rounded hover:bg-accent touch-none p-0.5',
          'font-mono text-[10px] text-muted-foreground flex items-center',
          disabled && 'opacity-30 cursor-not-allowed'
        )}
        disabled={disabled}
        aria-label={`Drag to reorder (position ${position})`}
        title={`Position ${position} - drag to reorder`}
      >
        <GripVertical className="h-3.5 w-3.5" />
        <span className="text-muted-foreground/70 tabular-nums leading-none">{formattedPosition}</span>
      </button>
      {children}
    </div>
  );
}

/**
 * MasterDetailLayout - A two-panel layout with a master list and detail view
 *
 * This replaces sheet-based editing with a persistent side-by-side layout.
 *
 * Usage:
 * ```tsx
 * <MasterDetailLayout
 *   items={sources}
 *   selectedId={selectedSource?.id}
 *   onSelect={setSelectedSource}
 *   onAdd={() => setShowCreate(true)}
 *   title="Stream Sources"
 * >
 *   {(source) => source ? <SourceEditor source={source} /> : <EmptyDetail />}
 * </MasterDetailLayout>
 * ```
 */
export function MasterDetailLayout<T extends MasterItem>({
  items,
  selectedId,
  onSelect,
  onAdd,
  headerAction,
  isLoading = false,
  title = 'Items',
  addLabel = 'Add',
  searchPlaceholder = 'Search...',
  emptyState,
  children,
  className,
  masterWidth = 320,
  collapsible = true,
  filterFn,
  renderItem,
  selectable = false,
  selectedIds = [],
  onSelectionChange,
  bulkActions = [],
  sortable = false,
  onReorder,
  defaultCollapsed = false,
  storageKey,
  showDetailPanel = false,
}: MasterDetailLayoutProps<T>) {
  const [searchTerm, setSearchTerm] = useState('');
  const [isMobile, setIsMobile] = useState(false);
  const [showDetailOnMobile, setShowDetailOnMobile] = useState(false);

  // Initialize collapsed state from localStorage or default
  const [isCollapsed, setIsCollapsed] = useState(() => {
    if (typeof window === 'undefined' || !storageKey) return defaultCollapsed;
    try {
      const stored = localStorage.getItem(`master-detail-${storageKey}-collapsed`);
      return stored !== null ? stored === 'true' : defaultCollapsed;
    } catch {
      return defaultCollapsed;
    }
  });

  // Persist collapsed state to localStorage
  useEffect(() => {
    if (!storageKey) return;
    try {
      localStorage.setItem(`master-detail-${storageKey}-collapsed`, String(isCollapsed));
    } catch {
      // Ignore localStorage errors
    }
  }, [isCollapsed, storageKey]);

  // DnD sensors for drag and drop
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  // Detect mobile viewport
  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768);
    };
    checkMobile();
    window.addEventListener('resize', checkMobile);
    return () => window.removeEventListener('resize', checkMobile);
  }, []);

  // On mobile, show detail panel when an item is selected OR when the
  // consumer explicitly requests it (e.g., during create mode where
  // selectedId is null but the detail panel has content to display).
  useEffect(() => {
    if (isMobile && (selectedId || showDetailPanel)) {
      setShowDetailOnMobile(true);
    } else if (isMobile && !selectedId && !showDetailPanel) {
      setShowDetailOnMobile(false);
    }
  }, [isMobile, selectedId, showDetailPanel]);

  // Handle back navigation on mobile
  const handleMobileBack = useCallback(() => {
    setShowDetailOnMobile(false);
    onSelect(null);
  }, [onSelect]);

  // Default filter function
  const defaultFilter = useCallback(
    (item: T, term: string) => {
      const lower = term.toLowerCase();
      return (
        item.title.toLowerCase().includes(lower) ||
        (item.subtitle?.toLowerCase().includes(lower) ?? false)
      );
    },
    []
  );

  const filter = filterFn ?? defaultFilter;

  // Filter items based on search
  const filteredItems = searchTerm
    ? items.filter((item) => filter(item, searchTerm))
    : items;

  // Bulk selection handlers
  const handleToggleSelect = useCallback(
    (id: string) => {
      if (!onSelectionChange) return;
      const newSelection = selectedIds.includes(id)
        ? selectedIds.filter((i) => i !== id)
        : [...selectedIds, id];
      onSelectionChange(newSelection);
    },
    [selectedIds, onSelectionChange]
  );

  const handleSelectAll = useCallback(() => {
    if (!onSelectionChange) return;
    const allIds = filteredItems.map((item) => item.id);
    onSelectionChange(allIds);
  }, [filteredItems, onSelectionChange]);

  const handleDeselectAll = useCallback(() => {
    if (!onSelectionChange) return;
    onSelectionChange([]);
  }, [onSelectionChange]);

  // Handle drag end for sortable lists
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;

      if (over && active.id !== over.id && onReorder) {
        const oldIndex = filteredItems.findIndex((item) => item.id === active.id);
        const newIndex = filteredItems.findIndex((item) => item.id === over.id);

        if (oldIndex !== -1 && newIndex !== -1) {
          const reorderedItems = arrayMove(filteredItems, oldIndex, newIndex);
          onReorder(reorderedItems.map((item) => item.id));
        }
      }
    },
    [filteredItems, onReorder]
  );

  // Find selected item
  const selectedItem = selectedId
    ? items.find((item) => item.id === selectedId) ?? null
    : null;

  // Default item renderer
  const defaultRenderItem = (item: T, isSelected: boolean) => {
    const isDisabled = item.enabled === false;
    return (
      <div
        className={cn(
          'flex items-center gap-2 px-2 py-1.5 rounded-md cursor-pointer transition-colors overflow-hidden',
          'hover:bg-accent',
          isSelected && 'bg-accent',
          isDisabled && 'opacity-50'
        )}
      >
        {item.icon && (
          <div className={cn('flex-shrink-0', isDisabled ? 'text-muted-foreground/50' : 'text-muted-foreground')}>
            {item.icon}
          </div>
        )}
        <div className="flex-1 min-w-0 overflow-hidden">
          <div className={cn('text-sm font-medium truncate', isDisabled && 'text-muted-foreground')}>
            {item.title}
          </div>
          {item.subtitle && (
            <div className="text-[11px] text-muted-foreground truncate">
              {item.subtitle}
            </div>
          )}
        </div>
        {item.badge && <div className="flex-shrink-0 ml-auto">{item.badge}</div>}
      </div>
    );
  };

  const itemRenderer = renderItem ?? defaultRenderItem;

  // On mobile, show either master or detail (not side-by-side)
  const showMaster = !isMobile || !showDetailOnMobile;
  const showDetail = !isMobile || showDetailOnMobile;

  return (
    <div className={cn('flex h-full', className)}>
      {/* Master Panel */}
      {showMaster && (
        <div
          className={cn(
            'flex flex-col border-r bg-card transition-all duration-200 min-h-0 overflow-hidden',
            isMobile ? 'w-full' : isCollapsed ? 'w-12' : `w-[${masterWidth}px]`
          )}
          style={{ width: isMobile ? '100%' : isCollapsed ? 48 : masterWidth }}
        >
          {/* Master Header */}
          {!isCollapsed && (
            <>
              <div className="flex items-center justify-between px-4 py-3 border-b">
                <h2 className="font-semibold text-sm">{title}</h2>
                <div className="flex items-center gap-1">
                  {headerAction ? (
                    headerAction
                  ) : onAdd ? (
                    <Button size="sm" variant="ghost" onClick={onAdd}>
                      <Plus className="h-4 w-4" />
                      <span className="sr-only">{addLabel}</span>
                    </Button>
                  ) : null}
                  {collapsible && !isMobile && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setIsCollapsed(true)}
                      aria-expanded="true"
                      aria-label="Collapse panel"
                    >
                      <ChevronLeft className="h-4 w-4" />
                      <span className="sr-only">Collapse</span>
                    </Button>
                  )}
                </div>
              </div>

            {/* Search */}
            <div className="px-3 py-2 border-b">
              <div className="relative">
                <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder={searchPlaceholder}
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8 h-9"
                  aria-label={searchPlaceholder}
                />
              </div>
            </div>

            {/* Bulk Actions Bar */}
            {selectable && (
              <div className="px-3 py-2 border-b bg-muted/50">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    {selectedIds.length > 0 ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={handleDeselectAll}
                        className="h-7 px-2"
                      >
                        <XSquare className="h-4 w-4 mr-1" />
                        Clear ({selectedIds.length})
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={handleSelectAll}
                        className="h-7 px-2"
                        disabled={filteredItems.length === 0}
                      >
                        <CheckSquare className="h-4 w-4 mr-1" />
                        Select All
                      </Button>
                    )}
                  </div>
                  {selectedIds.length > 0 && bulkActions.length > 0 && (
                    <div className="flex items-center gap-1">
                      {bulkActions.map((action) => (
                        <Button
                          key={action.id}
                          size="sm"
                          variant={action.variant ?? 'ghost'}
                          onClick={() => action.onClick(selectedIds)}
                          className="h-7 px-2"
                        >
                          {action.icon}
                          <span className="ml-1">{action.label}</span>
                        </Button>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            )}
          </>
        )}

        {/* Collapsed state - vertical item indicators */}
        {isCollapsed && collapsible && (
          <div className="flex flex-col h-full">
            {/* Expand button */}
            <Button
              size="sm"
              variant="ghost"
              className="m-1"
              onClick={() => setIsCollapsed(false)}
              aria-expanded="false"
              aria-label="Expand panel"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>

            {/* Item count */}
            <div className="text-[10px] text-center text-muted-foreground px-1 py-1 border-b">
              {selectedId ? `${filteredItems.findIndex(i => i.id === selectedId) + 1}/` : ''}{filteredItems.length}
            </div>

            {/* Clickable item indicators */}
            <ScrollArea className="flex-1">
              <div className="flex flex-col items-center gap-1 py-2">
                {filteredItems.map((item, index) => {
                  const isSelected = item.id === selectedId;
                  // Status-based styling for collapsed indicators using semantic theme colors
                  const getStatusClasses = () => {
                    if (isSelected) return 'bg-primary text-primary-foreground';
                    switch (item.status) {
                      case 'active':
                        return 'bg-info text-info-foreground';
                      case 'warning':
                        return 'bg-warning text-warning-foreground';
                      case 'error':
                        return 'bg-destructive text-destructive-foreground';
                      case 'success':
                        return 'bg-success text-success-foreground';
                      default:
                        return 'bg-muted text-muted-foreground';
                    }
                  };
                  return (
                    <button
                      key={item.id}
                      onClick={() => onSelect(item)}
                      className={cn(
                        'w-6 h-6 rounded-sm flex items-center justify-center text-[10px] font-medium transition-colors',
                        'hover:opacity-80 focus:outline-none focus:ring-1 focus:ring-ring',
                        getStatusClasses(),
                        item.animate && 'badge-sparkle'
                      )}
                      title={item.title}
                      aria-label={`Select ${item.title}`}
                    >
                      {index + 1}
                    </button>
                  );
                })}
              </div>
            </ScrollArea>
          </div>
        )}

        {/* Master List */}
        {!isCollapsed && (
          <ScrollArea className="flex-1 min-h-0">
            <div className="p-2">
              {isLoading ? (
                <SkeletonList items={5} />
              ) : filteredItems.length === 0 ? (
                <EmptyState
                  title={emptyState?.title ?? 'No items'}
                  description={
                    searchTerm
                      ? 'Try adjusting your search'
                      : emptyState?.description
                  }
                  action={
                    !searchTerm && onAdd && emptyState?.actionLabel
                      ? {
                          label: emptyState.actionLabel,
                          onClick: onAdd,
                        }
                      : undefined
                  }
                  size="sm"
                />
              ) : sortable && onReorder ? (
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragEnd={handleDragEnd}
                >
                  <SortableContext
                    items={filteredItems.map((item) => item.id)}
                    strategy={verticalListSortingStrategy}
                  >
                    <div className="space-y-1" role="listbox" aria-label={title}>
                      {filteredItems.map((item, index) => {
                        // Calculate total digits needed for alignment (e.g., 3 digits for 100+ items)
                        const totalDigits = String(filteredItems.length).length;
                        return (
                          <SortableItem
                            key={item.id}
                            id={item.id}
                            position={index + 1}
                            totalDigits={totalDigits}
                          >
                            {selectable && (
                              <Checkbox
                                checked={selectedIds.includes(item.id)}
                                onCheckedChange={() => handleToggleSelect(item.id)}
                                className="flex-shrink-0"
                                onClick={(e) => e.stopPropagation()}
                                aria-label={`Select ${item.title}`}
                              />
                            )}
                            <div
                              className="flex-1 min-w-0"
                              onClick={() => onSelect(item)}
                              role="option"
                              aria-selected={item.id === selectedId}
                              tabIndex={0}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                  e.preventDefault();
                                  onSelect(item);
                                }
                              }}
                            >
                              {itemRenderer(item, item.id === selectedId)}
                            </div>
                          </SortableItem>
                        );
                      })}
                    </div>
                  </SortableContext>
                </DndContext>
              ) : (
                <div className="space-y-1" role="listbox" aria-label={title}>
                  {filteredItems.map((item) => (
                    <div
                      key={item.id}
                      className={cn(
                        'flex items-center gap-1',
                        selectable && 'group'
                      )}
                    >
                      {selectable && (
                        <Checkbox
                          checked={selectedIds.includes(item.id)}
                          onCheckedChange={() => handleToggleSelect(item.id)}
                          className="ml-1 flex-shrink-0"
                          onClick={(e) => e.stopPropagation()}
                          aria-label={`Select ${item.title}`}
                        />
                      )}
                      <div
                        className="flex-1 min-w-0"
                        onClick={() => onSelect(item)}
                        role="option"
                        aria-selected={item.id === selectedId}
                        tabIndex={0}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            onSelect(item);
                          }
                        }}
                      >
                        {itemRenderer(item, item.id === selectedId)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </ScrollArea>
        )}
        </div>
      )}

      {/* Detail Panel */}
      {showDetail && (
        <div className={cn('flex-1 overflow-hidden flex flex-col min-h-0', isMobile && 'w-full')}>
          {/* Mobile back button header */}
          {isMobile && (selectedItem || showDetailPanel) && (
            <div className="flex items-center gap-2 px-4 py-3 border-b bg-card">
              <Button size="sm" variant="ghost" onClick={handleMobileBack}>
                <ChevronLeft className="h-4 w-4 mr-1" />
                Back
              </Button>
              {selectedItem && <span className="font-medium truncate">{selectedItem.title}</span>}
            </div>
          )}
          <div className="flex-1 overflow-hidden min-h-0">{children(selectedItem)}</div>
        </div>
      )}
    </div>
  );
}

/**
 * DetailPanel - A wrapper component for detail content with consistent styling
 */
export const DetailPanel: React.FC<{
  children: React.ReactNode;
  className?: string;
  title?: string;
  actions?: React.ReactNode;
}> = ({ children, className, title, actions }) => {
  return (
    <div className={cn('h-full flex flex-col', className)}>
      {(title || actions) && (
        <div className="flex items-center justify-between px-6 py-4 border-b">
          {title && <h2 className="text-lg font-semibold">{title}</h2>}
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      <ScrollArea className="flex-1 min-h-0">
        <div className="p-6">{children}</div>
      </ScrollArea>
    </div>
  );
};

/**
 * DetailEmpty - Placeholder shown when no item is selected
 */
export const DetailEmpty: React.FC<{
  title?: string;
  description?: string;
  icon?: React.ReactNode;
}> = ({
  title = 'Select an item',
  description = 'Choose an item from the list to view details',
  icon,
}) => {
  return (
    <div className="h-full flex items-center justify-center">
      <EmptyState title={title} description={description} icon={icon} />
    </div>
  );
};

export default MasterDetailLayout;
