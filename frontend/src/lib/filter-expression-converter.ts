// Utility functions for converting between human-readable expressions and condition trees

export interface ConditionTreeNode {
  type: 'condition' | 'group';
  field?: string;
  operator?: string;
  value?: string;
  case_sensitive?: boolean;
  negate?: boolean;
  children?: ConditionTreeNode[];
}

export interface ConditionTree {
  root: ConditionTreeNode;
}

/**
 * Convert condition tree JSON string to human-readable expression
 */
export function conditionTreeToExpression(conditionTreeJson: string): string {
  try {
    const tree: ConditionTree = JSON.parse(conditionTreeJson);
    return nodeToExpression(tree.root);
  } catch (error) {
    console.warn('Failed to parse condition tree:', error);
    return '';
  }
}

/**
 * Convert a condition tree node to human-readable expression
 */
function nodeToExpression(node: ConditionTreeNode): string {
  if (!node) return '';

  if (node.type === 'condition') {
    const field = node.field || '';
    const operator = formatOperatorForExpression(node.operator || '');
    const value = node.value || '';
    const caseSensitive = node.case_sensitive ? 'case_sensitive ' : '';
    const negate = node.negate ? 'not ' : '';

    return `${negate}${caseSensitive}${field} ${operator} "${value}"`.trim();
  }

  if (node.type === 'group' && node.children && node.children.length > 0) {
    const operator = (node.operator || 'OR').toUpperCase();
    const childExpressions = node.children
      .map((child) => nodeToExpression(child))
      .filter((expr) => expr.length > 0);

    if (childExpressions.length === 0) return '';
    if (childExpressions.length === 1) return childExpressions[0];

    const joined = childExpressions.join(` ${operator} `);
    return `(${joined})`;
  }

  return '';
}

/**
 * Format operator from condition tree format to expression format
 */
function formatOperatorForExpression(operator: string): string {
  const normalized = operator.toLowerCase();
  switch (normalized) {
    case 'contains':
      return 'contains';
    case 'equals':
      return 'equals';
    case 'matches':
      return 'matches';
    case 'startswith':
      return 'starts_with';
    case 'endswith':
      return 'ends_with';
    default:
      return normalized;
  }
}

/**
 * Simple expression to condition tree conversion (fallback for client-side)
 * Note: The server validation endpoint should be the primary source of condition_tree
 */
export function expressionToConditionTree(expression: string): string {
  if (!expression.trim()) {
    return JSON.stringify({
      root: {
        type: 'condition',
        field: '',
        operator: '',
        value: '',
        case_sensitive: false,
        negate: false,
      },
    });
  }

  // Basic single condition pattern
  const singleConditionMatch = expression.match(
    /^(?:(not)\s+)?(?:(case_sensitive)\s+)?(\w+)\s+(contains|equals|matches|starts_with|ends_with)\s+"([^"]*)"\s*$/i
  );

  if (singleConditionMatch) {
    const [, negate, caseSensitive, field, operator, value] = singleConditionMatch;
    return JSON.stringify({
      root: {
        type: 'condition',
        field: field.trim(),
        operator: operator.toLowerCase().replace('_', ''),
        value: value,
        case_sensitive: !!caseSensitive,
        negate: !!negate,
      },
    });
  }

  // For complex expressions, return a placeholder
  // The server should handle the actual parsing
  return JSON.stringify({
    root: {
      type: 'condition',
      field: 'channel_name',
      operator: 'contains',
      value: expression,
      case_sensitive: false,
      negate: false,
    },
  });
}

/**
 * Get a human-readable summary of a condition tree for display purposes
 */
export function getConditionTreeSummary(conditionTreeJson: string): string {
  try {
    const tree: ConditionTree = JSON.parse(conditionTreeJson);
    return getNodeSummary(tree.root);
  } catch (error) {
    return 'Invalid condition tree';
  }
}

function getNodeSummary(node: ConditionTreeNode): string {
  if (!node) return 'Empty';

  if (node.type === 'condition') {
    const field = node.field || 'field';
    const operator = node.operator || 'operator';
    const value = node.value || 'value';
    const negate = node.negate ? 'NOT ' : '';
    const caseSensitive = node.case_sensitive ? ' (case sensitive)' : '';

    return `${negate}${field} ${operator} "${value}"${caseSensitive}`;
  }

  if (node.type === 'group' && node.children && node.children.length > 0) {
    const operator = (node.operator || 'OR').toUpperCase();
    const summaries = node.children.map(getNodeSummary).filter((s) => s !== 'Empty');

    if (summaries.length === 0) return 'Empty group';
    if (summaries.length === 1) return summaries[0];
    if (summaries.length <= 3) return summaries.join(` ${operator} `);

    return `${summaries.length} conditions with ${operator}`;
  }

  return 'Unknown node type';
}
