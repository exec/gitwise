import { create } from "zustand";
import { get, post, ApiError } from "../lib/api";

export interface User {
  id: string;
  username: string;
  email: string;
  full_name: string;
  is_admin: boolean;
  is_bot?: boolean;
  created_at: string;
  updated_at: string;
}

interface TwoFactorChallenge {
  pending_token: string;
}

// Discriminated union for the login response
type LoginResponse =
  | { requires_2fa: true; pending_token: string }
  | ({ requires_2fa?: false } & User);

function isTwoFactorResponse(
  r: LoginResponse,
): r is { requires_2fa: true; pending_token: string } {
  return r.requires_2fa === true;
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  twoFactorChallenge: TwoFactorChallenge | null;

  login: (login: string, password: string) => Promise<void>;
  verify2FA: (code: string) => Promise<void>;
  setTwoFactorChallenge: (token: string) => void;
  clearTwoFactorChallenge: () => void;
  register: (
    username: string,
    email: string,
    password: string,
    fullName: string,
  ) => Promise<void>;
  logout: () => Promise<void>;
  fetchMe: () => Promise<void>;
}

export const useAuthStore = create<AuthState>((set, getState) => ({
  user: null,
  isAuthenticated: false,
  isLoading: true,
  twoFactorChallenge: null,

  login: async (login, password) => {
    const { data } = await post<LoginResponse>("/auth/login", { login, password });
    if (isTwoFactorResponse(data)) {
      set({ twoFactorChallenge: { pending_token: data.pending_token } });
      return;
    }
    // Normal login (no 2FA) - data is the User object.
    const user: User = {
      id: data.id,
      username: data.username,
      email: data.email,
      full_name: data.full_name,
      is_admin: data.is_admin,
      is_bot: data.is_bot,
      created_at: data.created_at,
      updated_at: data.updated_at,
    };
    set({ user, isAuthenticated: true, twoFactorChallenge: null });
  },

  verify2FA: async (code: string) => {
    const challenge = getState().twoFactorChallenge;
    if (!challenge) {
      throw new Error("No pending 2FA challenge");
    }
    const { data } = await post<User>("/auth/verify-2fa", {
      pending_token: challenge.pending_token,
      code,
    });
    set({ user: data, isAuthenticated: true, twoFactorChallenge: null });
  },

  setTwoFactorChallenge: (token: string) => {
    set({ twoFactorChallenge: { pending_token: token } });
  },

  clearTwoFactorChallenge: () => {
    set({ twoFactorChallenge: null });
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
    set({ user: null, isAuthenticated: false, twoFactorChallenge: null });
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
