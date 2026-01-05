'use client';

import { useSearchParams } from 'next/navigation';
import { Filters } from '@/components/filters';
import { Suspense } from 'react';

function FiltersPageContent() {
  const searchParams = useSearchParams();

  // Parse query params for create mode
  const shouldCreate = searchParams.get('create') === 'true';
  const initialExpression = searchParams.get('expression') || '';
  const initialSourceType = (searchParams.get('source_type') as 'stream' | 'epg') || 'stream';

  return (
    <Filters
      initialCreateMode={shouldCreate}
      initialExpression={initialExpression}
      initialSourceType={initialSourceType}
    />
  );
}

export default function FiltersPage() {
  return (
    <Suspense fallback={<div className="container mx-auto p-6">Loading...</div>}>
      <FiltersPageContent />
    </Suspense>
  );
}
