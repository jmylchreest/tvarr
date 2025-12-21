// Status colour utility that uses theme-aware colours
// This ensures consistent styling across all components

export type StatusType = 'success' | 'warning' | 'error' | 'info' | 'neutral';

export interface StatusColourConfig {
  bg: string;
  text: string;
  border: string;
}

// Theme-aware status colour mappings using semantic theme colors
export const statusColors: Record<StatusType, StatusColourConfig> = {
  success: {
    bg: 'bg-success/10',
    text: 'text-success',
    border: 'border-success/20',
  },
  warning: {
    bg: 'bg-warning/10',
    text: 'text-warning',
    border: 'border-warning/20',
  },
  error: {
    bg: 'bg-destructive/10',
    text: 'text-destructive',
    border: 'border-destructive/20',
  },
  info: {
    bg: 'bg-info/10',
    text: 'text-info',
    border: 'border-info/20',
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
      return 'bg-success';
    case 'warning':
      return 'bg-warning';
    case 'error':
      return 'bg-destructive';
    case 'info':
      return 'bg-info';
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
      // Uses accent color for special operators
      return 'bg-accent/10 text-accent-foreground border-accent/20 border';
    case 'in':
      // Uses info color for set operators
      return statusColors.info.bg + ' ' + statusColors.info.text + ' ' + statusColors.info.border + ' border';
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
