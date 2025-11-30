import React, { useRef, useEffect, useCallback, useState, useMemo } from 'react';
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';
import { Play } from 'lucide-react';
import { Debug } from '@/utils/debug';

interface Channel {
  id: string;
  name: string;
  logo?: string;
}

interface EpgProgram {
  id: string;
  channel_id: string;
  channel_name: string;
  channel_logo?: string;
  title: string;
  description?: string;
  start_time: string;
  end_time: string;
  category?: string;
  rating?: string;
  source_id?: string;
  metadata?: Record<string, string>;
  is_streamable: boolean;
}

interface EpgGuideResponse {
  channels: Record<string, { id: string; name: string; logo?: string }>;
  programs: Record<string, EpgProgram[]>;
  time_slots: string[];
  start_time: string;
  end_time: string;
}

interface CanvasEPGProps {
  guideData: EpgGuideResponse | null;
  guideTimeRange: string;
  channelFilter: string;
  currentTime: Date;
  selectedTimezone: string;
  onProgramClick?: (program: EpgProgram) => void;
  onChannelPlay?: (channel: { id: string; name: string; logo?: string }) => void;
  className?: string;
}

// Constants
const CHANNEL_HEIGHT = 60;
const CHANNEL_SIDEBAR_WIDTH = 200;
const PIXELS_PER_HOUR = 200;
const TIME_HEADER_HEIGHT = 50;

