import { AlertCircle, CheckCircle2, RefreshCw, Shield, Users } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type AdminUser = {
  id: string;
  moodleSiteUrl: string;
  moodleUserId: number;
  displayName: string;
  clerkUserId?: string;
  isAdmin: boolean;
  codexStateQuotaBytes: number;
  codexStateUsageBytes: number;
  codexStateSnapshotCount: number;
  codexStateQuotaConfiguredByDefault: boolean;
};

type AdminUsersResponse = {
  defaultQuotaBytes: number;
  maxQuotaBytes: number;
  users: AdminUser[];
};

type AdminUserResponse = {
  defaultQuotaBytes: number;
  maxQuotaBytes: number;
  user: AdminUser;
};

type Message = {
  kind: "idle" | "ok" | "error";
  text: string;
};

const endpoint = "/api/auth/qr/exchange?codex=admin";
const fallbackDefaultQuotaBytes = 128 * 1024 * 1024;
const fallbackMaxQuotaBytes = 5 * 1024 * 1024 * 1024;

export function AdminDashboard() {
  const [internalSecret, setInternalSecret] = useState("");
  const [clerkUserId, setClerkUserId] = useState("");
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [defaultQuotaBytes, setDefaultQuotaBytes] = useState(fallbackDefaultQuotaBytes);
  const [maxQuotaBytes, setMaxQuotaBytes] = useState(fallbackMaxQuotaBytes);
  const [isBusy, setIsBusy] = useState(false);
  const [message, setMessage] = useState<Message>({
    kind: "idle",
    text: "Enter the internal secret and your admin Clerk user ID.",
  });

  const loadedUsageBytes = useMemo(() => users.reduce((sum, user) => sum + user.codexStateUsageBytes, 0), [users]);

  async function loadUsers() {
    if (!internalSecret.trim() || !clerkUserId.trim()) {
      setMessage({ kind: "error", text: "Internal secret and Clerk user ID are required." });
      return;
    }
    setIsBusy(true);
    try {
      const payload = await requestAdmin<AdminUsersResponse>("GET", internalSecret, clerkUserId);
      setDefaultQuotaBytes(payload.defaultQuotaBytes);
      setMaxQuotaBytes(payload.maxQuotaBytes);
      setUsers(payload.users ?? []);
      setMessage({ kind: "ok", text: `Loaded ${payload.users?.length ?? 0} users.` });
    } catch (error) {
      setMessage({ kind: "error", text: error instanceof Error ? error.message : "Could not load users." });
    } finally {
      setIsBusy(false);
    }
  }

  async function updateUser(userId: string, body: Record<string, unknown>) {
    setIsBusy(true);
    try {
      const payload = await requestAdmin<AdminUserResponse>("PATCH", internalSecret, clerkUserId, { userId, ...body });
      setDefaultQuotaBytes(payload.defaultQuotaBytes);
      setMaxQuotaBytes(payload.maxQuotaBytes);
      setUsers((current) => current.map((user) => (user.id === payload.user.id ? payload.user : user)));
      setMessage({ kind: "ok", text: "User updated." });
    } catch (error) {
      setMessage({ kind: "error", text: error instanceof Error ? error.message : "Could not update user." });
    } finally {
      setIsBusy(false);
    }
  }

  return (
    <main className="flex h-screen min-h-[520px] flex-col overflow-hidden bg-muted/40 text-foreground">
      <header className="flex min-h-14 items-center justify-between gap-3 border-b bg-background/95 px-4 backdrop-blur">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-card text-muted-foreground shadow-sm">
            <Shield className="size-4" />
          </div>
          <div className="min-w-0">
            <h1 className="truncate text-sm font-semibold tracking-normal">Admin</h1>
            <p className="truncate text-xs text-muted-foreground">Codex session storage per user</p>
          </div>
        </div>
        <Badge variant="secondary" className="hidden sm:inline-flex">
          {users.length} users
        </Badge>
      </header>

      <section className="min-h-0 flex-1 overflow-auto p-4 pdf-scrollbar">
        <div className="mx-auto flex max-w-6xl flex-col gap-4">
          <section className="grid gap-3 rounded-lg border bg-card p-3 shadow-sm md:grid-cols-[minmax(0,1fr)_minmax(220px,320px)_auto]">
            <label className="grid gap-1.5 text-xs font-semibold text-muted-foreground">
              Internal secret
              <input
                className="h-11 min-w-0 rounded-md border bg-background px-3 text-base text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring md:text-sm"
                type="password"
                value={internalSecret}
                autoComplete="off"
                placeholder="X-Moodle-Internal-Secret"
                onChange={(event) => setInternalSecret(event.target.value)}
              />
            </label>
            <label className="grid gap-1.5 text-xs font-semibold text-muted-foreground">
              Your Clerk user ID
              <input
                className="h-11 min-w-0 rounded-md border bg-background px-3 text-base text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring md:text-sm"
                value={clerkUserId}
                autoComplete="off"
                placeholder="user_..."
                onChange={(event) => setClerkUserId(event.target.value)}
              />
            </label>
            <Button className="self-end rounded-full" disabled={isBusy} onClick={loadUsers}>
              <RefreshCw className={cn("size-4", isBusy && "animate-spin")} />
              Load users
            </Button>
          </section>

          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">Default: {formatBytes(defaultQuotaBytes)}</Badge>
            <Badge variant="secondary">Max: {formatBytes(maxQuotaBytes)}</Badge>
            <Badge variant="secondary">Loaded usage: {formatBytes(loadedUsageBytes)}</Badge>
          </div>

          <StatusMessage message={message} />

          <section className="overflow-hidden rounded-lg border bg-card shadow-sm">
            <div className="grid grid-cols-[minmax(240px,1.4fr)_0.8fr_1fr_140px] gap-3 border-b px-4 py-3 text-xs font-bold uppercase tracking-wide text-muted-foreground max-lg:hidden">
              <span>User</span>
              <span>Usage</span>
              <span>Quota</span>
              <span>Admin</span>
            </div>
            {users.length === 0 ? (
              <EmptyUsers />
            ) : (
              <div className="divide-y">
                {users.map((user) => (
                  <UserQuotaRow
                    key={user.id}
                    user={user}
                    maxQuotaBytes={maxQuotaBytes}
                    isBusy={isBusy}
                    onSetQuota={(quotaBytes) => updateUser(user.id, { codexStateQuotaBytes: quotaBytes })}
                    onResetQuota={() => updateUser(user.id, { resetCodexStateQuota: true })}
                    onToggleAdmin={(isAdmin) => updateUser(user.id, { isAdmin })}
                  />
                ))}
              </div>
            )}
          </section>
        </div>
      </section>
    </main>
  );
}

