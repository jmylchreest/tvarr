// Status colour utility that uses theme-aware colours
// This ensures consistent styling across all components

export type StatusType = 'success' | 'warning' | 'error' | 'info' | 'neutral';

export interface StatusColourConfig {
  bg: string;
  text: string;
  border: string;
}

// Theme-aware status colour mappings
export const statusColors: Record<StatusType, StatusColourConfig> = {
  success: {
    bg: 'bg-green-50 dark:bg-green-950/20',
    text: 'text-green-700 dark:text-green-300',
    border: 'border-green-200 dark:border-green-800',
  },
  warning: {
    bg: 'bg-amber-50 dark:bg-amber-950/20',
    text: 'text-amber-700 dark:text-amber-300',
    border: 'border-amber-200 dark:border-amber-800',
  },
  error: {
    bg: 'bg-destructive/10',
    text: 'text-destructive',
    border: 'border-destructive/20',
  },
  info: {
    bg: 'bg-blue-50 dark:bg-blue-950/20',
    text: 'text-blue-700 dark:text-blue-300',
    border: 'border-blue-200 dark:border-blue-800',
  },
  neutral: {
    bg: 'bg-muted',
    text: 'text-muted-foreground',
    border: 'border-muted',
  },
};

// Utility function to get status colours based on status string
export function getStatusType(status: string | undefined | null): StatusType {
  if (!status || typeof status !== 'string') {
    return 'neutral';
  }

  const normalizedStatus = status.toLowerCase();

  if (
    ['connected', 'running', 'healthy', 'active', 'success', 'completed'].includes(normalizedStatus)
  ) {
    return 'success';
  }

  if (['buffering', 'pending', 'warning', 'limited'].includes(normalizedStatus)) {
    return 'warning';
  }

  if (['error', 'failed', 'disconnected', 'unhealthy', 'timeout'].includes(normalizedStatus)) {
    return 'error';
  }

  if (['info', 'connecting', 'starting', 'loading'].includes(normalizedStatus)) {
    return 'info';
  }

  return 'neutral';
}

// Get status badge classes
export function getStatusBadgeClasses(status: string | undefined | null): string {
  const statusType = getStatusType(status);
  const colours = statusColors[statusType];

  return `${colours.bg} ${colours.text} ${colours.border} border`;
}

// Get status indicator (dot) classes
export function getStatusIndicatorClasses(status: string | undefined | null): string {
  const statusType = getStatusType(status);

  switch (statusType) {
    case 'success':
      return 'bg-green-500';
    case 'warning':
      return 'bg-amber-500';
    case 'error':
      return 'bg-destructive';
    case 'info':
      return 'bg-blue-500';
    default:
      return 'bg-muted-foreground';
  }
}

// Operator-specific colours for filter expressions
export function getOperatorBadgeClasses(operator: string | undefined | null): string {
  if (!operator || typeof operator !== 'string') {
    return (
      statusColors.neutral.bg +
      ' ' +
      statusColors.neutral.text +
      ' ' +
      statusColors.neutral.border +
      ' border'
    );
  }

  const normalizedOp = operator.toLowerCase();

  switch (normalizedOp) {
    case 'contains':
      return (
        statusColors.info.bg +
        ' ' +
        statusColors.info.text +
        ' ' +
        statusColors.info.border +
        ' border'
      );
    case 'equals':
      return (
        statusColors.success.bg +
        ' ' +
        statusColors.success.text +
        ' ' +
        statusColors.success.border +
        ' border'
      );
    case 'not_equals':
      return (
        statusColors.warning.bg +
        ' ' +
        statusColors.warning.text +
        ' ' +
        statusColors.warning.border +
        ' border'
      );
    case 'regex':
      return 'bg-purple-50 dark:bg-purple-950/20 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800 border';
    case 'in':
      return 'bg-cyan-50 dark:bg-cyan-950/20 text-cyan-700 dark:text-cyan-300 border-cyan-200 dark:border-cyan-800 border';
    case 'not_in':
      return (
        statusColors.error.bg +
        ' ' +
        statusColors.error.text +
        ' ' +
        statusColors.error.border +
        ' border'
      );
    default:
      return (
        statusColors.neutral.bg +
        ' ' +
        statusColors.neutral.text +
        ' ' +
        statusColors.neutral.border +
        ' border'
      );
  }
}
