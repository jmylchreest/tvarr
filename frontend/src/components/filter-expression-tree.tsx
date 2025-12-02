'use client';

import { FilterExpressionTree } from '@/types/api';
import { Badge } from '@/components/ui/badge';
import {
  GitBranch,
  ArrowRight,
  ChevronRight,
  ChevronDown,
  Code2,
  Filter as FilterIcon,
} from 'lucide-react';
import { useState } from 'react';

interface FilterExpressionTreeViewProps {
  tree: FilterExpressionTree;
  compact?: boolean;
  className?: string;
}

function getOperatorColor(operator: string): string {
  switch (operator?.toLowerCase()) {
    case 'contains':
    case 'not_contains':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200';
    case 'equals':
    case 'not_equals':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200';
    case 'matches':
    case 'not_matches':
      return 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200';
    case 'starts_with':
    case 'ends_with':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200';
    case 'and':
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200';
    case 'or':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200';
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200';
  }
}

function formatOperatorName(operator: string): string {
  switch (operator?.toLowerCase()) {
    case 'contains':
      return 'Contains';
    case 'not_contains':
      return 'NotContains';
    case 'equals':
      return 'Equals';
    case 'not_equals':
      return 'NotEquals';
    case 'matches':
      return 'Matches';
    case 'not_matches':
      return 'NotMatches';
    case 'starts_with':
      return 'StartsWith';
    case 'ends_with':
      return 'EndsWith';
    case 'and':
      return 'AND';
    case 'or':
      return 'OR';
    default:
      return operator || 'Unknown';
  }
}

function ConditionNode({
  tree,
  level = 0,
  isLast = false,
  compact = false,
}: {
  tree: FilterExpressionTree;
  level?: number;
  isLast?: boolean;
  compact?: boolean;
}) {
  if (tree.type === 'condition') {
    const operatorName = formatOperatorName(tree.operator || '');
    const caseSensitive = tree.case_sensitive ? ' (case-sensitive)' : ' (case-insensitive)';
    const negated = tree.negate ? 'NOT ' : '';

    return (
      <div className={`flex items-center gap-2 ${level > 0 ? 'ml-4' : ''}`}>
        {level > 0 && (
          <div className="flex items-center text-muted-foreground">{isLast ? '└──' : '├──'}</div>
        )}
        <div className="flex items-center gap-2 flex-wrap">
          <Badge variant="outline" className="text-xs font-mono">
            {tree.field}
          </Badge>
          {negated && (
            <Badge variant="destructive" className="text-xs">
              NOT
            </Badge>
          )}
          <Badge className={`text-xs ${getOperatorColor(tree.operator || '')}`}>
            {operatorName}
          </Badge>
          <code className="text-xs bg-muted px-2 py-1 rounded font-mono max-w-[200px] truncate">
            "{tree.value}"
          </code>
          {!compact && <span className="text-xs text-muted-foreground">{caseSensitive}</span>}
        </div>
      </div>
    );
  }

  return null;
}

function GroupNode({
  tree,
  level = 0,
  isLast = false,
  compact = false,
}: {
  tree: FilterExpressionTree;
  level?: number;
  isLast?: boolean;
  compact?: boolean;
}) {
  const [isExpanded, setIsExpanded] = useState(level < 2); // Auto-expand first 2 levels

  if (tree.type === 'group' && tree.children) {
    const operatorName = formatOperatorName(tree.operator || 'AND');

    return (
      <div className={level > 0 ? 'ml-4' : ''}>
        <div className="flex items-center gap-2 mb-1">
          {level > 0 && (
            <div className="flex items-center text-muted-foreground">{isLast ? '└──' : '├──'}</div>
          )}
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="flex items-center gap-1 hover:bg-accent rounded px-1 py-0.5 transition-colors"
          >
            {isExpanded ? (
              <ChevronDown className="h-3 w-3" />
            ) : (
              <ChevronRight className="h-3 w-3" />
            )}
            <Badge className={`text-xs ${getOperatorColor(tree.operator || 'and')}`}>
              {operatorName}
            </Badge>
            <span className="text-xs text-muted-foreground">
              ({tree.children.length} condition{tree.children.length !== 1 ? 's' : ''})
            </span>
          </button>
        </div>

        {isExpanded && (
          <div className="space-y-1">
            {tree.children.map((child, index) => {
              // Ensure all parts are strings to prevent [object Object] keys
              const keyParts = [
                level.toString(),
                index.toString(),
                String(child.type || 'unknown'),
                String(child.field || child.operator || `item-${index}`),
              ];
              const uniqueKey = keyParts.join('-');

              return (
                <ExpressionTreeNode
                  key={uniqueKey}
                  tree={child}
                  level={level + 1}
                  isLast={index === tree.children!.length - 1}
                  compact={compact}
                />
              );
            })}
          </div>
        )}
      </div>
    );
  }

  return null;
}

function ExpressionTreeNode({
  tree,
  level = 0,
  isLast = false,
  compact = false,
}: {
  tree: FilterExpressionTree;
  level?: number;
  isLast?: boolean;
  compact?: boolean;
}) {
  if (tree.type === 'condition') {
    return <ConditionNode tree={tree} level={level} isLast={isLast} compact={compact} />;
  }

  if (tree.type === 'group') {
    return <GroupNode tree={tree} level={level} isLast={isLast} compact={compact} />;
  }

  return null;
}

export function FilterExpressionTreeView({
  tree,
  compact = false,
  className = '',
}: FilterExpressionTreeViewProps) {
  return (
    <div className={`font-mono text-sm ${className}`}>
      <div className="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
        <GitBranch className="h-3 w-3" />
        <span>Logical Structure:</span>
      </div>
      <ExpressionTreeNode tree={tree} compact={compact} />
    </div>
  );
}

// Compact version for table display
export function FilterExpressionCompact({ tree }: { tree: FilterExpressionTree }) {
  if (tree.type === 'condition') {
    return (
      <div className="flex items-center gap-1 flex-wrap">
        <Badge variant="outline" className="text-xs font-mono">
          {tree.field}
        </Badge>
        <Badge className={`text-xs ${getOperatorColor(tree.operator || '')}`}>
          {formatOperatorName(tree.operator || '')}
        </Badge>
        <code className="text-xs bg-muted px-1 py-0.5 rounded max-w-[100px] truncate">
          "{tree.value}"
        </code>
      </div>
    );
  }

  if (tree.type === 'group' && tree.children) {
    const firstCondition = tree.children.find((child) => child.type === 'condition');
    const totalConditions =
      tree.children.filter((child) => child.type === 'condition').length +
      tree.children
        .filter((child) => child.type === 'group')
        .reduce((acc, group) => acc + (group.children?.length || 0), 0);

    return (
      <div className="flex items-center gap-1 flex-wrap">
        <Badge className={`text-xs ${getOperatorColor(tree.operator || 'and')}`}>
          {formatOperatorName(tree.operator || 'AND')}
        </Badge>
        {firstCondition && (
          <>
            <Badge variant="outline" className="text-xs font-mono">
              {firstCondition.field}
            </Badge>
            <Badge className={`text-xs ${getOperatorColor(firstCondition.operator || '')}`}>
              {formatOperatorName(firstCondition.operator || '')}
            </Badge>
          </>
        )}
        {totalConditions > 1 && (
          <span className="text-xs text-muted-foreground">+{totalConditions - 1} more</span>
        )}
      </div>
    );
  }

  return (
    <div className="flex items-center gap-1">
      <FilterIcon className="h-3 w-3 text-muted-foreground" />
      <span className="text-xs text-muted-foreground">Empty filter</span>
    </div>
  );
}
