import { expect, test } from "@playwright/test";
import {
  createProxyLog,
  ensureVerifiedUser,
  getUserProfile,
  loginUser,
  proxyFetch,
  setSuperAdminByEmail,
  setUserRoleByEmail,
  type AuthTokens,
  type E2EUser,
  type UserProfile,
} from "./helpers";

const ADMIN_USER: E2EUser = {
  name: "Admin E2E",
  email: "admin-e2e@test.com",
  password: "AdminPass123!",
  orgName: "Admin E2E Org",
};

const runID = Date.now().toString(36);

const OWNER_USER: E2EUser = {
  name: "Identity Owner",
  email: `identity-owner-${runID}@test.com`,
  password: "OwnerPass123!",
  orgName: `Identity Owner Org ${runID}`,
};

const MEMBER_USER: E2EUser = {
  name: "Identity Member",
  email: `identity-member-${runID}@test.com`,
  password: "MemberPass123!",
  orgName: `Identity Member Org ${runID}`,
};

const tenantKeysPath = "/internal/admin/api-management/tenant-keys";

type ErrorResponse = {
  error: string;
};

type ManagedKey = {
  id: number;
  user_id: string;
  display_name: string;
  goorg_user_id?: string;
  raw_api_key: string;
};

type ManagedKeyCreateResponse = {
  user_key: Omit<ManagedKey, "raw_api_key">;
  raw_api_key: string;
};

type DashboardMeResponse = {
  user_id: string;
  key_alias: string;
};

type DashboardLogsResponse = {
  user_id: string;
  logs: Array<{ request_id: string }>;
};

type DashboardUsageResponse = {
  user_id: string;
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
};

type OverviewResponse = {
  users: Array<{ user_id: string }>;
};

