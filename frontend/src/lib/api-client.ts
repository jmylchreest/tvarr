import { API_CONFIG, REQUEST_TIMEOUT } from './config';
import { Debug } from '@/utils/debug';
import {
  ApiResponse,
  PaginatedResponse,
  StreamSourceResponse,
  CreateStreamSourceRequest,
  UpdateStreamSourceRequest,
  EpgSourceResponse,
  CreateEpgSourceRequest,
  StreamProxy,
  CreateStreamProxyRequest,
  UpdateStreamProxyRequest,
  Filter,
  FilterListResponse,
  FilterTestRequest,
  DataMappingRule,
  DataMappingRuleListResponse,
  RelayProfile,
  RelayHealthApiResponse,
  RuntimeSettings,
  UpdateSettingsRequest,
  SettingsResponse,
  LogoAsset,
  LogoAssetsResponse,
  LogoStats,
  LogoAssetUpdateRequest,
  LogoUploadRequest,
  ManualChannelInput,
} from '@/types/api';

class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public response?: any
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

// Transform backend stream source response to frontend format
function transformStreamSourceResponse(source: any): StreamSourceResponse {
  return {
    ...source,
    source_type: source.type || source.source_type,
    update_cron: source.cron_schedule || source.update_cron || '',
    max_concurrent_streams: source.max_concurrent_streams || 0,
  };
}

// Transform backend EPG source response to frontend format
function transformEpgSourceResponse(source: any): EpgSourceResponse {
  return {
    ...source,
    source_type: source.type || source.source_type,
    update_cron: source.cron_schedule || source.update_cron || '',
  };
}

class ApiClient {
  private baseUrl: string;
  private debug = Debug.createLogger('ApiClient');

  constructor(baseUrl: string = API_CONFIG.baseUrl) {
    this.baseUrl = baseUrl;
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;

    // Don't set Content-Type for FormData uploads - let browser set multipart boundary
    const isFormData = options.body instanceof FormData;
    const defaultHeaders: Record<string, string> = {
      Accept: 'application/json',
    };

    if (!isFormData) {
      defaultHeaders['Content-Type'] = 'application/json';
    }

    const config: RequestInit = {
      ...options,
      headers: {
        ...defaultHeaders,
        ...options.headers,
      },
    };

    // Add timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT);
    config.signal = controller.signal;

    try {
      const response = await fetch(url, config);
      clearTimeout(timeoutId);

      if (!response.ok) {
        let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
        let errorData;

        try {
          errorData = await response.json();
          // Check for different error formats:
          // - Huma RFC 7807 problem details: { detail: "message" }
          // - Standard error: { error: "message" }
          // - Message field: { message: "message" }
          if (errorData.detail) {
            errorMessage = errorData.detail;
          } else if (errorData.error) {
            errorMessage = errorData.error;
          } else if (errorData.message) {
            errorMessage = errorData.message;
          }
        } catch {
          // Response is not JSON, use status text
        }

        throw new ApiError(errorMessage, response.status, errorData);
      }

      // Handle empty responses
      if (response.status === 204) {
        return {} as T;
      }

      const data = await response.json();

      // Handle wrapped responses with success/data structure
      if (data.success && data.data) {
        return data.data;
      }

      return data;
    } catch (error) {
      clearTimeout(timeoutId);

      if (error instanceof ApiError) {
        throw error;
      }

      if (error instanceof Error && error.name === 'AbortError') {
        throw new ApiError('Request timeout', 408);
      }

      throw new ApiError(error instanceof Error ? error.message : 'Network error occurred', 0);
    }
  }

  // Stream Sources API
  async getStreamSources(params?: {
    page?: number;
    limit?: number;
    search?: string;
    source_type?: string;
  }): Promise<PaginatedResponse<StreamSourceResponse>> {
    const searchParams = new URLSearchParams();

    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.search) searchParams.set('search', params.search);
    if (params?.source_type) searchParams.set('type', params.source_type); // Backend uses 'type'

