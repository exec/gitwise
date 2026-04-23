import { useEffect, useRef, useCallback, useState } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import { get, post } from "../lib/api";
import { useAuthStore } from "../stores/auth";

interface Notification {
  id: string;
  user_id: string;
  type: string;
  title: string;
  body: string;
  link: string;
  read: boolean;
  metadata: Record<string, unknown>;
  created_at: string;
}

export function useNotifications() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const queryClient = useQueryClient();
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoff = useRef(1000);
  const [wsConnected, setWsConnected] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["notifications"],
    queryFn: async () => {
      const { data } = await get<Notification[]>("/notifications?limit=30");
      return data;
    },
    enabled: isAuthenticated,
    refetchInterval: 60000,
  });

  const notifications = data ?? [];
  const unreadCount = notifications.filter((n) => !n.read).length;

  const markReadMutation = useMutation({
    mutationFn: (id: string) => post(`/notifications/${id}/read`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notifications"] }),
  });

  const markAllReadMutation = useMutation({
    mutationFn: () => post("/notifications/read-all"),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["notifications"] }),
  });

  // Keep a stable ref to queryClient so identity changes don't trigger reconnects
  const queryClientRef = useRef(queryClient);
  useEffect(() => {
    queryClientRef.current = queryClient;
  }, [queryClient]);

  const connect = useCallback(() => {
    if (!useAuthStore.getState().isAuthenticated) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const ws = new WebSocket(`${proto}//${host}/ws`);

    ws.onopen = () => {
      setWsConnected(true);
      backoff.current = 1000;
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === "notification") {
          queryClientRef.current.invalidateQueries({ queryKey: ["notifications"] });
        }
      } catch {
        // ignore non-JSON messages
      }
    };

    ws.onclose = () => {
      setWsConnected(false);
      wsRef.current = null;
      // Check current auth state, not stale closure value
      if (useAuthStore.getState().isAuthenticated) {
        reconnectTimer.current = setTimeout(() => {
          backoff.current = Math.min(backoff.current * 2, 30000);
          connect();
        }, backoff.current);
      }
    };

    ws.onerror = () => {
      ws.close();
    };

    wsRef.current = ws;
    // connect is stable — no deps that change on unrelated renders
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (isAuthenticated) {
      connect();
    }
    return () => {
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [isAuthenticated, connect]);

  return {
    notifications,
    unreadCount,
    isLoading,
    wsConnected,
    markRead: (id: string) => markReadMutation.mutate(id),
    markAllRead: () => markAllReadMutation.mutate(),
  };
}
