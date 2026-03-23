import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { Admin } from '@/types';

interface AuthState {
  token: string | null;
  user: Admin | null;
  isAuthenticated: boolean;
  login: (token: string, user: Admin) => void;
  logout: () => void;
  updateUser: (user: Admin) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,
      login: (token, user) =>
        set({
          token,
          user,
          isAuthenticated: true,
        }),
      logout: () =>
        set({
          token: null,
          user: null,
          isAuthenticated: false,
        }),
      updateUser: (user) =>
        set((state) => ({
          user: state.user ? { ...state.user, ...user } : user,
        })),
    }),
    {
      name: 'auth-storage',
    }
  )
);