export const CanvasEPG: React.FC<CanvasEPGProps> = ({
  guideData,
  guideTimeRange,
  channelFilter,
  currentTime,
  selectedTimezone,
  onProgramClick,
  onChannelPlay,
  className = '',
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  const [canvasContext, setCanvasContext] = useState<CanvasRenderingContext2D | null>(null);
  const [canvasDimensions, setCanvasDimensions] = useState({ width: 1200, height: 600 });
  const [scrollPosition, setScrollPosition] = useState({ x: 0, y: 0 });
  const [hoveredProgram, setHoveredProgram] = useState<EpgProgram | null>(null);
  const [themeKey, setThemeKey] = useState(0); // Force theme updates
  const isRenderingRef = useRef(false);

  // Calculate guide parameters
  const GUIDE_HOURS = parseInt(guideTimeRange.replace('h', ''));
  const TOTAL_GUIDE_WIDTH = GUIDE_HOURS * PIXELS_PER_HOUR;

  // Extract theme colors from CSS custom properties - memoized for stability
  const theme = useMemo(() => {
    const computedStyle = getComputedStyle(document.documentElement);

    const getColor = (property: string) => {
      const value = computedStyle.getPropertyValue(property).trim();
      // Handle OKLCH colors - wrap in oklch() function
      if (value.startsWith('oklch(') || value.startsWith('hsl(')) {
        return value;
      }
      return `oklch(${value})`;
    };

    const themeColors = {
      background: getColor('--background'),
      foreground: getColor('--foreground'),
      muted: getColor('--muted'),
      mutedForeground: getColor('--muted-foreground'),
      border: getColor('--border'),
      accent: getColor('--accent'),
      accentForeground: getColor('--accent-foreground'),
      primary: getColor('--primary'),
      primaryForeground: getColor('--primary-foreground'),
      secondary: getColor('--secondary'),
      secondaryForeground: getColor('--secondary-foreground'),
    };

    return themeColors;
  }, [themeKey]); // Depend on themeKey to force updates

  // Filtered channels with search - memoized with stable keys
  const filteredChannels = useMemo(() => {
    if (!guideData) return [];

    let channels = Object.entries(guideData.channels);

    if (channelFilter) {
      const searchLower = channelFilter.toLowerCase();
      channels = channels.filter(([id, channel]) => {
        if (
          channel.name.toLowerCase().includes(searchLower) ||
          id.toLowerCase().includes(searchLower)
        ) {
          return true;
        }

        const channelPrograms = guideData.programs[id] || [];
        return channelPrograms.some(
          (program) =>
            program.title.toLowerCase().includes(searchLower) ||
            (program.description && program.description.toLowerCase().includes(searchLower))
        );
      });
    }

    return channels.sort(([, a], [, b]) => a.name.localeCompare(b.name));
  }, [guideData, channelFilter]);

  // Create a stable identifier for filtered channels to reduce re-renders
  const filteredChannelsKey = useMemo(() => {
    return `${filteredChannels.length}-${filteredChannels.map(([id]) => id).join(',')}`;
  }, [filteredChannels]);

  // Calculate total dimensions
  const totalHeight = filteredChannels.length * CHANNEL_HEIGHT + TIME_HEADER_HEIGHT;
  // For total width, use the full guide width for scrolling, but let the container control the visible area
  const totalWidth = CHANNEL_SIDEBAR_WIDTH + TOTAL_GUIDE_WIDTH;

  // Initialize and update canvas context
  useEffect(() => {
    const canvas = canvasRef.current;
    const scrollArea = scrollAreaRef.current;
    if (!canvas || !scrollArea) {
      Debug.log('Canvas or scroll area not found');
      return;
    }

    // Get the main container that constrains the visible area (not the scroll content)
    const mainContainer = document.querySelector('main main');
    if (!mainContainer) {
      Debug.log('Main container not found');
      return;
    }

    // Find the ScrollArea viewport element to get the actual available space
    const viewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]');
    if (!viewport) {
      Debug.log('ScrollArea viewport not found');
      return;
    }

    const rect = viewport.getBoundingClientRect();
    let width = rect.width; // Use full viewport width
    let height = rect.height; // Use full viewport height

    // Ensure minimum reasonable dimensions
    if (width < 100) width = 1200;
    if (height < 100) height = 600;

    const devicePixelRatio = window.devicePixelRatio || 1;

    Debug.log('Initializing canvas to available viewport size:', width, 'x', height);

    // Set canvas size to available viewport dimensions
    canvas.width = width * devicePixelRatio;
    canvas.height = height * devicePixelRatio;

    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';

    const ctx = canvas.getContext('2d');
    if (ctx) {
      ctx.scale(devicePixelRatio, devicePixelRatio);
      setCanvasContext(ctx);
      setCanvasDimensions({ width, height });
      Debug.log('Canvas context initialized with proper viewport dimensions');
    } else {
      console.error('Failed to get canvas context');
    }
  }, [guideData]); // Add guideData as dependency to re-initialize when data changes

  // Handle resize with ResizeObserver for more accurate container size tracking
  useEffect(() => {
    const scrollArea = scrollAreaRef.current;
    if (!scrollArea) return;

    const handleResize = () => {
      const canvas = canvasRef.current;
      if (!canvas) return;

      // Find the ScrollArea viewport element to get the actual available space
      const viewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]');
      if (!viewport) return;

      const rect = viewport.getBoundingClientRect();
      let width = rect.width; // Use full viewport width
      let height = rect.height; // Use full viewport height

      // Ensure minimum reasonable dimensions
      if (width < 100) width = 1200;
      if (height < 100) height = 600;

      const devicePixelRatio = window.devicePixelRatio || 1;

      Debug.log('Resizing canvas to available viewport size:', width, 'x', height);

      try {
        canvas.width = width * devicePixelRatio;
        canvas.height = height * devicePixelRatio;

        canvas.style.width = width + 'px';
        canvas.style.height = height + 'px';

        const ctx = canvas.getContext('2d');
        if (ctx) {
          ctx.scale(devicePixelRatio, devicePixelRatio);
          setCanvasDimensions({ width, height });
        }
      } catch (error) {
        console.error('Canvas resize error:', error);
      }
    };

    const resizeObserver = new ResizeObserver(handleResize);
    // Observe the ScrollArea viewport element for size changes
    const viewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]');
    if (viewport) {
      resizeObserver.observe(viewport);
    }

    return () => {
      resizeObserver.disconnect();
    };
  }, []);

  // Listen for theme changes
  useEffect(() => {
    const observer = new MutationObserver((mutations) => {
      mutations.forEach((mutation) => {
        if (
          mutation.type === 'attributes' &&
          (mutation.attributeName === 'class' || mutation.attributeName === 'data-theme')
        ) {
          Debug.log('Theme changed, updating Canvas colors');
          setThemeKey((prev) => prev + 1);
        }
      });
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class', 'data-theme'],
    });

    return () => observer.disconnect();
  }, []);

  // Helper function to format time
  const formatTimeInTimezone = useCallback(
    (timeString: string) => {
      try {
        const date = new Date(timeString);
        if (isNaN(date.getTime())) return '--:--';

        return date.toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
          timeZone: selectedTimezone,
        });
      } catch {
        return '--:--';
      }
    },
    [selectedTimezone]
  );

  // Calculate program position and dimensions
  const calculateProgramMetrics = useCallback(
    (program: EpgProgram, guideStartTime: Date) => {
      const programStart = new Date(program.start_time);
      const programEnd = new Date(program.end_time);
      const guideStart = guideStartTime;
      const guideEnd = new Date(guideStart.getTime() + GUIDE_HOURS * 60 * 60 * 1000);

      if (programEnd <= guideStart || programStart >= guideEnd) {
        return { left: -9999, width: 0, programStart, programEnd };
      }

      const visibleStart = programStart < guideStart ? guideStart : programStart;
      const visibleEnd = programEnd > guideEnd ? guideEnd : programEnd;

      const startOffset = visibleStart.getTime() - guideStart.getTime();
      const leftPosition = (startOffset / (1000 * 60 * 60)) * PIXELS_PER_HOUR;

      const visibleDuration = visibleEnd.getTime() - visibleStart.getTime();
      const width = Math.max(30, (visibleDuration / (1000 * 60 * 60)) * PIXELS_PER_HOUR);

      return { left: Math.max(0, leftPosition), width, programStart, programEnd };
    },
    [GUIDE_HOURS]
  );

  // Main render function
  const renderCanvas = useCallback(() => {
    if (!canvasContext || !guideData) {
      Debug.log('Missing context or data:', !!canvasContext, !!guideData);
      return;
    }

    const ctx = canvasContext;
    const { width, height } = canvasDimensions;

    Debug.log('Rendering canvas:', width, 'x', height, 'channels:', filteredChannels.length);

    // Clear canvas
    ctx.clearRect(0, 0, width, height);

    // Test render - draw a simple rectangle to verify canvas is working
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(10, 10, 100, 50);
    ctx.fillStyle = '#000000';
    ctx.font = '16px Arial';
    ctx.fillText('Canvas Test', 15, 35);

    const guideStartTime = new Date(guideData.start_time);
    const viewportStartY = scrollPosition.y;
    const viewportEndY = viewportStartY + height;

    // Calculate visible channel range
    const startChannelIndex = Math.max(
      0,
      Math.floor((viewportStartY - TIME_HEADER_HEIGHT) / CHANNEL_HEIGHT)
    );
    const endChannelIndex = Math.min(
      filteredChannels.length,
      Math.ceil((viewportEndY - TIME_HEADER_HEIGHT) / CHANNEL_HEIGHT) + 1
    );

    // Set fonts and styles
    ctx.font = '12px ui-sans-serif, system-ui, -apple-system, sans-serif';
    ctx.textBaseline = 'middle';

    // First pass: Render main content area (channels and programs)
    // This gets clipped by sticky headers

    // Render visible channels and programs first
    for (let i = startChannelIndex; i < endChannelIndex; i++) {
      if (!filteredChannels[i]) continue;

      const [channelId, channel] = filteredChannels[i];
      const channelY = TIME_HEADER_HEIGHT + i * CHANNEL_HEIGHT - viewportStartY;

      // Skip if channel row is not visible
      if (channelY + CHANNEL_HEIGHT < TIME_HEADER_HEIGHT || channelY > height) continue;

      // Channel row background (alternating) - only in the program area
      ctx.fillStyle = i % 2 === 0 ? theme.background : theme.muted;
      ctx.fillRect(CHANNEL_SIDEBAR_WIDTH, channelY, width - CHANNEL_SIDEBAR_WIDTH, CHANNEL_HEIGHT);

      // Channel separator line - only in the program area
      ctx.strokeStyle = theme.border;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(CHANNEL_SIDEBAR_WIDTH, channelY + CHANNEL_HEIGHT);
      ctx.lineTo(width, channelY + CHANNEL_HEIGHT);
      ctx.stroke();

      // Render programs for this channel
      const channelPrograms = guideData.programs[channelId] || [];
      const programStartX = CHANNEL_SIDEBAR_WIDTH - scrollPosition.x;

      channelPrograms.forEach((program) => {
        const metrics = calculateProgramMetrics(program, guideStartTime);

        if (metrics.left < -9999 || metrics.width === 0) return;

        const programX = programStartX + metrics.left;
        const programY = channelY + 8;
        const programWidth = metrics.width;
        const programHeight = CHANNEL_HEIGHT - 16;

        // Skip if program is not visible horizontally
        if (programX + programWidth < CHANNEL_SIDEBAR_WIDTH || programX > width) return;

        // Determine if program is live or hovered
        const now = currentTime;
        const programStart = new Date(program.start_time);
        const programEnd = new Date(program.end_time);
        const isLive = now >= programStart && now <= programEnd;
        const isHovered = hoveredProgram?.id === program.id;

        // Program background - use distinct colors for live programs
        if (isLive) {
          // Live programs: Use accent for normal, primary for hover (more prominent)
          ctx.fillStyle = isHovered ? theme.primary : theme.accent;
        } else {
          // Regular programs: Use secondary for normal, muted for hover (subtle)
          ctx.fillStyle = isHovered ? theme.muted : theme.secondary;
        }

        ctx.fillRect(programX, programY, programWidth, programHeight);

        // Program borders
        ctx.strokeStyle = theme.border;
        ctx.lineWidth = 1;
        ctx.strokeRect(programX, programY, programWidth, programHeight);

        // Live program indicator
        if (isLive) {
          ctx.strokeStyle = theme.primary;
          ctx.lineWidth = 3;
          ctx.beginPath();
          ctx.moveTo(programX, programY);
          ctx.lineTo(programX, programY + programHeight);
          ctx.stroke();
        }

        // Program text - ensure good contrast
        if (isLive) {
          // Live programs use accent foreground colors for better contrast
          ctx.fillStyle = isHovered ? theme.primaryForeground : theme.accentForeground;
        } else {
          // Regular programs use standard foreground
          ctx.fillStyle = theme.foreground;
        }
        ctx.font = 'bold 11px ui-sans-serif, system-ui, -apple-system, sans-serif';

        // Program title
        const maxTitleWidth = programWidth - 12;
        if (maxTitleWidth > 20) {
          const titleText =
            program.title.length > maxTitleWidth / 7
              ? program.title.substring(0, Math.floor(maxTitleWidth / 7)) + '...'
              : program.title;
          ctx.fillText(titleText, programX + 6, programY + 14);
        }

        // Program time (if there's space)
        if (programHeight > 32) {
          ctx.font = '9px ui-sans-serif, system-ui, -apple-system, sans-serif';
          if (isLive) {
            // Live programs use accent foreground colors for consistency
            ctx.fillStyle = isHovered ? theme.primaryForeground : theme.accentForeground;
          } else {
            // Regular programs use muted foreground for secondary text
            ctx.fillStyle = theme.mutedForeground;
          }
          const timeText = `${formatTimeInTimezone(program.start_time)}-${formatTimeInTimezone(program.end_time)}`;
          ctx.fillText(timeText, programX + 6, programY + 30);
        }

        // Store program bounds for hit testing (adjusted for scroll)
        (program as any)._bounds = {
          x: programX,
          y: programY,
          width: programWidth,
          height: programHeight,
          scrollX: scrollPosition.x,
          scrollY: scrollPosition.y,
        };
      });
    }

    // Second pass: Render sticky time header (always on top)
    ctx.fillStyle = theme.muted;
    ctx.fillRect(0, 0, width, TIME_HEADER_HEIGHT);

    // Time header border
    ctx.strokeStyle = theme.border;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, TIME_HEADER_HEIGHT);
    ctx.lineTo(width, TIME_HEADER_HEIGHT);
    ctx.stroke();

    // Render time grid headers (sticky)
    const headerStartX = CHANNEL_SIDEBAR_WIDTH - scrollPosition.x;
    for (let hour = 0; hour < GUIDE_HOURS; hour++) {
      const x = headerStartX + hour * PIXELS_PER_HOUR;

      // Skip if header is not visible
      if (x + PIXELS_PER_HOUR < CHANNEL_SIDEBAR_WIDTH || x > width) continue;

      // Vertical hour lines (extend through content area)
      ctx.strokeStyle = theme.border;
      ctx.beginPath();
      ctx.moveTo(x, TIME_HEADER_HEIGHT);
      ctx.lineTo(x, height);
      ctx.stroke();

      // Time labels (in sticky header)
      if (x > CHANNEL_SIDEBAR_WIDTH - 50 && x < width - 50) {
        const time = new Date(guideStartTime.getTime() + hour * 60 * 60 * 1000);
        const timeLabel = formatTimeInTimezone(time.toISOString());

        ctx.fillStyle = theme.foreground;
        ctx.font = '12px ui-sans-serif, system-ui, -apple-system, sans-serif';
        ctx.fillText(timeLabel, x + 10, TIME_HEADER_HEIGHT / 2);
      }
    }

    // Third pass: Render sticky channel sidebar (always on left)
    // Channel sidebar background
    ctx.fillStyle = theme.secondary;
    ctx.fillRect(0, TIME_HEADER_HEIGHT, CHANNEL_SIDEBAR_WIDTH, height - TIME_HEADER_HEIGHT);

    // Render visible channel info in sidebar
    for (let i = startChannelIndex; i < endChannelIndex; i++) {
      if (!filteredChannels[i]) continue;

      const [channelId, channel] = filteredChannels[i];
      const channelY = TIME_HEADER_HEIGHT + i * CHANNEL_HEIGHT - viewportStartY;

      // Skip if channel row is not visible
      if (channelY + CHANNEL_HEIGHT < TIME_HEADER_HEIGHT || channelY > height) continue;

      // Channel info area background (alternating) - only in sidebar
      ctx.fillStyle = i % 2 === 0 ? theme.secondary : theme.muted;
      ctx.fillRect(0, channelY, CHANNEL_SIDEBAR_WIDTH, CHANNEL_HEIGHT);

      // Channel name and ID
      ctx.fillStyle = theme.foreground;
      ctx.font = 'bold 13px ui-sans-serif, system-ui, -apple-system, sans-serif';
      const channelName = channel.name || channelId;
      const truncatedName =
        channelName.length > 20 ? channelName.substring(0, 17) + '...' : channelName;
      ctx.fillText(truncatedName, 50, channelY + CHANNEL_HEIGHT / 2 - 8);

      ctx.font = '11px ui-sans-serif, system-ui, -apple-system, sans-serif';
      ctx.fillStyle = theme.mutedForeground;
      ctx.fillText(channelId, 50, channelY + CHANNEL_HEIGHT / 2 + 8);

      // Play button area
      ctx.fillStyle = theme.accent;
      ctx.fillRect(10, channelY + 16, 28, 28);
      ctx.strokeStyle = theme.border;
      ctx.strokeRect(10, channelY + 16, 28, 28);

      // Play icon (triangle)
      ctx.fillStyle = theme.primary;
      ctx.beginPath();
      ctx.moveTo(18, channelY + 24);
      ctx.lineTo(18, channelY + 36);
      ctx.lineTo(28, channelY + 30);
      ctx.closePath();
      ctx.fill();

      // Channel separator line - only in sidebar area
      ctx.strokeStyle = theme.border;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(0, channelY + CHANNEL_HEIGHT);
      ctx.lineTo(CHANNEL_SIDEBAR_WIDTH, channelY + CHANNEL_HEIGHT);
      ctx.stroke();
    }

    // Channel sidebar separator (always visible)
    ctx.strokeStyle = theme.border;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(CHANNEL_SIDEBAR_WIDTH, 0);
    ctx.lineTo(CHANNEL_SIDEBAR_WIDTH, height);
    ctx.stroke();

    // Current time indicator
    if (guideData.start_time && guideData.end_time) {
      const now = currentTime;
      const guideStart = new Date(guideData.start_time);
      const guideEnd = new Date(guideData.end_time);

      if (now >= guideStart && now <= guideEnd) {
        const currentOffset = now.getTime() - guideStart.getTime();
        const currentX =
          CHANNEL_SIDEBAR_WIDTH +
          (currentOffset / (1000 * 60 * 60)) * PIXELS_PER_HOUR -
          scrollPosition.x;

        if (currentX >= CHANNEL_SIDEBAR_WIDTH && currentX <= width) {
          ctx.strokeStyle = '#ef4444';
          ctx.lineWidth = 3;
          ctx.beginPath();
          ctx.moveTo(currentX, TIME_HEADER_HEIGHT);
          ctx.lineTo(currentX, height);
          ctx.stroke();

          // Time indicator dot
          ctx.fillStyle = '#ef4444';
          ctx.beginPath();
          ctx.arc(currentX, TIME_HEADER_HEIGHT, 6, 0, 2 * Math.PI);
          ctx.fill();
        }
      }
    }
  }, [
    canvasContext,
    guideData,
    canvasDimensions.width,
    canvasDimensions.height,
    scrollPosition.x,
    scrollPosition.y,
    filteredChannelsKey, // Use stable key instead of array
    currentTime.getTime(), // Use primitive value
    hoveredProgram?.id,
    calculateProgramMetrics,
    formatTimeInTimezone,
    GUIDE_HOURS,
    themeKey, // Use themeKey instead of theme object
  ]);

  // Render when key dependencies change (debounced to prevent infinite loops)
  useEffect(() => {
    if (!canvasContext || !guideData || isRenderingRef.current) return;

    Debug.log('Render effect triggered');
    isRenderingRef.current = true;

    // Use requestAnimationFrame to debounce and optimize rendering
    const frameId = requestAnimationFrame(() => {
      renderCanvas();
      isRenderingRef.current = false;
    });

    return () => {
      cancelAnimationFrame(frameId);
      isRenderingRef.current = false;
    };
  }, [
    canvasContext,
    guideData,
    canvasDimensions.width,
    canvasDimensions.height,
    scrollPosition.x,
    scrollPosition.y,
    filteredChannelsKey, // Use stable key for channels
    currentTime.getTime(), // Use primitive value instead of Date object
    hoveredProgram?.id,
    themeKey,
    // Removed renderCanvas to prevent circular dependency
  ]);

  // Handle scroll events
  const handleScroll = useCallback((event: Event) => {
    const target = event.target as HTMLDivElement;
    const newScrollPosition = {
      x: target.scrollLeft,
      y: target.scrollTop,
    };
    setScrollPosition(newScrollPosition);
  }, []);

  // Set up scroll listener
  useEffect(() => {
    const scrollArea = scrollAreaRef.current;
    if (!scrollArea) return;

    // ScrollArea ref points directly to the ScrollArea component,
    // we need to find the viewport within it
    const scrollViewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]');
    if (!scrollViewport) {
      Debug.log('ScrollArea viewport not found');
      return;
    }

    Debug.log('Setting up scroll listener on viewport');
    scrollViewport.addEventListener('scroll', handleScroll);
    return () => {
      Debug.log('Removing scroll listener');
      scrollViewport.removeEventListener('scroll', handleScroll);
    };
  }, [handleScroll]);

  // Hit testing for mouse events
  const findProgramAtPoint = useCallback(
    (x: number, y: number): EpgProgram | null => {
      if (!guideData) return null;

      for (const [channelId] of filteredChannels) {
        const programs = guideData.programs[channelId] || [];
        for (const program of programs) {
          const bounds = (program as any)._bounds;
          if (
            bounds &&
            x >= bounds.x &&
            x <= bounds.x + bounds.width &&
            y >= bounds.y &&
            y <= bounds.y + bounds.height
          ) {
            return program;
          }
        }
      }
      return null;
    },
    [guideData, filteredChannels]
  );

  const findChannelAtPoint = useCallback(
    (x: number, y: number): [string, Channel] | null => {
      if (x > CHANNEL_SIDEBAR_WIDTH) return null;

      const channelIndex = Math.floor((y + scrollPosition.y - TIME_HEADER_HEIGHT) / CHANNEL_HEIGHT);
      if (channelIndex >= 0 && channelIndex < filteredChannels.length) {
        return filteredChannels[channelIndex];
      }
      return null;
    },
    [filteredChannels, scrollPosition.y]
  );

  // Mouse event handlers
  const handleMouseMove = useCallback(
    (event: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas) return;

      const rect = canvas.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;

      // Check if hovering over play button
      let isOverPlayButton = false;
      if (x >= 10 && x <= 38) {
        const adjustedY = y + scrollPosition.y;
        const channelIndex = Math.floor((adjustedY - TIME_HEADER_HEIGHT) / CHANNEL_HEIGHT);

        if (channelIndex >= 0 && channelIndex < filteredChannels.length) {
          const channelRowY = TIME_HEADER_HEIGHT + channelIndex * CHANNEL_HEIGHT;
          const relativeY = adjustedY - channelRowY;

          if (relativeY >= 16 && relativeY <= 44) {
            isOverPlayButton = true;
          }
        }
      }

      const program = findProgramAtPoint(x, y);
      setHoveredProgram(program);

      canvas.style.cursor = program || isOverPlayButton ? 'pointer' : 'default';
    },
    [findProgramAtPoint, scrollPosition, filteredChannels]
  );

  const handleClick = useCallback(
    (event: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas) return;

      const rect = canvas.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;

      // Check for play button click first (simplified logic)
      if (x >= 10 && x <= 38) {
        const adjustedY = y + scrollPosition.y;
        const channelIndex = Math.floor((adjustedY - TIME_HEADER_HEIGHT) / CHANNEL_HEIGHT);

        if (channelIndex >= 0 && channelIndex < filteredChannels.length) {
          const channelRowY = TIME_HEADER_HEIGHT + channelIndex * CHANNEL_HEIGHT;
          const relativeY = adjustedY - channelRowY;

          // Play button is at y: 16-44 within the channel row
          if (relativeY >= 16 && relativeY <= 44) {
            const [channelId, channelData] = filteredChannels[channelIndex];
            if (onChannelPlay) {
              onChannelPlay({ id: channelId, name: channelData.name, logo: channelData.logo });
            }
            return;
          }
        }
      }

      // Check for program click
      const program = findProgramAtPoint(x, y);
      if (program && onProgramClick) {
        onProgramClick(program);
      }
    },
    [findProgramAtPoint, onProgramClick, onChannelPlay, scrollPosition, filteredChannels]
  );

  // Handle mouse wheel events and forward them to the ScrollArea
  const handleWheel = useCallback((event: WheelEvent) => {
    const scrollArea = scrollAreaRef.current;
    if (!scrollArea) return;

    // Find the ScrollArea viewport
    const viewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]') as HTMLElement;
    if (!viewport) return;

    // Prevent default scrolling behavior on the canvas
    event.preventDefault();

    // Forward the wheel event to the ScrollArea viewport
    viewport.scrollBy({
      left: event.deltaX,
      top: event.deltaY,
      behavior: 'auto',
    });
  }, []);

  // Set up wheel event listener with { passive: false }
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    // Add wheel event listener as non-passive to allow preventDefault
    canvas.addEventListener('wheel', handleWheel, { passive: false });

    return () => {
      canvas.removeEventListener('wheel', handleWheel);
    };
  }, [handleWheel]);

  return (
    <div className={`w-full flex-1 ${className}`} style={{ position: 'relative', height: '100%' }}>
      <canvas
        ref={canvasRef}
        className="absolute top-0 left-0 z-10"
        style={{
          pointerEvents: 'auto',
        }}
        onMouseMove={handleMouseMove}
        onClick={handleClick}
        tabIndex={0}
      />
      <ScrollArea
        className="w-full h-full epg-scroll overflow-auto"
        ref={scrollAreaRef}
        style={{ height: '100%', overflow: 'auto' }}
      >
        <div
          ref={containerRef}
          className="relative"
          style={{
            width: totalWidth,
            height: totalHeight,
            // Increase min dimensions slightly so scrollbars are always available
            minWidth: totalWidth + 200,
            minHeight: totalHeight + 120,
          }}
        >
          {/* Invisible content area for proper scrolling */}
        </div>
        <ScrollBar orientation="vertical" />
        <ScrollBar orientation="horizontal" />
      </ScrollArea>
      {/* Themed scrollbar styles (scoped to this component) */}
      <style jsx>{`
        .epg-scroll [data-radix-scroll-area-viewport] {
          scrollbar-width: thin;
          scrollbar-color: var(--muted-foreground) var(--muted);
        }
        .epg-scroll [data-radix-scroll-area-viewport]::-webkit-scrollbar {
          width: 10px;
          height: 10px;
        }
        .epg-scroll [data-radix-scroll-area-viewport]::-webkit-scrollbar-track {
          background: var(--muted);
        }
        .epg-scroll [data-radix-scroll-area-viewport]::-webkit-scrollbar-thumb {
          background: var(--accent);
          border-radius: 8px;
          border: 2px solid var(--muted);
        }
        .epg-scroll [data-radix-scroll-area-viewport]::-webkit-scrollbar-thumb:hover {
          background: var(--accent-foreground);
        }
        /* Fallback for environments without CSS vars */
        .epg-scroll [data-radix-scroll-area-viewport]::-webkit-scrollbar-thumb:active {
          background: var(--primary);
        }
      `}</style>
    </div>
  );
};

export default CanvasEPG;
