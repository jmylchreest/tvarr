'use client';

import { FilterExpressionEditor } from '@/components/filter-expression-editor';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Settings, HelpCircle, Type, Lightbulb } from 'lucide-react';

interface FilterEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

const OPERATORS = [
  {
    name: 'contains',
    description: 'Text contains substring (case insensitive)',
    example: 'channel_name contains "sport"',
  },
  {
    name: 'not_contains',
    description: 'Text does not contain substring',
    example: 'channel_name not_contains "adult"',
  },
  {
    name: 'equals',
    description: 'Exact text match (case insensitive)',
    example: 'group_title equals "Sports"',
  },
  {
    name: 'not_equals',
    description: 'Text does not equal',
    example: 'group_title not_equals "Adult"',
  },
  {
    name: 'matches',
    description: 'Regular expression match (Rust regex syntax)',
    example: 'channel_name matches "(hd|4k|uhd)"',
  },
  {
    name: 'not_matches',
    description: 'Does not match regex pattern',
    example: 'channel_name not_matches "test.*"',
  },
  {
    name: 'starts_with',
    description: 'Text starts with substring',
    example: 'channel_name starts_with "US"',
  },
  {
    name: 'ends_with',
    description: 'Text ends with substring',
    example: 'channel_name ends_with "HD"',
  },
];

const MODIFIERS = [
  { name: 'not', description: 'Negates the operator', example: 'not channel_name contains "test"' },
  {
    name: 'case_sensitive',
    description: 'Makes the match case sensitive',
    example: 'channel_name case_sensitive contains "BBC"',
  },
];

const LOGIC_OPERATORS = [
  {
    name: 'AND',
    description: 'All conditions must match',
    example: 'channel_name contains "news" AND group_title equals "International"',
  },
  {
    name: 'OR',
    description: 'Any condition can match',
    example: 'group_title contains "sport" OR channel_name contains "football"',
  },
];

const EXAMPLES = [
  'channel_name contains "sport"',
  'group_title equals "Sports"',
  'channel_name matches "(hd|4k|uhd)"',
  'channel_name not_contains "adult" AND group_title not_contains "xxx"',
  'channel_name case_sensitive contains "BBC"',
  'not channel_name contains "test"',
  'channel_name starts_with "US" AND channel_name ends_with "HD"',
  '(channel_name contains "Sport" OR channel_name contains "Football") AND group_title not_contains "Adult"',
];

function SyntaxHelp() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
          <HelpCircle className="h-3 w-3" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 popover-backdrop" side="top">
        <div className="space-y-4">
          <div>
            <h4 className="font-medium mb-2 flex items-center gap-2">
              <Settings className="h-4 w-4" />
              Operators
            </h4>
            <div className="space-y-1 text-xs">
              {OPERATORS.slice(0, 4).map((op) => (
                <div key={op.name} className="flex items-center gap-2">
                  <Badge variant="outline" className="text-xs">
                    {op.name}
                  </Badge>
                  <span className="text-muted-foreground">{op.description}</span>
                </div>
              ))}
            </div>
          </div>

          <div>
            <h4 className="font-medium mb-2 flex items-center gap-2">
              <Type className="h-4 w-4" />
              Logic
            </h4>
            <div className="space-y-1 text-xs">
              {LOGIC_OPERATORS.map((op) => (
                <div key={op.name} className="flex items-center gap-2">
                  <Badge variant="outline" className="text-xs">
                    {op.name}
                  </Badge>
                  <span className="text-muted-foreground">{op.description}</span>
                </div>
              ))}
            </div>
          </div>

          <div>
            <h4 className="font-medium mb-2 flex items-center gap-2">
              <Lightbulb className="h-4 w-4" />
              Examples
            </h4>
            <div className="space-y-1 text-xs font-mono">
              {EXAMPLES.slice(0, 3).map((example, idx) => (
                <div key={idx} className="p-1 bg-muted rounded text-xs">
                  {example}
                </div>
              ))}
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function FilterEditor({
  value,
  onChange,
  placeholder = 'Enter filter expression...',
  disabled = false,
  className = '',
}: FilterEditorProps) {
  return (
    <div className={`space-y-4 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Filter Expression</span>
        <SyntaxHelp />
      </div>

      {/* Rich Filter Expression Editor */}
      <FilterExpressionEditor
        value={value}
        onChange={onChange}
        sourceType="stream"
        placeholder={placeholder}
        disabled={disabled}
        showTestResults={true}
        autoTest={true}
      />
    </div>
  );
}
