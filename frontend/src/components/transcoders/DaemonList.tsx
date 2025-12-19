'use client';

import { useState, useCallback, useMemo } from 'react';
import { Daemon } from '@/types/api';
import { DaemonCard } from './DaemonCard';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
  DropdownMenuLabel,
} from '@/components/ui/dropdown-menu';
import { Server, Copy, Check, ChevronDown, Terminal, Container } from 'lucide-react';

interface DaemonListProps {
  daemons: Daemon[];
  isLoading?: boolean;
  onDrain?: (id: string) => Promise<void>;
  onActivate?: (id: string) => Promise<void>;
}

function LoadingSkeleton() {
  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: 3 }).map((_, i) => (
        <div
          key={i}
          className="rounded-lg border bg-card p-4 animate-pulse space-y-4"
        >
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-2">
              <div className="h-5 w-5 bg-muted rounded" />
              <div className="space-y-1">
                <div className="h-4 w-32 bg-muted rounded" />
                <div className="h-3 w-24 bg-muted rounded" />
              </div>
            </div>
            <div className="h-5 w-16 bg-muted rounded" />
          </div>
          <div className="grid grid-cols-4 gap-2">
            {Array.from({ length: 4 }).map((_, j) => (
              <div key={j} className="h-4 bg-muted rounded" />
            ))}
          </div>
          <div className="space-y-2">
            <div className="h-1.5 bg-muted rounded" />
            <div className="h-1.5 bg-muted rounded" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function ConnectTranscoderButton() {
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);

  // Derive coordinator URL from current location
  const coordinatorUrl = useMemo(() => {
    if (typeof window === 'undefined') return 'localhost:9090';
    const hostname = window.location.hostname;
    return `${hostname}:9090`;
  }, []);

  const commands = useMemo(() => [
    {
      id: 'binary',
      label: 'Binary (Direct)',
      icon: Terminal,
      command: `tvarr-ffmpegd serve --coordinator-url ${coordinatorUrl}`,
    },
    {
      id: 'docker-nvidia',
      label: 'Docker (NVIDIA)',
      icon: Container,
      command: `docker run -d \\
  --name tvarr-ffmpegd \\
  --gpus all \\
  -e NVIDIA_VISIBLE_DEVICES=all \\
  -e NVIDIA_DRIVER_CAPABILITIES=compute,video,utility \\
  -e TVARR_COORDINATOR_URL=${coordinatorUrl} \\
  ghcr.io/jmylchreest/tvarr-ffmpegd:latest`,
    },
    {
      id: 'docker-vaapi',
      label: 'Docker (VA-API)',
      icon: Container,
      command: `docker run -d \\
  --name tvarr-ffmpegd \\
  --device /dev/dri:/dev/dri \\
  -e TVARR_COORDINATOR_URL=${coordinatorUrl} \\
  ghcr.io/jmylchreest/tvarr-ffmpegd:latest`,
    },
    {
      id: 'docker-cpu',
      label: 'Docker (CPU only)',
      icon: Container,
      command: `docker run -d \\
  --name tvarr-ffmpegd \\
  -e TVARR_COORDINATOR_URL=${coordinatorUrl} \\
  ghcr.io/jmylchreest/tvarr-ffmpegd:latest`,
    },
  ], [coordinatorUrl]);

  const handleCopy = useCallback(async (commandId: string, command: string) => {
    try {
      await navigator.clipboard.writeText(command);
      setCopiedCommand(commandId);
      setTimeout(() => setCopiedCommand(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }, []);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="gap-1.5 text-muted-foreground hover:text-foreground h-8">
          <Server className="h-3.5 w-3.5" />
          <span className="text-xs">Connect</span>
          <ChevronDown className="h-3 w-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80">
        <DropdownMenuLabel className="text-xs text-muted-foreground font-normal">
          Copy command to connect a transcoder
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {commands.map((cmd) => (
          <DropdownMenuItem
            key={cmd.id}
            className="flex items-start gap-2 p-3 cursor-pointer"
            onClick={() => handleCopy(cmd.id, cmd.command)}
          >
            <cmd.icon className="h-4 w-4 mt-0.5 shrink-0" />
            <div className="flex-1 min-w-0">
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">{cmd.label}</span>
                {copiedCommand === cmd.id ? (
                  <Check className="h-4 w-4 text-green-500" />
                ) : (
                  <Copy className="h-4 w-4 text-muted-foreground" />
                )}
              </div>
              <code className="text-[10px] text-muted-foreground block truncate mt-1">
                {cmd.command.split('\n')[0]}...
              </code>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-12 text-center">
      <Server className="h-12 w-12 text-muted-foreground mb-4" />
      <h3 className="text-lg font-semibold">No Transcoders Connected</h3>
      <p className="text-sm text-muted-foreground max-w-md mt-2">
        Start a tvarr-ffmpegd daemon to enable distributed transcoding.
        Transcoders will automatically register with the coordinator.
        Use the <span className="font-medium">Connect</span> button above to get started.
      </p>
    </div>
  );
}

export function DaemonList({ daemons, isLoading, onDrain, onActivate }: DaemonListProps) {
  if (isLoading) {
    return <LoadingSkeleton />;
  }

  if (daemons.length === 0) {
    return <EmptyState />;
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {daemons.map((daemon) => (
        <DaemonCard
          key={daemon.id}
          daemon={daemon}
          onDrain={onDrain}
          onActivate={onActivate}
        />
      ))}
    </div>
  );
}