test.describe("Dashboard identity scoping", () => {
  test.describe.configure({ mode: "serial" });
  test.setTimeout(120_000);

  let adminTokens: AuthTokens;
  let ownerTokens: AuthTokens;
  let memberTokens: AuthTokens;
  let ownerProfile: UserProfile;
  let memberProfile: UserProfile;
  let ownerKey!: ManagedKey;
  let memberKey!: ManagedKey;

  test.beforeAll(async () => {
    await ensureVerifiedUser(ADMIN_USER);
    setSuperAdminByEmail(ADMIN_USER.email);
    adminTokens = await loginUser(ADMIN_USER.email, ADMIN_USER.password);

    await ensureVerifiedUser(OWNER_USER);
    ownerTokens = await loginUser(OWNER_USER.email, OWNER_USER.password);
    ownerProfile = await getUserProfile(ownerTokens.access_token);

    await ensureVerifiedUser(MEMBER_USER);
    setUserRoleByEmail(MEMBER_USER.email, "member");
    memberTokens = await loginUser(MEMBER_USER.email, MEMBER_USER.password);
    memberProfile = await getUserProfile(memberTokens.access_token);
  });

  test("requires JWT plus a linked managed key for dashboard routes", async () => {
    const noToken = await proxyFetch("/internal/me");
    expect(noToken.status).toBe(401);
    await expectJSON(noToken, { error: "unauthorized" });

    const noLinkedKey = await proxyFetch("/internal/dashboard/logs", {
      headers: authHeaders(memberTokens.access_token),
    });
    expect(noLinkedKey.status).toBe(403);
    await expectJSON(noLinkedKey, {
      error: "No managed key found for this account",
    });
  });

  test("supports goorg user linking on create and patch", async () => {
    ownerKey = await createManagedKey(adminTokens.access_token, {
      display_name: `owner-link-${runID}`,
      goorg_user_id: ownerProfile.id,
    });
    expect(ownerKey.goorg_user_id).toBe(String(ownerProfile.id));
    expect(ownerKey.raw_api_key).toMatch(/^agtk_/);

    const ownerMe = await proxyFetch("/internal/me", {
      headers: authHeaders(ownerTokens.access_token),
    });
    expect(ownerMe.status).toBe(200);
    await expectJSON<DashboardMeResponse>(ownerMe, {
      user_id: ownerKey.user_id,
      key_alias: ownerKey.display_name,
    });

    memberKey = await createManagedKey(adminTokens.access_token, {
      display_name: `member-link-${runID}`,
    });
    expect(memberKey.goorg_user_id).toBeUndefined();

    const memberBeforePatch = await proxyFetch("/internal/me", {
      headers: authHeaders(memberTokens.access_token),
    });
    expect(memberBeforePatch.status).toBe(403);

    const patchResponse = await proxyFetch(`${tenantKeysPath}/${memberKey.id}`, {
      method: "PATCH",
      headers: jsonHeaders(adminTokens.access_token),
      body: JSON.stringify({ goorg_user_id: memberProfile.id }),
    });
    expect(patchResponse.status).toBe(200);
    await expectJSON(patchResponse, { status: "updated" });

    const memberMe = await proxyFetch("/internal/me", {
      headers: authHeaders(memberTokens.access_token),
    });
    expect(memberMe.status).toBe(200);
    await expectJSON<DashboardMeResponse>(memberMe, {
      user_id: memberKey.user_id,
      key_alias: memberKey.display_name,
    });
  });

  test("returns only the linked user's logs and usage", async () => {
    const otherKey = await createManagedKey(adminTokens.access_token, {
      display_name: `other-log-source-${runID}`,
    });

    const ownerRequestID = `owner-${runID}`;
    const otherRequestID = `other-${runID}`;

    const ownerLog = await createProxyLog(ownerKey.raw_api_key, ownerRequestID, {});
    expect(ownerLog.status).toBe(400);

    const otherLog = await createProxyLog(otherKey.raw_api_key, otherRequestID, {});
    expect(otherLog.status).toBe(400);

    const logsResponse = await proxyFetch("/internal/dashboard/logs", {
      headers: authHeaders(ownerTokens.access_token),
    });
    expect(logsResponse.status).toBe(200);
    const logsPayload = await parseJSON<DashboardLogsResponse>(logsResponse);
    expect(logsPayload.user_id).toBe(ownerKey.user_id);
    const requestIDs = logsPayload.logs.map((log) => log.request_id);
    expect(requestIDs).toContain(ownerRequestID);
    expect(requestIDs).not.toContain(otherRequestID);

    const usageResponse = await proxyFetch("/internal/dashboard/usage", {
      headers: authHeaders(ownerTokens.access_token),
    });
    expect(usageResponse.status).toBe(200);
    const usagePayload = await parseJSON<DashboardUsageResponse>(usageResponse);
    expect(usagePayload.user_id).toBe(ownerKey.user_id);
  });

  test("keeps overview routes admin-only and ignores playground user_id in the body", async () => {
    for (const path of [
      "/internal/dashboard/overview/logs",
      "/internal/dashboard/overview/usage",
    ]) {
      const memberOverview = await proxyFetch(path, {
        headers: authHeaders(memberTokens.access_token),
      });
      expect(memberOverview.status).toBe(403);
      await expectJSON<ErrorResponse>(memberOverview, { error: "forbidden" });

      const ownerOverview = await proxyFetch(path, {
        headers: authHeaders(ownerTokens.access_token),
      });
      expect(ownerOverview.status).toBe(200);
      const ownerPayload = await parseJSON<OverviewResponse>(ownerOverview);
      expect(Array.isArray(ownerPayload.users)).toBe(true);

      const adminOverview = await proxyFetch(path, {
        headers: authHeaders(adminTokens.access_token),
      });
      expect(adminOverview.status).toBe(200);
      const adminPayload = await parseJSON<OverviewResponse>(adminOverview);
      expect(Array.isArray(adminPayload.users)).toBe(true);
    }

    const playgroundResponse = await proxyFetch("/internal/dashboard/playground", {
      method: "POST",
      headers: jsonHeaders(ownerTokens.access_token),
      body: JSON.stringify({
        user_id: "someone-else",
        model: "",
        prompt: "",
      }),
    });
    expect(playgroundResponse.status).toBe(400);
    await expectJSON<ErrorResponse>(playgroundResponse, {
      error: "model and prompt are required",
    });
  });
});

async function createManagedKey(
  adminToken: string,
  input: { display_name: string; goorg_user_id?: number }
): Promise<ManagedKey> {
  const response = await proxyFetch(tenantKeysPath, {
    method: "POST",
    headers: jsonHeaders(adminToken),
    body: JSON.stringify(input),
  });
  expect(response.status).toBe(201);

  const payload = await parseJSON<ManagedKeyCreateResponse>(response);
  return {
    ...payload.user_key,
    raw_api_key: payload.raw_api_key,
  };
}

function authHeaders(token: string): Record<string, string> {
  return {
    Authorization: `Bearer ${token}`,
  };
}

function jsonHeaders(token: string): Record<string, string> {
  return {
    ...authHeaders(token),
    "Content-Type": "application/json",
  };
}

async function parseJSON<T>(response: Response): Promise<T> {
  return (await response.json()) as T;
}

async function expectJSON<T extends object>(
  response: Response,
  expected: Partial<T>
): Promise<void> {
  const payload = await parseJSON<T>(response);
  expect(payload).toMatchObject(expected);
}
