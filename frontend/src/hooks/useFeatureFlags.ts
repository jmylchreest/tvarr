import { useState, useEffect } from 'react';
import { getBackendUrl } from '@/lib/config';

interface FeatureFlags {
  [key: string]: boolean;
}

interface FeatureConfig {
  [key: string]: any;
}

interface FeatureConfigs {
  [featureName: string]: FeatureConfig;
}

interface FeaturesResponse {
  flags: FeatureFlags;
  config: FeatureConfigs;
  timestamp: string;
}

// Global feature flags and config cache
let featureFlagsCache: FeatureFlags = {};
let featureConfigsCache: FeatureConfigs = {};
let isCacheValid = false;
let cacheTimestamp = 0;
// Cache duration is controlled by server configuration

/**
 * Hook to fetch and manage feature flags from the dedicated features endpoint
 */
export function useFeatureFlags() {
  const [featureFlags, setFeatureFlags] = useState<FeatureFlags>(featureFlagsCache);
  const [featureConfigs, setFeatureConfigs] = useState<FeatureConfigs>(featureConfigsCache);
  const [isLoading, setIsLoading] = useState(!isCacheValid);
  const [error, setError] = useState<string | null>(null);

  const fetchFeatureFlags = async () => {
    try {
      setIsLoading(true);
      setError(null);

      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/features`);
      if (!response.ok) {
        throw new Error(`Failed to fetch feature flags: ${response.status}`);
      }

      const apiResponse = await response.json();
      if (!apiResponse.success || !apiResponse.data) {
        throw new Error('Invalid response format from features API');
      }

      const featuresData: FeaturesResponse = apiResponse.data;
      const flags = featuresData.flags || {};
      const configs = featuresData.config || {};

      // Update cache (always update cache data, but caching behavior is controlled by feature flag)
      featureFlagsCache = flags;
      featureConfigsCache = configs;

      // Only mark cache as valid if caching is enabled AND configured
      const isCacheEnabled = flags['feature-cache'] === true;
      const cacheDuration = configs['feature-cache']?.['cache-duration'];

      if (isCacheEnabled && cacheDuration) {
        isCacheValid = true;
        cacheTimestamp = Date.now();
      } else {
        // If caching is disabled or not configured, always mark cache as invalid
        isCacheValid = false;
        cacheTimestamp = 0;
      }

      setFeatureFlags(flags);
      setFeatureConfigs(configs);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch feature flags';
      setError(errorMessage);
      console.error('Error fetching feature flags:', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    // Check if caching is enabled via feature flag from server
    const isCacheEnabled = featureFlagsCache['feature-cache'] === true;
    const cacheDuration = featureConfigsCache['feature-cache']?.['cache-duration'];

    // Only use cache if caching is enabled AND cache duration is configured AND cache is valid
    if (isCacheEnabled && cacheDuration && isCacheValid) {
      const now = Date.now();
      if (now - cacheTimestamp < cacheDuration) {
        setFeatureFlags(featureFlagsCache);
        setFeatureConfigs(featureConfigsCache);
        setIsLoading(false);
        return;
      }
    }

    // Always fetch if caching is disabled, not configured, or cache is invalid
    fetchFeatureFlags();
  }, []);

  const refetch = () => {
    // Invalidate cache and refetch
    isCacheValid = false;
    fetchFeatureFlags();
  };

  return { featureFlags, featureConfigs, isLoading, error, refetch };
}

/**
 * Utility function to check if a specific feature is enabled
 * @param featureKey - The feature key to check
 * @param featureFlags - Optional feature flags object, if not provided will use cached values
 * @returns boolean - true if feature is enabled, false if disabled or not found
 */
export function isFeatureEnabled(featureKey: string, featureFlags?: FeatureFlags): boolean {
  const flags = featureFlags || featureFlagsCache;
  return flags[featureKey] === true; // Explicitly check for true, defaults to false if not found
}

/**
 * Utility function to get configuration for a specific feature
 * @param featureKey - The feature key to get config for
 * @param featureConfigs - Optional feature configs object, if not provided will use cached values
 * @returns FeatureConfig - configuration object for the feature, empty object if not found
 */
export function getFeatureConfig(
  featureKey: string,
  featureConfigs?: FeatureConfigs
): FeatureConfig {
  const configs = featureConfigs || featureConfigsCache;
  return configs[featureKey] || {}; // Return empty object if not found
}

/**
 * Utility function to get a specific config value for a feature
 * @param featureKey - The feature key
 * @param configKey - The config property key
 * @param featureConfigs - Optional feature configs object, if not provided will use cached values
 * @returns any - the config value, undefined if not found
 */
export function getFeatureConfigValue(
  featureKey: string,
  configKey: string,
  featureConfigs?: FeatureConfigs
): any {
  const config = getFeatureConfig(featureKey, featureConfigs);
  return config[configKey];
}

/**
 * Utility function to get a config value as a string
 * @param featureKey - The feature key
 * @param configKey - The config property key
 * @param defaultValue - Default value if not found or not a string
 * @param featureConfigs - Optional feature configs object
 * @returns string - the config value as string, or default value
 */
export function getFeatureConfigString(
  featureKey: string,
  configKey: string,
  defaultValue: string = '',
  featureConfigs?: FeatureConfigs
): string {
  const value = getFeatureConfigValue(featureKey, configKey, featureConfigs);
  return typeof value === 'string' ? value : defaultValue;
}

/**
 * Utility function to get a config value as a number
 * @param featureKey - The feature key
 * @param configKey - The config property key
 * @param defaultValue - Default value if not found or not a number
 * @param featureConfigs - Optional feature configs object
 * @returns number - the config value as number, or default value
 */
export function getFeatureConfigNumber(
  featureKey: string,
  configKey: string,
  defaultValue: number = 0,
  featureConfigs?: FeatureConfigs
): number {
  const value = getFeatureConfigValue(featureKey, configKey, featureConfigs);
  return typeof value === 'number' ? value : defaultValue;
}

/**
 * Utility function to get a config value as a boolean
 * @param featureKey - The feature key
 * @param configKey - The config property key
 * @param defaultValue - Default value if not found or not a boolean
 * @param featureConfigs - Optional feature configs object
 * @returns boolean - the config value as boolean, or default value
 */
export function getFeatureConfigBoolean(
  featureKey: string,
  configKey: string,
  defaultValue: boolean = false,
  featureConfigs?: FeatureConfigs
): boolean {
  const value = getFeatureConfigValue(featureKey, configKey, featureConfigs);
  return typeof value === 'boolean' ? value : defaultValue;
}

/**
 * Hook for checking a specific feature flag
 * @param featureKey - The feature key to check
 * @returns object with enabled status and loading state
 */
export function useFeatureFlag(featureKey: string) {
  const { featureFlags, isLoading, error } = useFeatureFlags();

  return {
    isEnabled: isFeatureEnabled(featureKey, featureFlags),
    isLoading,
    error,
  };
}

/**
 * Hook for getting a specific feature configuration
 * @param featureKey - The feature key to get config for
 * @returns object with config, loading state, and convenience methods
 */
export function useFeatureConfig(featureKey: string) {
  const { featureConfigs, isLoading, error } = useFeatureFlags();
  const config = getFeatureConfig(featureKey, featureConfigs);

  return {
    config,
    isLoading,
    error,
    // Convenience methods for common config value types
    getString: (configKey: string, defaultValue: string = '') =>
      getFeatureConfigString(featureKey, configKey, defaultValue, featureConfigs),
    getNumber: (configKey: string, defaultValue: number = 0) =>
      getFeatureConfigNumber(featureKey, configKey, defaultValue, featureConfigs),
    getBoolean: (configKey: string, defaultValue: boolean = false) =>
      getFeatureConfigBoolean(featureKey, configKey, defaultValue, featureConfigs),
    getValue: (configKey: string) => getFeatureConfigValue(featureKey, configKey, featureConfigs),
  };
}

/**
 * Combined hook for both feature flag and config
 * @param featureKey - The feature key
 * @returns object with both flag status and config
 */
export function useFeature(featureKey: string) {
  const { featureFlags, featureConfigs, isLoading, error } = useFeatureFlags();
  const isEnabled = isFeatureEnabled(featureKey, featureFlags);
  const config = getFeatureConfig(featureKey, featureConfigs);

  return {
    isEnabled,
    config,
    isLoading,
    error,
    // Convenience methods for common config value types
    getString: (configKey: string, defaultValue: string = '') =>
      getFeatureConfigString(featureKey, configKey, defaultValue, featureConfigs),
    getNumber: (configKey: string, defaultValue: number = 0) =>
      getFeatureConfigNumber(featureKey, configKey, defaultValue, featureConfigs),
    getBoolean: (configKey: string, defaultValue: boolean = false) =>
      getFeatureConfigBoolean(featureKey, configKey, defaultValue, featureConfigs),
    getValue: (configKey: string) => getFeatureConfigValue(featureKey, configKey, featureConfigs),
  };
}

/**
 * Invalidate the feature flags cache to force a fresh fetch
 */
export function invalidateFeatureFlagsCache() {
  isCacheValid = false;
  featureFlagsCache = {};
  featureConfigsCache = {};
  cacheTimestamp = 0;
}
