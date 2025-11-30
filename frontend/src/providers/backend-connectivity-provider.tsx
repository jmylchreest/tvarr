'use client';

import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from 'react';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';

export interface BackendConnectivityState {
  isConnected: boolean;
  isChecking: boolean;
  lastChecked: Date | null;
  error: string | null;
  backendUrl: string;
  checkConnection: () => Promise<void>;
}

const BackendConnectivityContext = createContext<BackendConnectivityState | undefined>(undefined);

export function useBackendConnectivity() {
  const context = useContext(BackendConnectivityContext);
  if (context === undefined) {
    throw new Error('useBackendConnectivity must be used within a BackendConnectivityProvider');
  }
  return context;
}

interface BackendConnectivityProviderProps {
  children: React.ReactNode;
}

export function BackendConnectivityProvider({ children }: BackendConnectivityProviderProps) {
  const [isConnected, setIsConnected] = useState(false);
  const [isChecking, setIsChecking] = useState(false);
  const [lastChecked, setLastChecked] = useState<Date | null>(null);
  const [error, setError] = useState<string | null>(null);

  const backendUrl = getBackendUrl();

  // Track ongoing request to prevent overlaps
  const activeRequestRef = useRef<AbortController | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const checkConnection = useCallback(async () => {
    // Cancel any ongoing request first
    if (activeRequestRef.current) {
      activeRequestRef.current.abort();
      activeRequestRef.current = null;
    }

    setIsChecking(true);
    setError(null);

    try {
      // Simple logging without feature flag dependency to prevent circular dependency
      Debug.log('[BackendConnectivity] Checking connectivity to:', backendUrl);

      // Use the /live endpoint as it's a simple health check
      const controller = new AbortController();
      activeRequestRef.current = controller;
      const timeoutId = setTimeout(() => controller.abort(), 10000); // 10 second timeout

      const response = await fetch(`${backendUrl}/live`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
        signal: controller.signal,
      });

      clearTimeout(timeoutId);
      activeRequestRef.current = null;

      if (response.ok) {
        Debug.log('[BackendConnectivity] Connection successful');
        setIsConnected(true);
        setError(null);
      } else {
        throw new Error(`Backend returned ${response.status}: ${response.statusText}`);
      }
    } catch (err) {
      activeRequestRef.current = null;

      // Don't log aborted requests as errors - they're intentional cancellations
      if (err instanceof Error && err.name === 'AbortError') {
        Debug.log('[BackendConnectivity] Connection check aborted');
        return; // Don't update state for aborted requests
      }

      console.error('[Backend] Connection failed:', err); // Keep as console.error - critical for production
      setIsConnected(false);

      if (err instanceof Error) {
        if (err.message.includes('fetch')) {
          setError('Network error - unable to reach backend service');
        } else {
          setError(err.message);
        }
      } else {
        setError('Unknown connection error');
      }
    } finally {
      setIsChecking(false);
      setLastChecked(new Date());
    }
  }, [backendUrl]);

  // Start monitoring ONLY after initial check is done and we're connected
  const startMonitoring = useCallback(() => {
    // Clear any existing interval
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    // Only start monitoring if we're connected
    if (isConnected && !isChecking) {
      Debug.log('[BackendConnectivity] Starting periodic health checks (60s interval)');
      intervalRef.current = setInterval(() => {
        Debug.log('[BackendConnectivity] Performing periodic health check');
        checkConnection();
      }, 60000); // 60 seconds
    }
  }, [isConnected, isChecking]); // Remove checkConnection dependency to prevent circular loop

  // Stop monitoring
  const stopMonitoring = useCallback(() => {
    if (intervalRef.current) {
      Debug.log('[BackendConnectivity] Stopping periodic health checks');
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  // Initial connection check - run once on mount
  useEffect(() => {
    Debug.log('[BackendConnectivity] Running initial connection check');
    checkConnection();
  }, []); // Empty dependency array to run only once on mount

  // Start/stop monitoring based on connection state
  useEffect(() => {
    if (isConnected && !isChecking) {
      startMonitoring();
    } else {
      stopMonitoring();
    }

    return () => {
      stopMonitoring();
    };
  }, [isConnected, isChecking, startMonitoring, stopMonitoring]); // Include functions now that they're stable

  // Cleanup active requests on unmount
  useEffect(() => {
    return () => {
      Debug.log('[BackendConnectivity] Cleaning up on unmount');
      if (activeRequestRef.current) {
        activeRequestRef.current.abort();
        activeRequestRef.current = null;
      }
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, []);

  const contextValue: BackendConnectivityState = {
    isConnected,
    isChecking,
    lastChecked,
    error,
    backendUrl,
    checkConnection,
  };

  return (
    <BackendConnectivityContext.Provider value={contextValue}>
      {children}
    </BackendConnectivityContext.Provider>
  );
}
