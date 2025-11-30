'use client';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Palette, Copy, Check, Type } from 'lucide-react';
import { useState, useEffect } from 'react';

interface ColourSwatch {
  name: string;
  cssVar: string;
  description: string;
  category: string;
}

const colourSwatches: ColourSwatch[] = [
  // Base Theme Colors
  {
    name: 'Background',
    cssVar: '--background',
    description: 'Main background colour',
    category: 'Base',
  },
  { name: 'Foreground', cssVar: '--foreground', description: 'Main text colour', category: 'Base' },

  // Primary Colors
  {
    name: 'Primary',
    cssVar: '--primary',
    description: 'Primary brand colour',
    category: 'Primary',
  },
  {
    name: 'Primary Foreground',
    cssVar: '--primary-foreground',
    description: 'Text on primary background',
    category: 'Primary',
  },

  // Secondary Colors
  {
    name: 'Secondary',
    cssVar: '--secondary',
    description: 'Secondary brand colour',
    category: 'Secondary',
  },
  {
    name: 'Secondary Foreground',
    cssVar: '--secondary-foreground',
    description: 'Text on secondary background',
    category: 'Secondary',
  },

  // Accent Colors
  {
    name: 'Accent',
    cssVar: '--accent',
    description: 'Accent/highlight colour',
    category: 'Accent',
  },
  {
    name: 'Accent Foreground',
    cssVar: '--accent-foreground',
    description: 'Text on accent background',
    category: 'Accent',
  },

  // Muted Colors
  { name: 'Muted', cssVar: '--muted', description: 'Muted background colour', category: 'Muted' },
  {
    name: 'Muted Foreground',
    cssVar: '--muted-foreground',
    description: 'Muted text colour',
    category: 'Muted',
  },

  // Card Colors
  { name: 'Card', cssVar: '--card', description: 'Card background colour', category: 'Card' },
  {
    name: 'Card Foreground',
    cssVar: '--card-foreground',
    description: 'Text on card background',
    category: 'Card',
  },

  // Popover Colors
  {
    name: 'Popover',
    cssVar: '--popover',
    description: 'Popover background colour',
    category: 'Popover',
  },
  {
    name: 'Popover Foreground',
    cssVar: '--popover-foreground',
    description: 'Text on popover background',
    category: 'Popover',
  },

  // Border & Input
  { name: 'Border', cssVar: '--border', description: 'Border colour', category: 'Border' },
  { name: 'Input', cssVar: '--input', description: 'Input background colour', category: 'Border' },
  { name: 'Ring', cssVar: '--ring', description: 'Focus ring colour', category: 'Border' },

  // Destructive
  {
    name: 'Destructive',
    cssVar: '--destructive',
    description: 'Destructive/error colour',
    category: 'Destructive',
  },
  {
    name: 'Destructive Foreground',
    cssVar: '--destructive-foreground',
    description: 'Text on destructive background',
    category: 'Destructive',
  },

  // Chart Colors
  { name: 'Chart 1', cssVar: '--chart-1', description: 'Chart colour 1', category: 'Chart' },
  { name: 'Chart 2', cssVar: '--chart-2', description: 'Chart colour 2', category: 'Chart' },
  { name: 'Chart 3', cssVar: '--chart-3', description: 'Chart colour 3', category: 'Chart' },
  { name: 'Chart 4', cssVar: '--chart-4', description: 'Chart colour 4', category: 'Chart' },
  { name: 'Chart 5', cssVar: '--chart-5', description: 'Chart colour 5', category: 'Chart' },

  // Sidebar Colors
  {
    name: 'Sidebar Background',
    cssVar: '--sidebar-background',
    description: 'Sidebar background colour',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Foreground',
    cssVar: '--sidebar-foreground',
    description: 'Sidebar text colour',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Primary',
    cssVar: '--sidebar-primary',
    description: 'Sidebar primary colour',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Primary Foreground',
    cssVar: '--sidebar-primary-foreground',
    description: 'Text on sidebar primary',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Accent',
    cssVar: '--sidebar-accent',
    description: 'Sidebar accent colour',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Accent Foreground',
    cssVar: '--sidebar-accent-foreground',
    description: 'Text on sidebar accent',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Border',
    cssVar: '--sidebar-border',
    description: 'Sidebar border colour',
    category: 'Sidebar',
  },
  {
    name: 'Sidebar Ring',
    cssVar: '--sidebar-ring',
    description: 'Sidebar focus ring colour',
    category: 'Sidebar',
  },
];

