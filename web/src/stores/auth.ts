import { create } from "zustand";
import { get, post, ApiError } from "../lib/api";

export interface User {
  id: string;
  username: string;
  email: string;
  full_name: string;
  created_at: string;
  updated_at: string;
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  login: (login: string, password: string) => Promise<void>;
  register: (
    username: string,
    email: string,
    password: string,
    fullName: string,
  ) => Promise<void>;
  logout: () => Promise<void>;
  fetchMe: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,
  isLoading: true,

  login: async (login, password) => {
    const { data } = await post<User>("/auth/login", { login, password });
    set({ user: data, isAuthenticated: true });
  },

  register: async (username, email, password, fullName) => {
    const { data } = await post<User>("/auth/register", {
      username,
      email,
      password,
      full_name: fullName,
    });
    set({ user: data, isAuthenticated: true });
  },

  logout: async () => {
    try {
      await post("/auth/logout");
    } catch {
      // Clear local state even if server call fails
    }
    set({ user: null, isAuthenticated: false });
  },

  fetchMe: async () => {
    try {
      const { data } = await get<User>("/auth/me");
      set({ user: data, isAuthenticated: true, isLoading: false });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        set({ user: null, isAuthenticated: false, isLoading: false });
      } else {
        set({ isLoading: false });
      }
    }
  },
}));
