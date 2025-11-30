'use client';

import { useState, useEffect, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Plus,
  Search,
  Activity,
  Users,
  Zap,
  Settings,
  Edit,
  Trash2,
  HardDrive,
  Cpu,
  Volume2,
  Loader2,
  Grid,
  List,
  Table as TableIcon,
} from 'lucide-react';
import {
  RelayProfile,
  RelayHealthResponse,
  CreateRelayProfileRequest,
  UpdateRelayProfileRequest,
  ApiResponse,
} from '@/types/api';
import { getBackendUrl } from '@/lib/config';
import { RelayProfileForm } from '@/components/relay-profile-form';

function formatBitrate(bitrate?: number): string {
  if (!bitrate) return 'Auto';
  if (bitrate >= 1000) {
    return `${(bitrate / 1000).toFixed(1)}M`;
  }
  return `${bitrate}K`;
}

function formatCodec(codec: string): string {
  return codec.toUpperCase().replace('_', ' ');
}

function getProfileSummaryLabels(profile: RelayProfile): string[] {
  const labels = [];

  if (profile.enable_hardware_acceleration) {
    labels.push(
      `HW Accel${profile.preferred_hwaccel ? ` (${profile.preferred_hwaccel.toUpperCase()})` : ''}`
    );
  }

  if (profile.is_system_default) {
    labels.push('System Default');
  }

  labels.push(`${formatCodec(profile.video_codec)} + ${formatCodec(profile.audio_codec)}`);
  labels.push('TS'); // Always Transport Stream

  if (profile.video_bitrate) {
    labels.push(`Video: ${formatBitrate(profile.video_bitrate)}`);
  }

  if (profile.audio_bitrate) {
    labels.push(`Audio: ${formatBitrate(profile.audio_bitrate)}`);
  }

  return labels;
}

