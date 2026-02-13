'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { GripVertical, Plus, Trash2 } from 'lucide-react';
import { BadgeGroup, BadgeItem, BadgePriority } from './BadgeGroup';
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

export type SelectionBadge = BadgeItem;

export interface SelectionItem {
  id: string;
  title: string;
  subtitle?: string;
  badges?: SelectionBadge[];
  disabled?: boolean;
}

export interface SelectedItem {
  id: string;
  isActive?: boolean;
}

interface SortableItemProps {
  item: SelectionItem;
  isActive?: boolean;
  onToggleActive?: () => void;
  onRemove: () => void;
  position: number;
  showToggle?: boolean;
}

function SortableItem({
  item,
  isActive,
  onToggleActive,
  onRemove,
  position,
  showToggle = false,
}: SortableItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'flex items-center gap-1.5 px-2 py-1.5 bg-background border-b last:border-b-0',
        isDragging && 'opacity-50 z-50 shadow-lg'
      )}
    >
      {/* Drag Handle */}
      <button
        type="button"
        className="flex-shrink-0 cursor-grab active:cursor-grabbing text-muted-foreground hover:text-foreground p-0.5 -ml-1"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5" />
      </button>

      {/* Position Number */}
      <span className="text-[10px] text-muted-foreground w-4 text-center flex-shrink-0 font-mono">
        {position}
      </span>

      {/* Title & Badges */}
      <div className="flex-1 min-w-0 flex items-center gap-2 overflow-hidden">
        <span className={cn(
          'text-sm font-medium truncate',
          item.disabled && 'text-muted-foreground'
        )}>
          {item.title}
        </span>
        {item.subtitle && (
          <span className="text-xs text-muted-foreground truncate flex-shrink-0">
            {item.subtitle}
          </span>
        )}
      </div>

      {/* Badges */}
      {item.badges && item.badges.length > 0 && (
        <BadgeGroup badges={item.badges} size="sm" />
      )}

      {/* Active Toggle */}
      {showToggle && onToggleActive && (
        <Switch
          checked={isActive}
          onCheckedChange={onToggleActive}
          className="scale-75 flex-shrink-0"
        />
      )}

      {/* Remove Button */}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={onRemove}
        className="h-6 w-6 p-0 text-destructive hover:text-destructive flex-shrink-0"
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

interface UnselectedItemProps {
  item: SelectionItem;
  onAdd: () => void;
}

function UnselectedItem({ item, onAdd }: UnselectedItemProps) {
  return (
    <div
      className={cn(
        'flex items-center gap-2 px-2 py-1.5 border-b last:border-b-0 hover:bg-accent/50 cursor-pointer transition-colors',
        item.disabled && 'opacity-50 cursor-not-allowed'
      )}
      onClick={item.disabled ? undefined : onAdd}
    >
      {/* Add Button */}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={(e) => {
          e.stopPropagation();
          if (!item.disabled) onAdd();
        }}
        disabled={item.disabled}
        className="h-6 w-6 p-0 text-primary hover:text-primary flex-shrink-0"
      >
        <Plus className="h-3.5 w-3.5" />
      </Button>

      {/* Title */}
      <div className="flex-1 min-w-0 flex items-center gap-2 overflow-hidden">
        <span className={cn(
          'text-sm font-medium truncate',
          item.disabled && 'text-muted-foreground'
        )}>
          {item.title}
        </span>
        {item.subtitle && (
          <span className="text-xs text-muted-foreground truncate flex-shrink-0">
            {item.subtitle}
          </span>
        )}
      </div>

      {/* Badges */}
      {item.badges && item.badges.length > 0 && (
        <BadgeGroup badges={item.badges} size="sm" />
      )}
    </div>
  );
}

