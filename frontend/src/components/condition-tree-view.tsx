'use client';

import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { Code, Filter, GitBranch, Layers, ChevronRight, ChevronDown, Hash } from 'lucide-react';
import { useState } from 'react';
import type { ConditionTreeNode } from '@/lib/filter-expression-converter';

interface ConditionTreeViewProps {
  conditionTreeJson: string | object;
  className?: string;
  compact?: boolean;
}

interface ConditionNodeProps {
  node: ConditionTreeNode;
  depth?: number;
  compact?: boolean;
  isLast?: boolean;
  parentOperator?: string;
}

// Function to flatten redundant nesting while preserving meaningful logical grouping
function flattenConditionTree(node: ConditionTreeNode): ConditionTreeNode {
  if (!node || node.type === 'condition') {
    return node;
  }

  if (node.type === 'group' && node.children) {
    // First, recursively flatten all children
    const flattenedChildren = node.children.map(flattenConditionTree);

    // Only flatten if we have redundant single-operator chains
    // Don't flatten meaningful groupings like ((a OR b) AND (c OR d))
    const newChildren: ConditionTreeNode[] = [];

    for (const child of flattenedChildren) {
      if (
        child.type === 'group' &&
        child.operator?.toLowerCase() === node.operator?.toLowerCase() &&
        child.children &&
        // Only flatten if this child group doesn't represent meaningful logical grouping
        // (i.e., it's just a redundant wrapper with the same operator)
        !hasMultipleOperatorTypes(flattenedChildren)
      ) {
        // Merge children of redundant same-operator groups
        newChildren.push(...child.children);
      } else {
        newChildren.push(child);
      }
    }

    return {
      ...node,
      children: newChildren,
    };
  }

  return node;
}

// Helper function to check if children have multiple different operators
// This helps us preserve meaningful grouping like ((a OR b) AND (c OR d))
function hasMultipleOperatorTypes(children: ConditionTreeNode[]): boolean {
  const operators = new Set<string>();

  for (const child of children) {
    if (child.type === 'group' && child.operator) {
      operators.add(child.operator.toLowerCase());
    }
  }

  return operators.size > 1;
}

function ConditionNode({
  node,
  depth = 0,
  compact = false,
  isLast = false,
  parentOperator,
}: ConditionNodeProps) {
  const [isExpanded, setIsExpanded] = useState(true);

  if (!node) return null;

  if (node.type === 'condition') {
    return (
      <div className={cn('flex items-center gap-2 py-1', depth > 0 && 'ml-6 relative')}>
        {depth > 0 && (
          <div className="absolute -left-6 top-0 bottom-0 w-6 flex items-center justify-center">
            <div
              className={cn('w-4 h-px bg-border', !isLast && 'border-l border-border h-full w-px')}
            />
            {!isLast && <div className="absolute top-1/2 left-0 w-4 h-px bg-border" />}
          </div>
        )}

        <Filter className="h-3 w-3 text-muted-foreground flex-shrink-0" />

        <div className="flex items-center gap-1 flex-wrap">
          {node.negate && (
            <Badge variant="destructive" className="text-xs px-1 py-0">
              NOT
            </Badge>
          )}

          {node.case_sensitive && (
            <Badge variant="secondary" className="text-xs px-1 py-0">
              Aa
            </Badge>
          )}

          <Badge variant="outline" className="text-xs font-mono">
            {node.field}
          </Badge>

          <span className="text-xs text-muted-foreground font-medium">
            {formatOperatorDisplay(node.operator)}
          </span>

          <Badge variant="default" className="text-xs">
            "{node.value}"
          </Badge>
        </div>
      </div>
    );
  }

  if (node.type === 'group' && node.children) {
    const hasMultipleChildren = node.children.length > 1;
    const canCollapse = !compact && hasMultipleChildren;

    return (
      <div className={cn('relative', depth > 0 && 'ml-6')}>
        {depth > 0 && (
          <div className="absolute -left-6 top-0 bottom-0 w-6 flex items-center justify-center">
            <div className={cn('border-l border-border h-full w-px', !isLast && 'border-l')} />
            <div className="absolute top-4 left-0 w-4 h-px bg-border" />
          </div>
        )}

        <div className="flex items-center gap-2 py-1 mb-2">
          {canCollapse && (
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              className="p-0.5 hover:bg-muted rounded transition-colors"
            >
              {isExpanded ? (
                <ChevronDown className="h-3 w-3" />
              ) : (
                <ChevronRight className="h-3 w-3" />
              )}
            </button>
          )}

          <GitBranch className="h-3 w-3 text-muted-foreground" />

          <Badge
            variant="secondary"
            className={cn(
              'text-xs font-semibold',
              node.operator?.toLowerCase() === 'and' && 'bg-blue-100 text-blue-700',
              node.operator?.toLowerCase() === 'or' && 'bg-amber-100 text-amber-700'
            )}
          >
            {node.operator?.toUpperCase() || 'GROUP'}
          </Badge>

          <span className="text-xs text-muted-foreground">
            {node.children.length} condition{node.children.length !== 1 ? 's' : ''}
          </span>
        </div>

        {(isExpanded || !canCollapse) && (
          <div className="space-y-1">
            {node.children.map((child, index) => (
              <ConditionNode
                key={index}
                node={child}
                depth={depth + 1}
                compact={compact}
                isLast={index === node.children!.length - 1}
                parentOperator={node.operator}
              />
            ))}
          </div>
        )}

        {!isExpanded && canCollapse && (
          <div className="ml-8 text-xs text-muted-foreground">
            ... {node.children.length} hidden conditions
          </div>
        )}
      </div>
    );
  }

  return null;
}

function formatOperatorDisplay(operator?: string): string {
  if (!operator) return 'unknown';

  const formatted = operator.toLowerCase();
  switch (formatted) {
    case 'contains':
    case 'not_contains':
      return 'contains';
    case 'equals':
    case 'not_equals':
      return 'equals';
    case 'matches':
    case 'not_matches':
      return 'matches';
    case 'starts_with':
    case 'startswith':
    case 'not_starts_with':
    case 'not_startswith':
      return 'starts with';
    case 'ends_with':
    case 'endswith':
    case 'not_ends_with':
    case 'not_endswith':
      return 'ends with';
    default:
      return formatted;
  }
}

export function ConditionTreeView({
  conditionTreeJson,
  className,
  compact = false,
}: ConditionTreeViewProps) {
  try {
    // Handle both string and object inputs
    let tree: any;
    if (typeof conditionTreeJson === 'string') {
      tree = JSON.parse(conditionTreeJson);
    } else {
      tree = conditionTreeJson;
    }

    if (!tree.root) {
      return (
        <div className={cn('text-center py-4', className)}>
          <Filter className="h-6 w-6 mx-auto mb-2 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">No condition tree available</p>
        </div>
      );
    }

    // Flatten the tree to reduce unnecessary nesting
    const flattenedRoot = flattenConditionTree(tree.root);

    return (
      <div className={cn('space-y-1', className)}>
        <ConditionNode node={flattenedRoot} compact={compact} isLast={true} />
      </div>
    );
  } catch (error) {
    return (
      <div className={cn('text-center py-4 text-destructive', className)}>
        <Hash className="h-6 w-6 mx-auto mb-2" />
        <p className="text-sm">Invalid condition tree format</p>
        <p className="text-xs text-muted-foreground mt-1">
          {error instanceof Error ? error.message : 'Parse error'}
        </p>
      </div>
    );
  }
}