// Group colours by category
const colourCategories = colourSwatches.reduce(
  (acc, colour) => {
    if (!acc[colour.category]) {
      acc[colour.category] = [];
    }
    acc[colour.category].push(colour);
    return acc;
  },
  {} as Record<string, ColourSwatch[]>
);

function ColourSwatch({ colour }: { colour: ColourSwatch }) {
  const [copied, setCopied] = useState(false);

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy text: ', err);
    }
  };

  return (
    <div className="flex items-center space-x-3 p-3 rounded-lg border hover:bg-muted/50 transition-colors">
      {/* Colour Preview */}
      <div
        className="w-12 h-12 rounded-lg border-2 shadow-sm flex-shrink-0"
        style={{ backgroundColor: `var(${colour.cssVar})` }}
      />

      {/* Colour Info */}
      <div className="flex-grow min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h4 className="font-medium text-sm">{colour.name}</h4>
          <Badge variant="outline" className="text-xs">
            {colour.cssVar}
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">{colour.description}</p>
      </div>

      {/* Copy Button */}
      <Button
        variant="ghost"
        size="sm"
        onClick={() => copyToClipboard(colour.cssVar)}
        className="flex-shrink-0"
      >
        {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
      </Button>
    </div>
  );
}

function TailwindClassExamples() {
  const examples = [
    { class: 'bg-primary', description: 'Primary background' },
    { class: 'text-primary-foreground', description: 'Primary foreground text' },
    { class: 'bg-secondary', description: 'Secondary background' },
    { class: 'text-secondary-foreground', description: 'Secondary foreground text' },
    { class: 'bg-accent', description: 'Accent background' },
    { class: 'text-accent-foreground', description: 'Accent foreground text' },
    { class: 'bg-muted', description: 'Muted background' },
    { class: 'text-muted-foreground', description: 'Muted foreground text' },
    { class: 'bg-card', description: 'Card background' },
    { class: 'text-card-foreground', description: 'Card foreground text' },
    { class: 'border-border', description: 'Border colour' },
    { class: 'bg-destructive', description: 'Destructive background' },
    { class: 'text-destructive-foreground', description: 'Destructive foreground text' },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tailwind CSS Classes</CardTitle>
        <CardDescription>Common Tailwind CSS classes using theme colours</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {examples.map((example) => (
            <div key={example.class} className={`p-3 rounded-lg border ${example.class}`}>
              <code className="text-sm font-mono">{example.class}</code>
              <p className="text-xs mt-1 opacity-75">{example.description}</p>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function FontInformation() {
  const [currentFonts, setCurrentFonts] = useState<{
    sans: string;
    serif: string;
    mono: string;
    current: string;
  }>({
    sans: '',
    serif: '',
    mono: '',
    current: '',
  });

  useEffect(() => {
    // Get computed font families from CSS variables
    const root = document.documentElement;
    const computedStyle = getComputedStyle(root);

    const fontSans = computedStyle.getPropertyValue('--font-sans').trim() || 'system default';
    const fontSerif = computedStyle.getPropertyValue('--font-serif').trim() || 'system default';
    const fontMono = computedStyle.getPropertyValue('--font-mono').trim() || 'system default';
    const fontFamily =
      computedStyle.getPropertyValue('--font-family').trim() ||
      getComputedStyle(document.body).fontFamily;

    setCurrentFonts({
      sans: fontSans,
      serif: fontSerif,
      mono: fontMono,
      current: fontFamily,
    });
  }, []);

  const fontExamples = [
    {
      name: 'Sans Serif',
      cssVar: '--font-sans',
      value: currentFonts.sans,
      className: 'font-sans',
      sample: 'The quick brown fox jumps over the lazy dog',
    },
    {
      name: 'Serif',
      cssVar: '--font-serif',
      value: currentFonts.serif,
      className: 'font-serif',
      sample: 'The quick brown fox jumps over the lazy dog',
    },
    {
      name: 'Monospace',
      cssVar: '--font-mono',
      value: currentFonts.mono,
      className: 'font-mono',
      sample: "console.log('Hello, World!');",
    },
  ];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Type className="h-5 w-5" />
          Typography & Fonts
        </CardTitle>
        <CardDescription>Font families defined by the current theme</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Current Applied Font */}
        <div className="p-4 bg-muted/50 rounded-lg border">
          <h4 className="font-medium mb-2">Currently Applied Font</h4>
          <p className="text-sm text-muted-foreground mb-2">
            Font family being used by the body element:
          </p>
          <code className="text-sm font-mono bg-background px-2 py-1 rounded border">
            {currentFonts.current}
          </code>
        </div>

        {/* Font Categories */}
        <div className="space-y-4">
          {fontExamples.map((font) => (
            <div key={font.name} className="space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <h4 className="font-medium">{font.name}</h4>
                  <div className="flex items-center gap-2 mt-1">
                    <Badge variant="outline" className="text-xs">
                      {font.cssVar}
                    </Badge>
                    <Badge variant="outline" className="text-xs">
                      {font.className}
                    </Badge>
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => navigator.clipboard.writeText(font.value)}
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>

              <div className="p-3 border rounded-lg bg-card">
                <p className="text-xs text-muted-foreground mb-2">Font stack:</p>
                <code className="text-sm">{font.value}</code>
              </div>

              <div className="p-4 border rounded-lg bg-card">
                <p className="text-xs text-muted-foreground mb-3">Sample text:</p>
                <div className={font.className}>
                  <p className="text-lg mb-2">{font.sample}</p>
                  <div className="text-sm space-y-1">
                    <p>
                      Regular text • <strong>Bold text</strong> • <em>Italic text</em>
                    </p>
                    <p className="text-xs">Small text (12px)</p>
                    <p className="text-lg">Large text (18px)</p>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>

        {/* Font Usage Examples */}
        <div className="space-y-3">
          <h4 className="font-medium">Usage Examples</h4>
          <div className="space-y-3">
            <div className="p-3 border rounded-lg bg-card">
              <p className="text-xs text-muted-foreground mb-2">Tailwind Classes:</p>
              <div className="space-y-2 text-sm">
                <code className="block">className="font-sans" // Sans serif</code>
                <code className="block">className="font-serif" // Serif</code>
                <code className="block">className="font-mono" // Monospace</code>
              </div>
            </div>

            <div className="p-3 border rounded-lg bg-card">
              <p className="text-xs text-muted-foreground mb-2">CSS Variables:</p>
              <div className="space-y-2 text-sm">
                <code className="block">font-family: var(--font-sans);</code>
                <code className="block">font-family: var(--font-serif);</code>
                <code className="block">font-family: var(--font-mono);</code>
              </div>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function ColourPalette() {
  return (
    <div className="space-y-6">
      {/* Page Description */}
      <div className="mb-6">
        <p className="text-muted-foreground">
          Complete overview of all theme colours and CSS variables
        </p>
      </div>

      {/* Colour Categories */}
      {Object.entries(colourCategories).map(([category, colours]) => (
        <Card key={category}>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              {category} Colours
              <Badge variant="secondary">{colours.length}</Badge>
            </CardTitle>
            <CardDescription>
              {category === 'Base' && 'Fundamental background and text colours'}
              {category === 'Primary' && 'Main brand colours for buttons and key UI elements'}
              {category === 'Secondary' && 'Secondary brand colours for alternative styling'}
              {category === 'Accent' && 'Accent colours for highlights and emphasis'}
              {category === 'Muted' && 'Subdued colours for less prominent elements'}
              {category === 'Card' && 'Colours specifically for card components'}
              {category === 'Popover' && 'Colours for popover and dropdown components'}
              {category === 'Border' && 'Border, input, and focus ring colours'}
              {category === 'Destructive' && 'Colours for errors and destructive actions'}
              {category === 'Chart' && 'Data visualization and chart colours'}
              {category === 'Sidebar' && 'Sidebar-specific colour variations'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {colours.map((colour) => (
                <ColourSwatch key={colour.cssVar} colour={colour} />
              ))}
            </div>
          </CardContent>
        </Card>
      ))}

      {/* Font Information */}
      <FontInformation />

      {/* Tailwind Examples */}
      <TailwindClassExamples />

      {/* Usage Instructions */}
      <Card>
        <CardHeader>
          <CardTitle>Usage Instructions</CardTitle>
          <CardDescription>How to use these colours in your components</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <h4 className="font-medium mb-2">CSS Variables</h4>
            <p className="text-sm text-muted-foreground mb-2">
              Use CSS variables directly in your styles:
            </p>
            <code className="block p-2 bg-muted rounded text-sm">
              background-color: var(--primary);
            </code>
          </div>

          <div>
            <h4 className="font-medium mb-2">Tailwind Classes</h4>
            <p className="text-sm text-muted-foreground mb-2">
              Use predefined Tailwind classes for common patterns:
            </p>
            <code className="block p-2 bg-muted rounded text-sm">
              className="bg-primary text-primary-foreground"
            </code>
          </div>

          <div>
            <h4 className="font-medium mb-2">Theme Switching</h4>
            <p className="text-sm text-muted-foreground">
              All colors automatically update when switching themes. No additional code needed.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
