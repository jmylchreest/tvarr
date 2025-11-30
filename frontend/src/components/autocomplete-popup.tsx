'use client';

import { cn } from '@/lib/utils';
import { Code } from '@/components/ui/code';
import { Badge } from '@/components/ui/badge';
import { Loader2 } from 'lucide-react';
import type { AutocompleteSuggestion, AutocompleteState } from '@/hooks/useHelperAutocomplete';

export interface AutocompletePopupProps {
  state: AutocompleteState;
  onSuggestionClick: (suggestion: AutocompleteSuggestion) => void;
  className?: string;
}

export function AutocompletePopup({ state, onSuggestionClick, className }: AutocompletePopupProps) {
  if (!state.isOpen) return null;

  const { suggestions, selectedIndex, position, loading } = state;

  return (
    <div
      data-autocomplete-popup="true"
      className={cn(
        'fixed z-50 min-w-[200px] max-w-[400px] bg-popover border border-border rounded-md shadow-lg',
        'animate-in fade-in-0 zoom-in-95 duration-100',
        className
      )}
      style={{
        left: Math.min(position.x, window.innerWidth - 220), // Prevent overflow
        top: Math.min(position.y + 5, window.innerHeight - 200), // Prevent overflow with offset
      }}
    >
      {loading ? (
        <div className="p-3 flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading suggestions...
        </div>
      ) : suggestions.length === 0 ? (
        <div className="p-3 text-sm text-muted-foreground">No suggestions found</div>
      ) : (
        <div className="py-1 max-h-[300px] overflow-y-auto">
          {suggestions.map((suggestion, index) => (
            <div
              key={`${suggestion.type}-${suggestion.value}-${index}`}
              className={cn(
                'px-3 py-2 cursor-pointer text-sm transition-colors',
                'hover:bg-accent hover:text-accent-foreground',
                index === selectedIndex && 'bg-accent text-accent-foreground'
              )}
              onClick={() => onSuggestionClick(suggestion)}
              onMouseEnter={() => {
                // Update selected index on hover for keyboard navigation consistency
                // This could be handled by passing a callback to update selectedIndex
              }}
            >
              <div className="flex items-center gap-2">
                <Code variant="muted" size="sm" className="font-mono">
                  {suggestion.label}
                </Code>
                {suggestion.type === 'helper' && (
                  <Badge variant="outline" className="text-xs">
                    helper
                  </Badge>
                )}
              </div>

              {suggestion.preview && (
                <div className="mt-2">
                  {suggestion.preview.startsWith('http') &&
                  (suggestion.preview.includes('/logos/') ||
                    suggestion.preview.includes('/images/')) ? (
                    <img
                      src={suggestion.preview}
                      alt={suggestion.label}
                      className="w-full object-contain border border-border rounded bg-muted"
                      onError={(e) => {
                        // Fallback to text preview if image fails
                        (e.target as HTMLImageElement).style.display = 'none';
                        const textPreview = document.createElement('div');
                        textPreview.className = 'text-xs text-blue-600 font-mono';
                        textPreview.textContent = `Preview: ${suggestion.preview}`;
                        const target = e.target as HTMLImageElement;
                        target.parentNode?.appendChild(textPreview);
                      }}
                    />
                  ) : (
                    <div className="text-xs text-blue-600 font-mono">
                      Preview: {suggestion.preview}
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Footer with keyboard hints */}
      <div className="border-t border-border px-3 py-2 text-xs text-muted-foreground bg-muted/50">
        <div className="flex items-center gap-4">
          <span>↑↓ Navigate</span>
          <span>Tab Complete</span>
          <span>Esc Close</span>
        </div>
      </div>
    </div>
  );
}
