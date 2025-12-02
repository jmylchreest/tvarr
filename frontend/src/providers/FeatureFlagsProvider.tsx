'use client';

import React, { createContext, useContext, useEffect, useState } from 'react';
import { useFeatureFlags, invalidateFeatureFlagsCache } from '@/hooks/useFeatureFlags';
import { Debug } from '@/utils/debug';

interface FeatureFlagsContextType {
  isLoaded: boolean;
  invalidateCache: () => void;
}

const FeatureFlagsContext = createContext<FeatureFlagsContextType>({
  isLoaded: false,
  invalidateCache: () => {},
});

interface FeatureFlagsProviderProps {
  children: React.ReactNode;
}

export function FeatureFlagsProvider({ children }: FeatureFlagsProviderProps) {
  const [isLoaded, setIsLoaded] = useState(false);
  const { featureFlags, isLoading, error } = useFeatureFlags();

  // Mark as loaded when feature flags have been fetched (even if empty or error)
  useEffect(() => {
    if (!isLoading) {
      setIsLoaded(true);
    }
  }, [isLoading]);

  const invalidateCache = () => {
    invalidateFeatureFlagsCache();
    setIsLoaded(false);
  };

  // Log feature flags state for debugging
  useEffect(() => {
    if (isLoaded) {
      Debug.log('[FeatureFlags] Loaded:', Object.keys(featureFlags).length, 'flags');
      if (error) {
        console.warn('[FeatureFlags] Error loading flags:', error);
      }
    }
  }, [isLoaded, featureFlags, error]);

  return (
    <FeatureFlagsContext.Provider value={{ isLoaded, invalidateCache }}>
      {children}
    </FeatureFlagsContext.Provider>
  );
}

export function useFeatureFlagsContext() {
  return useContext(FeatureFlagsContext);
}