function UserQuotaRow({
  user,
  maxQuotaBytes,
  isBusy,
  onSetQuota,
  onResetQuota,
  onToggleAdmin,
}: {
  user: AdminUser;
  maxQuotaBytes: number;
  isBusy: boolean;
  onSetQuota: (quotaBytes: number) => void;
  onResetQuota: () => void;
  onToggleAdmin: (isAdmin: boolean) => void;
}) {
  const [quotaMiB, setQuotaMiB] = useState(() => Math.round(user.codexStateQuotaBytes / 1024 / 1024).toString());
  const title = user.displayName || `Moodle user ${user.moodleUserId}`;
  const maxQuotaMiB = Math.floor(maxQuotaBytes / 1024 / 1024);

  useEffect(() => {
    setQuotaMiB(Math.round(user.codexStateQuotaBytes / 1024 / 1024).toString());
  }, [user.codexStateQuotaBytes]);

  function saveQuota() {
    const value = Number(quotaMiB);
    if (!Number.isFinite(value) || value <= 0) return;
    onSetQuota(Math.round(value * 1024 * 1024));
  }

  return (
    <article className="grid gap-3 px-4 py-4 lg:grid-cols-[minmax(240px,1.4fr)_0.8fr_1fr_140px] lg:items-center">
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{title}</p>
        <p className="break-words text-xs text-muted-foreground">{[user.clerkUserId, user.moodleSiteUrl].filter(Boolean).join(" · ")}</p>
      </div>
      <Metric label="Usage" value={`${formatBytes(user.codexStateUsageBytes)} · ${user.codexStateSnapshotCount} snapshots`} />
      <div className="grid gap-2 sm:grid-cols-[minmax(100px,140px)_auto_auto]">
        <input
          className="h-10 min-w-0 rounded-md border bg-background px-3 text-base text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring md:text-sm"
          type="number"
          min={1}
          max={maxQuotaMiB}
          value={quotaMiB}
          onChange={(event) => setQuotaMiB(event.target.value)}
        />
        <Button variant="secondary" className="rounded-full" disabled={isBusy} onClick={saveQuota}>
          Save MiB
        </Button>
        <Button variant="outline" className="rounded-full" disabled={isBusy} onClick={onResetQuota}>
          {user.codexStateQuotaConfiguredByDefault ? "Default" : "Reset"}
        </Button>
      </div>
      <label className="inline-flex items-center gap-2 text-sm font-semibold">
        <input
          className="size-4 accent-foreground"
          type="checkbox"
          checked={user.isAdmin}
          disabled={isBusy}
          onChange={(event) => onToggleAdmin(event.target.checked)}
        />
        Admin
      </label>
    </article>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1 lg:block">
      <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground lg:hidden">{label}</span>
      <span className="text-sm">{value}</span>
    </div>
  );
}

function StatusMessage({ message }: { message: Message }) {
  const Icon = message.kind === "error" ? AlertCircle : message.kind === "ok" ? CheckCircle2 : Users;
  return (
    <div
      className={cn(
        "flex items-center gap-2 text-sm font-medium",
        message.kind === "error" && "text-destructive",
        message.kind === "ok" && "text-emerald-700 dark:text-emerald-300",
        message.kind === "idle" && "text-muted-foreground",
      )}
    >
      <Icon className="size-4" />
      <span>{message.text}</span>
    </div>
  );
}

function EmptyUsers() {
  return (
    <div className="flex min-h-40 items-center justify-center p-8 text-center text-sm text-muted-foreground">
      No users loaded yet.
    </div>
  );
}

async function requestAdmin<T>(method: "GET" | "PATCH", internalSecret: string, clerkUserId: string, body?: unknown): Promise<T> {
  const response = await fetch(endpoint, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-Moodle-Internal-Secret": internalSecret.trim(),
      "X-Clerk-User-Id": clerkUserId.trim(),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(typeof payload.error === "string" ? payload.error : "Admin request failed.");
  }
  return payload as T;
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes)) return "-";
  const units = ["B", "KiB", "MiB", "GiB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const precision = value >= 10 || unit === 0 ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unit]}`;
}
