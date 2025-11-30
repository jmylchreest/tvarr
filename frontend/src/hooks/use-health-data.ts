'use client';

import { useState, useEffect } from 'react';
import { getBackendUrl } from '@/lib/config';
import { HealthData } from '@/types/api';

export function useHealthData(refreshInterval: number = 30000) {
  const [healthData, setHealthData] = useState<HealthData | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchHealthData = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/health`);
        if (response.ok) {
          // The health endpoint returns data wrapped in ApiResponse format
          const apiResponse = await response.json();
          if (apiResponse.success && apiResponse.data) {
            setHealthData(apiResponse.data);
          }
        }
      } catch (error) {
        console.warn('Failed to fetch health data from health endpoint:', error);
        // Keep fallback data
      } finally {
        setIsLoading(false);
      }
    };

    fetchHealthData();

    // Refresh health status at specified interval (if not disabled)
    if (refreshInterval > 0) {
      const interval = setInterval(fetchHealthData, refreshInterval);
      return () => clearInterval(interval);
    }
  }, [refreshInterval]);

  return { healthData, isLoading };
}
