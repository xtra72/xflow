// Authentication state management.
// Tokens are stored in memory only (NOT localStorage) for security.

import { create } from 'zustand';

import type { AuthTokens, User } from '@/types/auth';

interface AuthState {
  user: User | null;
  tokens: AuthTokens | null;
  isAuthenticated: boolean;
  isLoading: boolean;
}

interface AuthActions {
  login: (user: User, tokens: AuthTokens) => void;
  logout: () => void;
  setTokens: (tokens: AuthTokens) => void;
  setLoading: (loading: boolean) => void;
}

export const useAuthStore = create<AuthState & AuthActions>()((set) => ({
  // State
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,

  // Actions
  login: (user, tokens) =>
    set({
      user,
      tokens,
      isAuthenticated: true,
      isLoading: false,
    }),

  logout: () =>
    set({
      user: null,
      tokens: null,
      isAuthenticated: false,
      isLoading: false,
    }),

  setTokens: (tokens) =>
    set({ tokens }),

  setLoading: (loading) =>
    set({ isLoading: loading }),
}));

/**
 * Non-hook accessor for use in axios interceptors and other
 * non-React contexts where hooks cannot be called.
 */
export const getAuthState = () => useAuthStore.getState();
