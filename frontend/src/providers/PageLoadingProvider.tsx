'use client';

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  ReactNode,
} from 'react';
import { usePathname } from 'next/navigation';

interface PageLoadingContextType {
  isLoading: boolean;
  setIsLoading: (loading: boolean) => void;
  startLoading: () => void;
  stopLoading: () => void;
}

const PageLoadingContext = createContext<PageLoadingContextType | null>(null);

export function usePageLoading() {
  const context = useContext(PageLoadingContext);
  if (!context) {
    throw new Error('usePageLoading must be used within a PageLoadingProvider');
  }
  return context;
}

interface PageLoadingProviderProps {
  children: ReactNode;
}

export function PageLoadingProvider({ children }: PageLoadingProviderProps) {
  const [isLoading, setIsLoadingState] = useState(false);
  const pathname = usePathname();

  // No auto-loading on route changes - components handle their own loading states
  useEffect(() => {
    setIsLoadingState(false);
  }, [pathname]);

  const setIsLoading = useCallback((loading: boolean) => {
    setIsLoadingState(loading);
  }, []);

  const startLoading = useCallback(() => {
    setIsLoadingState(true);
  }, []);

  const stopLoading = useCallback(() => {
    setIsLoadingState(false);
  }, []);

  const contextValue: PageLoadingContextType = {
    isLoading,
    setIsLoading,
    startLoading,
    stopLoading,
  };

  return <PageLoadingContext.Provider value={contextValue}>{children}</PageLoadingContext.Provider>;
}
