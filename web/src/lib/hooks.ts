import { useQuery } from "@tanstack/react-query";
import { ApiError, get } from "./api";
import type { Bootstrap, Dashboard, Me, Network } from "./types";

export const useBootstrap = () =>
  useQuery({ queryKey: ["bootstrap"], queryFn: () => get<Bootstrap>("/api/bootstrap"), staleTime: 60_000 });

export const useMe = () =>
  useQuery({
    queryKey: ["me"],
    queryFn: async () => {
      try {
        return await get<Me>("/api/me");
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) return null;
        throw e;
      }
    },
    retry: false,
    staleTime: 30_000,
  });

export const useNetwork = () =>
  useQuery({ queryKey: ["network"], queryFn: () => get<Network>("/api/network"), staleTime: 5 * 60_000 });

export const useDashboard = (enabled = true) =>
  useQuery({
    queryKey: ["dashboard"],
    queryFn: () => get<Dashboard>("/api/dashboard"),
    enabled,
    refetchInterval: 60_000,
  });

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : "未知错误");
