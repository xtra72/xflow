// React Query hook for the node type registry.

import { useQuery } from '@tanstack/react-query';

import * as nodeService from '@/services/api/nodeService';

/**
 * Fetch all registered node types.
 * Results are cached for 5 minutes since node types rarely change at runtime.
 */
export function useNodeTypes() {
  return useQuery({
    queryKey: ['nodeTypes'],
    queryFn: () => nodeService.getNodeTypes(),
    staleTime: 5 * 60 * 1000,
  });
}