export function RelayProfiles() {
  const [profiles, setProfiles] = useState<RelayProfile[]>([]);
  const [healthData, setHealthData] = useState<RelayHealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createLoading, setCreateLoading] = useState(false);
  const [editLoading, setEditLoading] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [editProfile, setEditProfile] = useState<RelayProfile | null>(null);
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');

  const fetchProfiles = async () => {
    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/relay/profiles`);
      if (!response.ok) throw new Error('Failed to fetch relay profiles');
      const data: RelayProfile[] = await response.json();
      setProfiles(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred');
    }
  };

  const fetchHealthData = async () => {
    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/relay/health`);
      if (!response.ok) throw new Error('Failed to fetch relay health');
      const data: RelayHealthResponse = await response.json();
      setHealthData(data || null);
    } catch (err) {
      console.warn('Failed to fetch relay health:', err);
    }
  };

  const handleCreateProfile = async (
    data: CreateRelayProfileRequest | UpdateRelayProfileRequest
  ) => {
    setCreateLoading(true);
    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/relay/profiles`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      if (!response.ok) throw new Error('Failed to create relay profile');
      await fetchProfiles();
      setCreateDialogOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create profile');
    } finally {
      setCreateLoading(false);
    }
  };

  const handleUpdateProfile = async (id: string, data: UpdateRelayProfileRequest) => {
    setEditLoading(true);
    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/relay/profiles/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      if (!response.ok) throw new Error('Failed to update relay profile');
      await fetchProfiles();
      setEditProfile(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update profile');
    } finally {
      setEditLoading(false);
    }
  };

  const handleDeleteProfile = async (id: string) => {
    if (!confirm('Are you sure you want to delete this relay profile?')) return;

    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/relay/profiles/${id}`, {
        method: 'DELETE',
      });
      if (!response.ok) throw new Error('Failed to delete relay profile');
      await fetchProfiles();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete profile');
    }
  };

  const filteredProfiles = useMemo(() => {
    let filtered = profiles;

    // Apply search filter
    if (searchTerm) {
      const term = searchTerm.toLowerCase();
      filtered = filtered.filter(
        (profile) =>
          profile.name.toLowerCase().includes(term) ||
          profile.description?.toLowerCase().includes(term) ||
          profile.video_codec.toLowerCase().includes(term) ||
          profile.audio_codec.toLowerCase().includes(term)
      );
    }

    // Sort: manual profiles first, then system defaults
    return filtered.sort((a, b) => {
      if (a.is_system_default && !b.is_system_default) return 1;
      if (!a.is_system_default && b.is_system_default) return -1;
      return a.name.localeCompare(b.name); // Secondary sort by name
    });
  }, [profiles, searchTerm]);

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      setError(null);
      await Promise.all([fetchProfiles(), fetchHealthData()]);
      setLoading(false);
    };

    loadData();

    // Refresh health data every 10 seconds
    const interval = setInterval(fetchHealthData, 10000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-48">
        <div className="text-muted-foreground">Loading relay profiles...</div>
      </div>
    );
  }

  const totalClients =
    healthData?.processes.reduce((sum, process) => sum + process.connected_clients.length, 0) || 0;

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">
            Manage transcoding profiles for stream relay and optimization
          </p>
        </div>
        <Sheet open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
          <SheetTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Create Profile
            </Button>
          </SheetTrigger>
          <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
            <SheetHeader>
              <SheetTitle>Create Relay Profile</SheetTitle>
              <SheetDescription>
                Create a new transcoding profile for stream relay and optimization.
              </SheetDescription>
            </SheetHeader>
            <RelayProfileForm
              formId="create-relay-profile-form"
              onSubmit={handleCreateProfile}
              onCancel={() => setCreateDialogOpen(false)}
              loading={createLoading}
            />
            <SheetFooter className="gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateDialogOpen(false)}
                disabled={createLoading}
              >
                Cancel
              </Button>
              <Button form="create-relay-profile-form" type="submit" disabled={createLoading}>
                {createLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create Profile
              </Button>
            </SheetFooter>
          </SheetContent>
        </Sheet>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-destructive">
              <span className="font-medium">Error:</span>
              <span>{error}</span>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Stats Bar */}
      <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Relays</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{healthData?.healthy_processes || 0}</div>
            <p className="text-xs text-muted-foreground">
              {healthData?.total_processes || 0} total processes
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Connected Clients</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalClients}</div>
            <p className="text-xs text-muted-foreground">Across all active relays</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Relay Profiles</CardTitle>
            <Settings className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{profiles.length}</div>
            <p className="text-xs text-muted-foreground">
              {profiles.filter((p) => p.is_active).length} active profiles
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System Status</CardTitle>
            <Zap className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {healthData?.unhealthy_processes === 0 ? 'Healthy' : 'Issues'}
            </div>
            <p className="text-xs text-muted-foreground">
              {healthData?.unhealthy_processes || 0} unhealthy processes
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Search & Filters */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Search className="h-5 w-5" />
            Search & Filters
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search profiles, codecs, formats..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8"
                />
              </div>
            </div>

            {/* Layout Chooser */}
            <div className="flex rounded-md border">
              <Button
                size="sm"
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                className="rounded-r-none border-r"
                onClick={() => setViewMode('table')}
              >
                <TableIcon className="w-4 h-4" />
              </Button>
              <Button
                size="sm"
                variant={viewMode === 'grid' ? 'default' : 'ghost'}
                className="rounded-none border-r"
                onClick={() => setViewMode('grid')}
              >
                <Grid className="w-4 h-4" />
              </Button>
              <Button
                size="sm"
                variant={viewMode === 'list' ? 'default' : 'ghost'}
                className="rounded-l-none"
                onClick={() => setViewMode('list')}
              >
                <List className="w-4 h-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Profiles Display */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>Relay Profiles ({filteredProfiles?.length || 0})</span>
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
          </CardTitle>
          <CardDescription>
            Manage transcoding profiles for stream relay and optimization
          </CardDescription>
        </CardHeader>
        <CardContent>
          {viewMode === 'table' && (
            <div className="space-y-4">
              {filteredProfiles.map((profile) => (
                <Card key={profile.id} className={!profile.is_active ? 'opacity-60' : ''}>
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="space-y-1 flex-1">
                        <CardTitle className="text-lg flex items-center gap-2">
                          {profile.name}
                          {profile.is_system_default && <Badge variant="secondary">Default</Badge>}
                          {!profile.is_active && <Badge variant="outline">Inactive</Badge>}
                        </CardTitle>
                        {profile.description && (
                          <CardDescription>{profile.description}</CardDescription>
                        )}
                      </div>

                      <div className="flex items-center gap-1">
                        {!profile.is_system_default && (
                          <>
                            <Sheet
                              open={editProfile?.id === profile.id}
                              onOpenChange={(open) => setEditProfile(open ? profile : null)}
                            >
                              <SheetTrigger asChild>
                                <Button variant="ghost" size="sm">
                                  <Edit className="h-4 w-4" />
                                </Button>
                              </SheetTrigger>
                              <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
                                <SheetHeader>
                                  <SheetTitle>Edit Relay Profile</SheetTitle>
                                  <SheetDescription>
                                    Update the transcoding configuration for this profile.
                                  </SheetDescription>
                                </SheetHeader>
                                <RelayProfileForm
                                  formId="edit-relay-profile-form"
                                  profile={profile}
                                  onSubmit={(data) => handleUpdateProfile(profile.id, data)}
                                  onCancel={() => setEditProfile(null)}
                                  loading={editLoading}
                                />
                                <SheetFooter className="gap-2">
                                  <Button
                                    type="button"
                                    variant="outline"
                                    onClick={() => setEditProfile(null)}
                                    disabled={editLoading}
                                  >
                                    Cancel
                                  </Button>
                                  <Button
                                    form="edit-relay-profile-form"
                                    type="submit"
                                    disabled={editLoading}
                                  >
                                    {editLoading && (
                                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                    )}
                                    Update Profile
                                  </Button>
                                </SheetFooter>
                              </SheetContent>
                            </Sheet>

                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleDeleteProfile(profile.id)}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </>
                        )}
                      </div>
                    </div>
                  </CardHeader>

                  <CardContent>
                    <div className="space-y-4">
                      <div className="flex flex-wrap gap-1">
                        {getProfileSummaryLabels(profile).map((label, index) => (
                          <Badge key={index} variant="outline" className="text-xs">
                            {label}
                          </Badge>
                        ))}
                      </div>

                      <div className="grid gap-2 text-sm">
                        <div className="flex items-center gap-2">
                          <Cpu className="h-3 w-3 text-muted-foreground" />
                          <span className="text-muted-foreground">Video:</span>
                          <span>{formatCodec(profile.video_codec)}</span>
                          {profile.video_bitrate && (
                            <span className="text-muted-foreground">
                              ({formatBitrate(profile.video_bitrate)})
                            </span>
                          )}
                        </div>

                        <div className="flex items-center gap-2">
                          <Volume2 className="h-3 w-3 text-muted-foreground" />
                          <span className="text-muted-foreground">Audio:</span>
                          <span>{formatCodec(profile.audio_codec)}</span>
                          {profile.audio_bitrate && (
                            <span className="text-muted-foreground">
                              ({formatBitrate(profile.audio_bitrate)})
                            </span>
                          )}
                        </div>

                        <div className="flex items-center gap-2">
                          <HardDrive className="h-3 w-3 text-muted-foreground" />
                          <span className="text-muted-foreground">Output:</span>
                          <span>Transport Stream</span>
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {viewMode === 'grid' && (
            <div className="grid gap-4 grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
              {filteredProfiles.map((profile) => (
                <Card
                  key={profile.id}
                  className={`transition-all hover:shadow-md ${!profile.is_active ? 'opacity-60' : ''}`}
                >
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="space-y-1 flex-1">
                        <CardTitle className="text-base flex items-center gap-2">
                          {profile.name}
                          {profile.is_system_default && <Badge variant="secondary">Default</Badge>}
                          {!profile.is_active && <Badge variant="outline">Inactive</Badge>}
                        </CardTitle>
                        {profile.description && (
                          <CardDescription className="text-sm">
                            {profile.description}
                          </CardDescription>
                        )}
                      </div>
                    </div>
                  </CardHeader>

                  <CardContent>
                    <div className="space-y-3">
                      <div className="flex flex-wrap gap-1">
                        {getProfileSummaryLabels(profile)
                          .slice(0, 3)
                          .map((label, index) => (
                            <Badge key={index} variant="outline" className="text-xs">
                              {label}
                            </Badge>
                          ))}
                      </div>

                      <div className="grid gap-1 text-sm">
                        <div className="flex items-center gap-2">
                          <Cpu className="h-3 w-3 text-muted-foreground" />
                          <span className="text-muted-foreground">Video:</span>
                          <span className="text-xs">{formatCodec(profile.video_codec)}</span>
                        </div>

                        <div className="flex items-center gap-2">
                          <Volume2 className="h-3 w-3 text-muted-foreground" />
                          <span className="text-muted-foreground">Audio:</span>
                          <span className="text-xs">{formatCodec(profile.audio_codec)}</span>
                        </div>
                      </div>

                      <div className="flex items-center justify-between pt-2 border-t">
                        <div className="text-xs text-muted-foreground">Transport Stream</div>
                        <div className="flex items-center gap-1">
                          {!profile.is_system_default && (
                            <>
                              <Sheet
                                open={editProfile?.id === profile.id}
                                onOpenChange={(open) => setEditProfile(open ? profile : null)}
                              >
                                <SheetTrigger asChild>
                                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                    <Edit className="h-4 w-4" />
                                  </Button>
                                </SheetTrigger>
                                <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
                                  <SheetHeader>
                                    <SheetTitle>Edit Relay Profile</SheetTitle>
                                    <SheetDescription>
                                      Update the transcoding configuration for this profile.
                                    </SheetDescription>
                                  </SheetHeader>
                                  <RelayProfileForm
                                    formId="edit-relay-profile-form"
                                    profile={profile}
                                    onSubmit={(data) => handleUpdateProfile(profile.id, data)}
                                    onCancel={() => setEditProfile(null)}
                                    loading={editLoading}
                                  />
                                  <SheetFooter className="gap-2">
                                    <Button
                                      type="button"
                                      variant="outline"
                                      onClick={() => setEditProfile(null)}
                                      disabled={editLoading}
                                    >
                                      Cancel
                                    </Button>
                                    <Button
                                      form="edit-relay-profile-form"
                                      type="submit"
                                      disabled={editLoading}
                                    >
                                      {editLoading && (
                                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                      )}
                                      Update Profile
                                    </Button>
                                  </SheetFooter>
                                </SheetContent>
                              </Sheet>

                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                onClick={() => handleDeleteProfile(profile.id)}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </>
                          )}
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {viewMode === 'list' && (
            <div className="space-y-2">
              {filteredProfiles.map((profile) => (
                <Card
                  key={profile.id}
                  className={`transition-all hover:shadow-sm ${!profile.is_active ? 'opacity-60' : ''}`}
                >
                  <CardContent className="pt-4">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-4 flex-1">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-3">
                            <div>
                              <p className="font-medium text-sm">{profile.name}</p>
                              <p className="text-xs text-muted-foreground">
                                {profile.description && profile.description.length > 60
                                  ? `${profile.description.substring(0, 60)}...`
                                  : profile.description ||
                                    `${formatCodec(profile.video_codec)} + ${formatCodec(profile.audio_codec)} to TS`}
                              </p>
                            </div>
                            <div className="flex items-center gap-2">
                              {profile.is_system_default && (
                                <Badge variant="secondary" className="text-xs">
                                  Default
                                </Badge>
                              )}
                              {!profile.is_active && (
                                <Badge variant="outline" className="text-xs">
                                  Inactive
                                </Badge>
                              )}
                              {profile.enable_hardware_acceleration && (
                                <Badge variant="outline" className="text-xs">
                                  <Cpu className="h-3 w-3 mr-1" />
                                  HW Accel
                                </Badge>
                              )}
                              <Badge variant="outline" className="text-xs">
                                {formatCodec(profile.video_codec)}
                              </Badge>
                              <Badge variant="outline" className="text-xs">
                                {formatCodec(profile.audio_codec)}
                              </Badge>
                            </div>
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 ml-4">
                        <div className="flex items-center gap-1">
                          {!profile.is_system_default && (
                            <>
                              <Sheet
                                open={editProfile?.id === profile.id}
                                onOpenChange={(open) => setEditProfile(open ? profile : null)}
                              >
                                <SheetTrigger asChild>
                                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                    <Edit className="h-4 w-4" />
                                  </Button>
                                </SheetTrigger>
                                <SheetContent className="w-full sm:max-w-2xl overflow-y-auto">
                                  <SheetHeader>
                                    <SheetTitle>Edit Relay Profile</SheetTitle>
                                    <SheetDescription>
                                      Update the transcoding configuration for this profile.
                                    </SheetDescription>
                                  </SheetHeader>
                                  <RelayProfileForm
                                    formId="edit-relay-profile-form"
                                    profile={profile}
                                    onSubmit={(data) => handleUpdateProfile(profile.id, data)}
                                    onCancel={() => setEditProfile(null)}
                                    loading={editLoading}
                                  />
                                  <SheetFooter className="gap-2">
                                    <Button
                                      type="button"
                                      variant="outline"
                                      onClick={() => setEditProfile(null)}
                                      disabled={editLoading}
                                    >
                                      Cancel
                                    </Button>
                                    <Button
                                      form="edit-relay-profile-form"
                                      type="submit"
                                      disabled={editLoading}
                                    >
                                      {editLoading && (
                                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                      )}
                                      Update Profile
                                    </Button>
                                  </SheetFooter>
                                </SheetContent>
                              </Sheet>

                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                onClick={() => handleDeleteProfile(profile.id)}
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            </>
                          )}
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {filteredProfiles.length === 0 && (
        <div className="text-center py-8">
          <Settings className="mx-auto h-12 w-12 text-muted-foreground" />
          <h3 className="mt-4 text-lg font-semibold">
            {searchTerm ? 'No profiles match your search.' : 'No relay profiles found.'}
          </h3>
          <p className="text-muted-foreground">
            {searchTerm
              ? 'Try adjusting your search criteria.'
              : 'Get started by creating your first relay profile.'}
          </p>
        </div>
      )}
    </div>
  );
}
