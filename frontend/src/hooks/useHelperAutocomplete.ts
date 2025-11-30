import { useState, useEffect, useCallback, useRef } from 'react';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';

export interface Helper {
  name: string;
  prefix: string;
  description: string;
  example: string;
  completion?: {
    type: 'search' | 'static' | 'function';
    endpoint?: string;
    query_param?: string;
    display_field?: string;
    value_field?: string;
    preview_field?: string;
    min_chars?: number;
    debounce_ms?: number;
    max_results?: number;
    placeholder?: string;
    empty_message?: string;
    options?: Array<{
      label: string;
      value: string;
      description?: string;
    }>;
    context_fields?: string[];
  };
}

export interface AutocompleteSuggestion {
  label: string;
  value: string;
  description?: string;
  preview?: string;
  type: 'helper' | 'completion';
}

export interface AutocompleteContext {
  type: 'helper' | 'completion';
  helper?: Helper;
  query: string;
  startPos: number;
  endPos: number;
}

export interface AutocompleteState {
  isOpen: boolean;
  suggestions: AutocompleteSuggestion[];
  selectedIndex: number;
  position: { x: number; y: number };
  context?: AutocompleteContext;
  loading: boolean;
}

export function useHelperAutocomplete(
  textareaRef: React.RefObject<HTMLTextAreaElement>,
  helpers: Helper[],
  onValueChange?: (value: string) => void
) {
  const [state, setState] = useState<AutocompleteState>({
    isOpen: false,
    suggestions: [],
    selectedIndex: 0,
    position: { x: 0, y: 0 },
    loading: false,
  });

  const debounceTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Calculate cursor position for popup
  const getCursorPosition = useCallback((textarea: HTMLTextAreaElement) => {
    const { selectionStart } = textarea;
    const rect = textarea.getBoundingClientRect();
    const style = window.getComputedStyle(textarea);

    // Create a hidden div to measure text position
    const div = document.createElement('div');
    div.style.position = 'absolute';
    div.style.visibility = 'hidden';
    div.style.whiteSpace = 'pre-wrap';
    div.style.wordWrap = 'break-word';
    div.style.font = style.font;
    div.style.padding = style.padding;
    div.style.border = style.border;
    div.style.width = style.width;
    div.style.lineHeight = style.lineHeight;

    const textBeforeCursor = textarea.value.substring(0, selectionStart);
    div.textContent = textBeforeCursor;

    document.body.appendChild(div);
    const divRect = div.getBoundingClientRect();
    document.body.removeChild(div);

    return {
      x: rect.left + divRect.width + parseInt(style.paddingLeft, 10),
      y: rect.top + divRect.height + parseInt(style.paddingTop, 10),
    };
  }, []);

  // Detect autocomplete context from cursor position
  const getAutocompleteContext = useCallback(
    (textarea: HTMLTextAreaElement): AutocompleteContext | null => {
      const { selectionStart } = textarea;
      const text = textarea.value;
      const beforeCursor = text.substring(0, selectionStart);

      // Find the last @ symbol
      const lastAtPos = beforeCursor.lastIndexOf('@');
      if (lastAtPos === -1) return null;

      // Check if we're in a helper context
      const afterAt = beforeCursor.substring(lastAtPos);
      const colonPos = afterAt.indexOf(':');

      if (colonPos === -1) {
        // We're typing helper name: @hel...
        const query = afterAt.substring(1); // Remove @
        return {
          type: 'helper',
          query,
          startPos: lastAtPos,
          endPos: selectionStart,
        };
      } else {
        // We're typing completion: @logo:spo...
        const helperName = afterAt.substring(1, colonPos);
        const helper = helpers.find((h) => h.name === helperName);
        if (!helper) return null;

        const query = afterAt.substring(colonPos + 1);
        return {
          type: 'completion',
          helper,
          query,
          startPos: lastAtPos + colonPos + 1,
          endPos: selectionStart,
        };
      }
    },
    [helpers]
  );

  // Get helper suggestions
  const getHelperSuggestions = useCallback(
    (query: string): AutocompleteSuggestion[] => {
      Debug.log('AutoComplete: getHelperSuggestions called', {
        query,
        helpersCount: helpers.length,
      });
      const suggestions = helpers
        .filter((helper) => helper.name.toLowerCase().includes(query.toLowerCase()))
        .map((helper) => ({
          label: helper.prefix,
          value: helper.prefix,
          description: helper.description,
          type: 'helper' as const,
        }));
      Debug.log('AutoComplete: helper suggestions generated', suggestions);
      return suggestions;
    },
    [helpers]
  );

  // Get completion suggestions
  const getCompletionSuggestions = useCallback(
    async (
      helper: Helper,
      query: string,
      signal?: AbortSignal
    ): Promise<AutocompleteSuggestion[]> => {
      Debug.log('AutoComplete: getCompletionSuggestions called', {
        helper: helper.name,
        query,
        completion: helper.completion,
      });

      if (!helper.completion) {
        Debug.log('AutoComplete: No completion config for helper');
        return [];
      }

      const { completion } = helper;

      if (completion.type === 'static') {
        Debug.log('AutoComplete: Static completion with options', completion.options);
        return (completion.options || [])
          .filter((option) => option.label.toLowerCase().includes(query.toLowerCase()))
          .map((option) => ({
            label: option.label,
            value: option.value,
            description: option.description,
            type: 'completion' as const,
          }));
      }

      if (completion.type === 'search' && completion.endpoint) {
        const minChars = completion.min_chars || 1;
        Debug.log('AutoComplete: Search completion', {
          query,
          minChars,
          queryLength: query.length,
        });

        if (query.length < minChars) {
          Debug.log('AutoComplete: Query too short, need', minChars, 'chars');
          return [];
        }

        try {
          const backendUrl = getBackendUrl();
          // Remove the full URL from the endpoint if it's already there
          const endpointPath = completion.endpoint.replace(/^https?:\/\/[^/]+/, '');
          const url = new URL(`${backendUrl}${endpointPath}`);
          url.searchParams.set(completion.query_param || 'q', query);

          if (completion.max_results) {
            url.searchParams.set('limit', completion.max_results.toString());
          }

          Debug.log('AutoComplete: Fetching from URL', url.toString());
          const response = await fetch(url.toString(), { signal });

          if (!response.ok) {
            Debug.log('AutoComplete: Search API failed', response.status, response.statusText);
            return [];
          }

          const data = await response.json();
          Debug.log('AutoComplete: Search API response', data);

          const items = Array.isArray(data) ? data : data.results || data.items || data.data || [];
          Debug.log('AutoComplete: Extracted items', items);

          const suggestions = items.map((item: any) => ({
            label: item[completion.display_field || 'name'] || item.name,
            value: item[completion.value_field || 'id'] || item.id,
            description: item.description?.replace(/^Logo asset:\s*/, ''), // Clean up description
            preview: completion.preview_field ? item[completion.preview_field] : undefined,
            type: 'completion' as const,
          }));

          Debug.log('AutoComplete: Generated suggestions', suggestions);
          return suggestions;
        } catch (error) {
          if (error instanceof Error && error.name === 'AbortError') {
            Debug.log('AutoComplete: Search request aborted');
            return [];
          }
          console.error('AutoComplete: Search API error', error);
          return [];
        }
      }

      if (completion.type === 'function') {
        Debug.log('AutoComplete: Function completion - generating dynamic suggestions');
        // For function type completions, we could generate suggestions based on context
        // This could involve calling a function to get dynamic suggestions
        // For now, return empty array as function completions might not need suggestions
        return [];
      }

      Debug.log('AutoComplete: Unhandled completion type', completion.type);
      return [];
    },
    []
  );

  // Update suggestions based on context
  const updateSuggestions = useCallback(
    async (context: AutocompleteContext) => {
      Debug.log('AutoComplete: updateSuggestions called', context);
      setState((prev) => ({ ...prev, loading: true }));

      // Cancel previous request
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      const abortController = new AbortController();
      abortControllerRef.current = abortController;

      try {
        let suggestions: AutocompleteSuggestion[] = [];

        if (context.type === 'helper') {
          Debug.log('AutoComplete: Getting helper suggestions');
          suggestions = getHelperSuggestions(context.query);
        } else if (context.type === 'completion' && context.helper) {
          Debug.log('AutoComplete: Getting completion suggestions for helper', context.helper.name);
          suggestions = await getCompletionSuggestions(
            context.helper,
            context.query,
            abortController.signal
          );
        }

        Debug.log('AutoComplete: Final suggestions', suggestions);

        if (!abortController.signal.aborted) {
          setState((prev) => ({
            ...prev,
            suggestions,
            selectedIndex: 0,
            loading: false,
            context,
          }));
        }
      } catch (error) {
        console.error('AutoComplete: Error in updateSuggestions', error);
        if (!abortController.signal.aborted) {
          setState((prev) => ({ ...prev, loading: false, suggestions: [] }));
        }
      }
    },
    [getHelperSuggestions, getCompletionSuggestions]
  );

  // Handle input changes
  const handleInputChange = useCallback(() => {
    Debug.log('AutoComplete: handleInputChange called');
    const textarea = textareaRef.current;
    if (!textarea) {
      Debug.log('AutoComplete: No textarea ref');
      return;
    }

    const context = getAutocompleteContext(textarea);
    Debug.log('AutoComplete: context detected', context);

    if (!context) {
      setState((prev) => ({ ...prev, isOpen: false, suggestions: [], context: undefined }));
      return;
    }

    const position = getCursorPosition(textarea);
    Debug.log('📍 AutoComplete: position calculated', position);
    setState((prev) => ({ ...prev, isOpen: true, position }));

    // Debounce API calls for completions
    if (debounceTimeoutRef.current) {
      clearTimeout(debounceTimeoutRef.current);
    }

    const delay = (context.type === 'completion' && context.helper?.completion?.debounce_ms) || 300;
    debounceTimeoutRef.current = setTimeout(() => {
      updateSuggestions(context);
    }, delay);
  }, [textareaRef, getAutocompleteContext, getCursorPosition, updateSuggestions]);

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
      Debug.log('AutoComplete: Key pressed', {
        key: event.key,
        isOpen: state.isOpen,
        suggestionsCount: state.suggestions.length,
        selectedIndex: state.selectedIndex,
      });

      if (!state.isOpen || state.suggestions.length === 0) {
        Debug.log('AutoComplete: Early return - not open or no suggestions');
        return;
      }

      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault();
          setState((prev) => ({
            ...prev,
            selectedIndex: Math.min(prev.selectedIndex + 1, prev.suggestions.length - 1),
          }));
          break;

        case 'ArrowUp':
          event.preventDefault();
          setState((prev) => ({
            ...prev,
            selectedIndex: Math.max(prev.selectedIndex - 1, 0),
          }));
          break;

        case 'Tab':
        case 'Enter':
          event.preventDefault();
          Debug.log('AutoComplete: Tab/Enter pressed for completion');
          const selectedSuggestion = state.suggestions[state.selectedIndex];
          Debug.log('AutoComplete: Selected suggestion', selectedSuggestion);
          Debug.log('📍 AutoComplete: Current context', state.context);
          if (selectedSuggestion && state.context) {
            Debug.log('AutoComplete: Calling insertCompletion');
            insertCompletion(selectedSuggestion, state.context);
          } else {
            Debug.log('AutoComplete: Missing suggestion or context');
          }
          break;

        case 'Escape':
          setState((prev) => ({ ...prev, isOpen: false, suggestions: [] }));
          break;
      }
    },
    [state]
  );

  // Insert completion at cursor
  const insertCompletion = useCallback(
    (suggestion: AutocompleteSuggestion, context: AutocompleteContext) => {
      const textarea = textareaRef.current;
      if (!textarea || !onValueChange) {
        Debug.log('AutoComplete: Missing textarea or onValueChange callback');
        return;
      }

      const { value } = textarea;
      const beforeReplacement = value.substring(0, context.startPos);
      const afterReplacement = value.substring(context.endPos);

      let newValue: string;
      if (context.type === 'helper') {
        // Replace @query with @helper: (e.g., @lo -> @logo:)
        newValue = beforeReplacement + suggestion.value + afterReplacement;
      } else {
        // Replace query with completion value (e.g., @logo:sp -> @logo:uuid)
        // For completions, we only replace the query part after the colon
        newValue = beforeReplacement + suggestion.value + afterReplacement;
      }

      Debug.log('AutoComplete: Inserting completion', {
        contextType: context.type,
        beforeReplacement,
        suggestionValue: suggestion.value,
        suggestionLabel: suggestion.label,
        afterReplacement,
        newValue,
        startPos: context.startPos,
        endPos: context.endPos,
        originalValue: value,
      });

      // Use React's controlled component pattern
      onValueChange(newValue);

      // Set cursor position after React updates the value
      setTimeout(() => {
        if (textarea) {
          const newCursorPos = beforeReplacement.length + suggestion.value.length;
          textarea.setSelectionRange(newCursorPos, newCursorPos);
          textarea.focus();
        }
      }, 0);

      // Close autocomplete
      setState((prev) => ({ ...prev, isOpen: false, suggestions: [] }));
    },
    [textareaRef, onValueChange]
  );

  // Handle suggestion click
  const handleSuggestionClick = useCallback(
    (suggestion: AutocompleteSuggestion) => {
      Debug.log('AutoComplete: Mouse click on suggestion', {
        suggestion,
        hasContext: !!state.context,
      });
      if (state.context) {
        insertCompletion(suggestion, state.context);
      } else {
        Debug.log('AutoComplete: No context available for mouse click');
      }
    },
    [state.context, insertCompletion]
  );

  // Close on outside click
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        state.isOpen &&
        textareaRef.current &&
        !textareaRef.current.contains(event.target as Node)
      ) {
        // Check if the click was inside the autocomplete popup
        const target = event.target as Element;
        const popup = target.closest('[data-autocomplete-popup="true"]');
        if (!popup) {
          setState((prev) => ({ ...prev, isOpen: false }));
        }
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [state.isOpen, textareaRef]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current);
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  return {
    state,
    handlers: {
      onInputChange: handleInputChange,
      onKeyDown: handleKeyDown,
      onSuggestionClick: handleSuggestionClick,
    },
  };
}