export interface SortableSelectionListProps {
  /** All available items */
  items: SelectionItem[];
  /** Currently selected items with their order and optional active state */
  selectedItems: SelectedItem[];
  /** Called when selection changes */
  onSelectionChange: (items: SelectedItem[]) => void;
  /** Whether to show active/inactive toggle for selected items */
  showActiveToggle?: boolean;
  /** Maximum height for the list */
  maxHeight?: string;
  /** Empty state message when no items available */
  emptyMessage?: string;
  /** Label for selected section */
  selectedLabel?: string;
  /** Label for available section */
  availableLabel?: string;
  /** Additional class names */
  className?: string;
}

/**
 * SortableSelectionList - A unified list for selecting and ordering items
 *
 * Selected items appear at the top with drag handles for reordering.
 * Unselected items appear below with + buttons to add them.
 */
export function SortableSelectionList({
  items,
  selectedItems,
  onSelectionChange,
  showActiveToggle = false,
  maxHeight = '400px',
  emptyMessage = 'No items available',
  selectedLabel = 'Selected',
  availableLabel = 'Available',
  className,
}: SortableSelectionListProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 5,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  // Get item details by ID
  const getItem = (id: string) => items.find((item) => item.id === id);

  // Selected items in order
  const sortedSelectedItems = selectedItems
    .map((selected) => ({
      ...selected,
      item: getItem(selected.id),
    }))
    .filter((x) => x.item);

  // Unselected items
  const unselectedItems = items.filter(
    (item) => !selectedItems.some((s) => s.id === item.id)
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = selectedItems.findIndex((item) => item.id === active.id);
    const newIndex = selectedItems.findIndex((item) => item.id === over.id);

    if (oldIndex !== -1 && newIndex !== -1) {
      onSelectionChange(arrayMove(selectedItems, oldIndex, newIndex));
    }
  };

  const handleAdd = (id: string) => {
    onSelectionChange([
      ...selectedItems,
      { id, isActive: showActiveToggle ? true : undefined },
    ]);
  };

  const handleRemove = (id: string) => {
    onSelectionChange(selectedItems.filter((item) => item.id !== id));
  };

  const handleToggleActive = (id: string) => {
    onSelectionChange(
      selectedItems.map((item) =>
        item.id === id ? { ...item, isActive: !item.isActive } : item
      )
    );
  };

  if (items.length === 0) {
    return (
      <div className="text-center py-6 text-sm text-muted-foreground border rounded-lg border-dashed">
        {emptyMessage}
      </div>
    );
  }

  return (
    <div className={cn('border rounded-md overflow-hidden', className)}>
      <div style={{ maxHeight }} className="overflow-y-auto">
        {/* Selected Items Section */}
        {sortedSelectedItems.length > 0 && (
          <div>
            <div className="px-2 py-1 bg-muted/50 text-xs font-medium text-muted-foreground border-b sticky top-0 z-10">
              {selectedLabel} ({sortedSelectedItems.length})
            </div>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={selectedItems.map((item) => item.id)}
                strategy={verticalListSortingStrategy}
              >
                {sortedSelectedItems.map((selected, index) => (
                  <SortableItem
                    key={selected.id}
                    item={selected.item!}
                    isActive={selected.isActive}
                    onToggleActive={
                      showActiveToggle
                        ? () => handleToggleActive(selected.id)
                        : undefined
                    }
                    onRemove={() => handleRemove(selected.id)}
                    position={index + 1}
                    showToggle={showActiveToggle}
                  />
                ))}
              </SortableContext>
            </DndContext>
          </div>
        )}

        {/* Unselected Items Section */}
        {unselectedItems.length > 0 && (
          <div>
            <div className="px-2 py-1 bg-muted/50 text-xs font-medium text-muted-foreground border-b sticky top-0 z-10">
              {availableLabel} ({unselectedItems.length})
            </div>
            {unselectedItems.map((item) => (
              <UnselectedItem
                key={item.id}
                item={item}
                onAdd={() => handleAdd(item.id)}
              />
            ))}
          </div>
        )}

        {/* Empty selected state */}
        {sortedSelectedItems.length === 0 && unselectedItems.length > 0 && (
          <div className="text-center py-4 text-xs text-muted-foreground border-b">
            Click + to add items
          </div>
        )}
      </div>
    </div>
  );
}

export default SortableSelectionList;