    const queryString = searchParams.toString();
    const endpoint = `${API_CONFIG.endpoints.streamSources}${queryString ? `?${queryString}` : ''}`;

    const response = await this.request<any>(endpoint);
    // Backend returns 'sources' array, frontend expects 'items'
    const sources = response.sources || response.items || [];
    const transformedSources = sources.map(transformStreamSourceResponse);

    // Return in the format expected by frontend PaginatedResponse
    return {
      items: transformedSources,
      total: response.total || transformedSources.length,
      page: response.page || 1,
      per_page: response.per_page || response.limit || transformedSources.length,
      total_pages: response.total_pages || 1,
      has_next: response.has_next || false,
      has_previous: response.has_previous || false,
    } as PaginatedResponse<StreamSourceResponse>;
  }

  async getStreamSource(id: string): Promise<StreamSourceResponse> {
    const response = await this.request<any>(
      `${API_CONFIG.endpoints.streamSources}/${id}`
    );
    return transformStreamSourceResponse(response);
  }

  async createStreamSource(
    source: CreateStreamSourceRequest
  ): Promise<StreamSourceResponse> {
    // Manual source normalization:
    // - If source_type==='manual': require >=1 manual_channels; strip empty url (backend treats as optional)
    // - If not manual: remove accidental manual_channels to avoid 400 from backend
    const payload: any = { ...source };
    if (payload.source_type === 'manual') {
      if (!Array.isArray(payload.manual_channels) || payload.manual_channels.length === 0) {
        throw new ApiError(
          'manual_channels must contain at least one channel for manual sources',
          400
        );
      }
      if (!payload.url) {
        delete payload.url;
      }
    } else if ('manual_channels' in payload) {
      delete payload.manual_channels;
    }

    // Transform frontend field names to backend field names
    if ('source_type' in payload) {
      payload.type = payload.source_type;
      delete payload.source_type;
    }
    if ('update_cron' in payload) {
      payload.cron_schedule = payload.update_cron;
      delete payload.update_cron;
    }
    // Backend doesn't have max_concurrent_streams on source
    delete payload.max_concurrent_streams;

    const response = await this.request<any>(API_CONFIG.endpoints.streamSources, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    return transformStreamSourceResponse(response);
  }

  async updateStreamSource(
    id: string,
    source: UpdateStreamSourceRequest
  ): Promise<StreamSourceResponse> {
    // Normalize manual update semantics:
    // - If manual & manual_channels provided:
    //     * Reject explicit empty array to prevent accidental wipe (omit field for no change)
    //     * Strip empty url
    // - If non-manual: ensure manual_channels not sent
    const payload: any = { ...source };
    if (payload.source_type === 'manual') {
      if (payload.manual_channels && payload.manual_channels.length === 0) {
        throw new ApiError(
          'manual_channels list may not be empty for manual sources; omit the field to make no changes',
          400
        );
      }
      if (!payload.url) {
        delete payload.url;
      }
    } else if ('manual_channels' in payload) {
      delete payload.manual_channels;
    }

    // Transform frontend field names to backend field names
    if ('source_type' in payload) {
      payload.type = payload.source_type;
      delete payload.source_type;
    }
    if ('update_cron' in payload) {
      payload.cron_schedule = payload.update_cron;
      delete payload.update_cron;
    }
    // Backend doesn't have max_concurrent_streams on source
    delete payload.max_concurrent_streams;

    const response = await this.request<any>(
      `${API_CONFIG.endpoints.streamSources}/${id}`,
      {
        method: 'PUT',
        body: JSON.stringify(payload),
      }
    );
    return transformStreamSourceResponse(response);
  }

  async deleteStreamSource(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.streamSources}/${id}`, {
      method: 'DELETE',
    });
  }

  async refreshStreamSource(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.streamSources}/${id}/ingest`, {
      method: 'POST',
    });
  }

  async validateStreamSource(source: CreateStreamSourceRequest): Promise<any> {
    return this.request<any>(`${API_CONFIG.endpoints.streamSources}/validate`, {
      method: 'POST',
      body: JSON.stringify(source),
    });
  }

  // ---------------- Manual Channel Endpoints (Manual Stream Sources) ----------------

  /**
   * List manual channel definitions for a manual stream source.
   * includeInactive currently future-proofs the API (all rows active today).
   */
  async listManualChannels(
    sourceId: string,
    includeInactive = false
  ): Promise<ManualChannelInput[]> {
    const qs = includeInactive ? '?include_inactive=true' : '';
    return this.request<ManualChannelInput[]>(
      `${API_CONFIG.endpoints.streamSources}/${sourceId}/manual-channels${qs}`
    );
  }

  /**
   * Replace (full overwrite) manual channels and materialize them (server returns summary).
   * Returns the summary object with replace & delta stats.
   */
  async replaceManualChannels(sourceId: string, channels: ManualChannelInput[]): Promise<any> {
    if (!channels.length) {
      throw new ApiError('manual_channels payload cannot be empty', 400);
    }
    return this.request<any>(`${API_CONFIG.endpoints.streamSources}/${sourceId}/manual-channels`, {
      method: 'PUT',
      body: JSON.stringify(channels),
    });
  }

  /**
   * Import M3U for a manual source.
   * apply = false: preview parsed channels (array of ManualChannelInput-like objects).
   * apply = true: replace+materialize on the backend; returns summary (replace_summary + delta).
   */
  async importManualChannelsM3U(sourceId: string, m3uText: string, apply = false): Promise<any> {
    const qs = apply ? '?apply=true' : '';
    const endpoint = `${API_CONFIG.endpoints.streamSources}/${sourceId}/manual-channels/import-m3u${qs}`;
    // Bypass JSON wrapper because this is text/plain in, JSON (or array) out
    return this.request<any>(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
        Accept: 'application/json',
      },
      body: m3uText,
    });
  }

  /**
   * Export manual channels as an M3U playlist (returns raw text).
   */
  async exportManualChannelsM3U(sourceId: string): Promise<string> {
    const endpoint = `${API_CONFIG.endpoints.streamSources}/${sourceId}/manual-channels/export.m3u`;
    const url = `${this.baseUrl}${endpoint}`;
    const resp = await fetch(url, {
      method: 'GET',
      headers: {
        Accept: 'text/plain',
      },
    });
    if (!resp.ok) {
      throw new ApiError(`HTTP ${resp.status}: ${resp.statusText}`, resp.status);
    }
    return await resp.text();
  }

  // EPG Sources API
  async getEpgSources(params?: {
    page?: number;
    limit?: number;
    search?: string;
    source_type?: string;
  }): Promise<PaginatedResponse<EpgSourceResponse>> {
    const searchParams = new URLSearchParams();

    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.search) searchParams.set('search', params.search);
    if (params?.source_type) searchParams.set('type', params.source_type); // Backend uses 'type'

    const queryString = searchParams.toString();
    const endpoint = `${API_CONFIG.endpoints.epgSources}${queryString ? `?${queryString}` : ''}`;

    const response = await this.request<any>(endpoint);
    // Backend returns 'sources' array, frontend expects 'items'
    const sources = response.sources || response.items || [];
    const transformedSources = sources.map(transformEpgSourceResponse);

    // Return in the format expected by frontend PaginatedResponse
    return {
      items: transformedSources,
      total: response.total || transformedSources.length,
      page: response.page || 1,
      per_page: response.per_page || response.limit || transformedSources.length,
      total_pages: response.total_pages || 1,
      has_next: response.has_next || false,
      has_previous: response.has_previous || false,
    } as PaginatedResponse<EpgSourceResponse>;
  }

  async getEpgSource(id: string): Promise<EpgSourceResponse> {
    const response = await this.request<any>(`${API_CONFIG.endpoints.epgSources}/${id}`);
    return transformEpgSourceResponse(response);
  }

  async createEpgSource(source: CreateEpgSourceRequest): Promise<EpgSourceResponse> {
    // Transform frontend field names to backend field names
    const payload: any = { ...source };
    if ('source_type' in payload) {
      payload.type = payload.source_type;
      delete payload.source_type;
    }
    if ('update_cron' in payload) {
      payload.cron_schedule = payload.update_cron;
      delete payload.update_cron;
    }

    const response = await this.request<any>(API_CONFIG.endpoints.epgSources, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    return transformEpgSourceResponse(response);
  }

  async updateEpgSource(
    id: string,
    source: CreateEpgSourceRequest
  ): Promise<EpgSourceResponse> {
    // Transform frontend field names to backend field names
    const payload: any = { ...source };
    if ('source_type' in payload) {
      payload.type = payload.source_type;
      delete payload.source_type;
    }
    if ('update_cron' in payload) {
      payload.cron_schedule = payload.update_cron;
      delete payload.update_cron;
    }

    const response = await this.request<any>(
      `${API_CONFIG.endpoints.epgSources}/${id}`,
      {
        method: 'PUT',
        body: JSON.stringify(payload),
      }
    );
    return transformEpgSourceResponse(response);
  }

  async deleteEpgSource(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.epgSources}/${id}`, {
      method: 'DELETE',
    });
  }

  async refreshEpgSource(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.epgSources}/${id}/ingest`, {
      method: 'POST',
    });
  }

  // Proxy API
  async getProxies(params?: {
    page?: number;
    limit?: number;
    search?: string;
  }): Promise<PaginatedResponse<StreamProxy>> {
    const searchParams = new URLSearchParams();

    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.search) searchParams.set('search', params.search);

    const queryString = searchParams.toString();
    const endpoint = `${API_CONFIG.endpoints.proxies}${queryString ? `?${queryString}` : ''}`;

    const response = await this.request<any>(endpoint);

    // Backend returns 'proxies' array, frontend expects 'items'
    const proxies = response.proxies || response.items || [];
    const total = response.total || proxies.length;
    const page = response.page || params?.page || 1;
    const limit = response.limit || params?.limit || 50;
    const totalPages = response.total_pages || Math.ceil(total / limit);
    return {
      items: proxies,
      total,
      page,
      limit,
      per_page: limit,
      total_pages: totalPages,
      has_next: page < totalPages,
      has_previous: page > 1,
    } as PaginatedResponse<StreamProxy>;
  }

  async getProxy(id: string): Promise<ApiResponse<StreamProxy>> {
    return this.request<ApiResponse<StreamProxy>>(`${API_CONFIG.endpoints.proxies}/${id}`);
  }

  async createProxy(proxy: CreateStreamProxyRequest): Promise<ApiResponse<StreamProxy>> {
    // Transform frontend field names to backend field names
    const payload: any = { ...proxy };

    // Convert stream_sources array to source_ids array of ULIDs
    if ('stream_sources' in payload) {
      payload.source_ids = (payload.stream_sources || []).map((s: any) => s.id || s);
      delete payload.stream_sources;
    }

    // Convert epg_sources array to epg_source_ids array of ULIDs
    if ('epg_sources' in payload) {
      payload.epg_source_ids = (payload.epg_sources || []).map((s: any) => s.id || s);
      delete payload.epg_sources;
    }

    // Remove filters - handled separately via setProxyFilters
    delete payload.filters;

    return this.request<ApiResponse<StreamProxy>>(API_CONFIG.endpoints.proxies, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async updateProxy(
    id: string,
    proxy: UpdateStreamProxyRequest
  ): Promise<ApiResponse<StreamProxy>> {
    // Transform frontend field names to backend field names
    const payload: any = { ...proxy };

    // Remove source-related fields - update doesn't support them, use setProxySources instead
    delete payload.stream_sources;
    delete payload.source_ids;
    delete payload.epg_sources;
    delete payload.epg_source_ids;
    delete payload.filters;

    // Remove empty relay_profile_id - backend expects ULID or null, not empty string
    if (payload.relay_profile_id === '' || payload.relay_profile_id === null) {
      delete payload.relay_profile_id;
    }

    return this.request<ApiResponse<StreamProxy>>(`${API_CONFIG.endpoints.proxies}/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
  }

  async deleteProxy(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.proxies}/${id}`, {
      method: 'DELETE',
    });
  }

  async regenerateProxy(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.proxies}/${id}/regenerate`, {
      method: 'POST',
    });
  }

  // Proxy association methods - these may or may not exist in the API
  async getProxyStreamSources(proxyId: string): Promise<any[]> {
    try {
      return await this.request<any[]>(`${API_CONFIG.endpoints.proxies}/${proxyId}/sources`);
    } catch (error) {
      this.debug.warn(`Proxy stream sources endpoint not available for ${proxyId}:`, error);
      return [];
    }
  }

  async getProxyEpgSources(proxyId: string): Promise<any[]> {
    try {
      return await this.request<any[]>(`${API_CONFIG.endpoints.proxies}/${proxyId}/epg-sources`);
    } catch (error) {
      this.debug.warn(`Proxy EPG sources endpoint not available for ${proxyId}:`, error);
      return [];
    }
  }

  async getProxyFilters(proxyId: string): Promise<any[]> {
    try {
      return await this.request<any[]>(`${API_CONFIG.endpoints.proxies}/${proxyId}/filters`);
    } catch (error) {
      this.debug.warn(`Proxy filters endpoint not available for ${proxyId}:`, error);
      return [];
    }
  }

  // Filters API
  async getFilters(params?: {
    page?: number;
    limit?: number;
    search?: string;
    source_type?: string;
    enabled?: boolean;
  }): Promise<Filter[]> {
    const searchParams = new URLSearchParams();

    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.search) searchParams.set('search', params.search);
    if (params?.source_type) searchParams.set('source_type', params.source_type);
    if (params?.enabled !== undefined) searchParams.set('enabled', params.enabled.toString());

    const queryString = searchParams.toString();
    const endpoint = `${API_CONFIG.endpoints.filters}${queryString ? `?${queryString}` : ''}`;

    const response = await this.request<FilterListResponse>(endpoint);
    return response.filters;
  }

  async getFilter(id: string): Promise<ApiResponse<Filter>> {
    return this.request<ApiResponse<Filter>>(`${API_CONFIG.endpoints.filters}/${id}`);
  }

  async createFilter(
    filter: Omit<Filter, 'id' | 'created_at' | 'updated_at'>
  ): Promise<ApiResponse<Filter>> {
    return this.request<ApiResponse<Filter>>(API_CONFIG.endpoints.filters, {
      method: 'POST',
      body: JSON.stringify(filter),
    });
  }

  async updateFilter(
    id: string,
    filter: Omit<Filter, 'id' | 'created_at' | 'updated_at'>
  ): Promise<ApiResponse<Filter>> {
    return this.request<ApiResponse<Filter>>(`${API_CONFIG.endpoints.filters}/${id}`, {
      method: 'PUT',
      body: JSON.stringify(filter),
    });
  }

  async deleteFilter(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.filters}/${id}`, {
      method: 'DELETE',
    });
  }

  async testFilter(testRequest: FilterTestRequest): Promise<any> {
    return this.request<any>(`${API_CONFIG.endpoints.filters}/test`, {
      method: 'POST',
      body: JSON.stringify(testRequest),
    });
  }

  async validateFilter(
    filterExpression: string
  ): Promise<{ valid: boolean; error?: string; match_count?: number }> {
    return this.request<{ valid: boolean; error?: string; match_count?: number }>(
      `${API_CONFIG.endpoints.filters}/validate`,
      {
        method: 'POST',
        body: JSON.stringify({ filter_expression: filterExpression }),
      }
    );
  }

  async getFilterFields(): Promise<string[]> {
    return this.request<string[]>(`${API_CONFIG.endpoints.filters}/fields`);
  }

  // Data Mapping API
  async getDataMappingRules(params?: {
    page?: number;
    limit?: number;
    search?: string;
    source_type?: string;
  }): Promise<DataMappingRule[]> {
    const searchParams = new URLSearchParams();

    if (params?.page) searchParams.set('page', params.page.toString());
    if (params?.limit) searchParams.set('limit', params.limit.toString());
    if (params?.search) searchParams.set('search', params.search);
    if (params?.source_type) searchParams.set('source_type', params.source_type);

    const queryString = searchParams.toString();
    const endpoint = `${API_CONFIG.endpoints.dataMapping}${queryString ? `?${queryString}` : ''}`;

    const response = await this.request<DataMappingRuleListResponse>(endpoint);
    return response.rules;
  }

  async getDataMappingRule(id: string): Promise<ApiResponse<DataMappingRule>> {
    return this.request<ApiResponse<DataMappingRule>>(`${API_CONFIG.endpoints.dataMapping}/${id}`);
  }

  async createDataMappingRule(
    rule: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  ): Promise<ApiResponse<DataMappingRule>> {
    return this.request<ApiResponse<DataMappingRule>>(API_CONFIG.endpoints.dataMapping, {
      method: 'POST',
      body: JSON.stringify(rule),
    });
  }

  async updateDataMappingRule(
    id: string,
    rule: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  ): Promise<ApiResponse<DataMappingRule>> {
    return this.request<ApiResponse<DataMappingRule>>(`${API_CONFIG.endpoints.dataMapping}/${id}`, {
      method: 'PUT',
      body: JSON.stringify(rule),
    });
  }

  async deleteDataMappingRule(id: string): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.dataMapping}/${id}`, {
      method: 'DELETE',
    });
  }

  async reorderDataMappingRules(rules: { id: string; priority: number }[]): Promise<void> {
    await this.request<void>(`${API_CONFIG.endpoints.dataMapping}/reorder`, {
      method: 'PUT',
      body: JSON.stringify({ rules }),
    });
  }

  async validateDataMappingExpression(
    expression: string,
    sourceType: string
  ): Promise<{
    valid: boolean;
    error?: string;
    errors?: any[];
    canonical_expression?: string;
  }> {
    const domain = sourceType === 'epg' ? 'epg_mapping' : 'stream_mapping';

    // Call unified validation endpoint
    const data = await this.request<any>(`/api/v1/expressions/validate?domain=${domain}`, {
      method: 'POST',
      body: JSON.stringify({ expression }),
    });

    // Translate unified response (ExpressionValidateResult) into legacy shape expected by callers
    // Unified response fields: is_valid, canonical_expression, errors (array), etc.
    const firstErrorMessage =
      !data.is_valid && Array.isArray(data.errors) && data.errors.length > 0
        ? data.errors[0].message || data.errors[0].details || 'Invalid expression'
        : undefined;

    return {
      valid: !!data.is_valid,
      error: firstErrorMessage,
      errors: data.errors,
      canonical_expression: data.canonical_expression,
    };
  }

  async getDataMappingFields(sourceType: string): Promise<string[]> {
    return this.request<string[]>(`${API_CONFIG.endpoints.dataMapping}/fields/${sourceType}`);
  }

  async testDataMappingRule(testRequest: {
    source_id: string;
    source_type: string;
    expression: string;
  }): Promise<any> {
    return this.request<any>(`${API_CONFIG.endpoints.dataMapping}/test`, {
      method: 'POST',
      body: JSON.stringify(testRequest),
    });
  }

  async previewDataMappingRule(previewRequest: {
    source_id?: string;
    source_type: string;
    expression: string;
    sample_data?: any;
  }): Promise<any> {
    const method = previewRequest.sample_data ? 'POST' : 'GET';
    const endpoint = `${API_CONFIG.endpoints.dataMapping}/preview`;

    if (method === 'GET') {
      const searchParams = new URLSearchParams({
        source_type: previewRequest.source_type,
        expression: previewRequest.expression,
      });
      if (previewRequest.source_id) {
        searchParams.set('source_id', previewRequest.source_id);
      }
      return this.request<any>(`${endpoint}?${searchParams.toString()}`);
    } else {
      return this.request<any>(endpoint, {
        method: 'POST',
        body: JSON.stringify(previewRequest),
      });
    }
  }

  // Relay Profiles API
  async getRelayProfiles(): Promise<RelayProfile[]> {
    const response = await this.request<{ profiles: RelayProfile[] }>(
      `${API_CONFIG.endpoints.relays}/profiles`
    );
    return response.profiles || [];
  }

  // Settings API
  async getSettings(): Promise<SettingsResponse> {
    return this.request<SettingsResponse>('/api/v1/settings');
  }

  async updateSettings(settings: UpdateSettingsRequest): Promise<SettingsResponse> {
    return this.request<SettingsResponse>('/api/v1/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    });
  }

  async getSettingsInfo(): Promise<any> {
    return this.request<any>('/api/v1/settings/info');
  }

  // Logo endpoints
  async getLogos(params?: {
    page?: number;
    limit?: number;
    include_cached?: boolean;
    search?: string;
  }): Promise<LogoAssetsResponse> {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.set('page', params.page.toString());
    if (params?.limit) queryParams.set('limit', params.limit.toString());
    if (params?.include_cached !== undefined)
      queryParams.set('include_cached', params.include_cached.toString());
    if (params?.search) queryParams.set('search', params.search);

    const query = queryParams.toString();
    return this.request(`${API_CONFIG.endpoints.logos}${query ? `?${query}` : ''}`);
  }

  async getLogoStats(): Promise<LogoStats> {
    return this.request(`${API_CONFIG.endpoints.logos}/stats`);
  }

  async deleteLogo(id: string): Promise<void> {
    return this.request(`${API_CONFIG.endpoints.logos}/${id}`, {
      method: 'DELETE',
    });
  }

  async updateLogo(id: string, data: LogoAssetUpdateRequest): Promise<LogoAsset> {
    return this.request(`${API_CONFIG.endpoints.logos}/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async replaceLogoImage(
    id: string,
    file: File,
    name?: string,
    description?: string
  ): Promise<LogoAsset> {
    const formData = new FormData();
    formData.append('file', file);
    if (name) formData.append('name', name);
    if (description) formData.append('description', description);

    return this.request(`${API_CONFIG.endpoints.logos}/${id}/replace`, {
      method: 'PUT',
      body: formData,
    });
  }

  async uploadLogo(data: LogoUploadRequest): Promise<LogoAsset> {
    const formData = new FormData();
    formData.append('file', data.file);
    formData.append('name', data.name);
    if (data.description) {
      formData.append('description', data.description);
    }

    return this.request(`${API_CONFIG.endpoints.logos}/upload`, {
      method: 'POST',
      body: formData,
      // Don't set Content-Type header, let the browser set it with boundary
      headers: {},
    });
  }

  // Rescan logo cache
  async rescanLogoCache(): Promise<any> {
    return this.request(`${API_CONFIG.endpoints.logos}/rescan`, {
      method: 'POST',
    });
  }

  // Clear logo cache
  async clearLogoCache(): Promise<any> {
    return this.request(`${API_CONFIG.endpoints.logos}/clear-cache`, {
      method: 'DELETE',
    });
  }

  // Health check
  async healthCheck(): Promise<any> {
    return this.request<any>(API_CONFIG.endpoints.health);
  }

  // Feature flags API
  async getFeatures(): Promise<{
    flags: Record<string, boolean>;
    config: Record<string, Record<string, any>>;
    timestamp: string;
  }> {
    return this.request<{
      flags: Record<string, boolean>;
      config: Record<string, Record<string, any>>;
      timestamp: string;
    }>('/api/v1/features');
  }

  async updateFeatures(data: {
    flags: Record<string, boolean>;
    config: Record<string, Record<string, any>>;
  }): Promise<any> {
    return this.request<any>('/api/v1/features', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  // Relay health check
  async getRelayHealth(): Promise<RelayHealthApiResponse> {
    return this.request<RelayHealthApiResponse>('/api/v1/relay/health');
  }
}

// Export singleton instance
export const apiClient = new ApiClient();
export { ApiError };
export type { ApiClient };
