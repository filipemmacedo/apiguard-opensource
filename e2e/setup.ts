import {
  ensureVerifiedUser,
  getUserProfile,
  loginUser,
  proxyFetch,
  setSuperAdminByEmail,
  type E2EUser,
} from "./helpers";

const ADMIN_USER: E2EUser = {
  name: "Admin E2E",
  email: "admin-e2e@test.com",
  password: "AdminPass123!",
  orgName: "Admin E2E Org",
};

const DASHBOARD_USER: E2EUser = {
  name: "Dashboard E2E",
  email: "dash-e2e@test.com",
  password: "DashPass123!",
  orgName: "Dashboard E2E Org",
};

type ManagedKeyListResponse = {
  user_keys: Array<{
    id: number;
    user_id: string;
    display_name: string;
    status: string;
    goorg_user_id?: string;
  }>;
};

async function ensureManagedKeyLinked(
  adminToken: string,
  goorgUserID: number
): Promise<void> {
  const listResponse = await proxyFetch("/internal/admin/api-management/tenant-keys", {
    headers: {
      Authorization: `Bearer ${adminToken}`,
    },
  });
  if (!listResponse.ok) {
    const text = await listResponse.text();
    throw new Error(`List tenant keys failed (${listResponse.status}): ${text}`);
  }

  const listPayload = (await listResponse.json()) as ManagedKeyListResponse;
  const linkedKey = listPayload.user_keys.find(
    (key) =>
      key.status === "active" && key.goorg_user_id === String(goorgUserID)
  );
  if (linkedKey) {
    console.log(`  Managed key already linked for goorg user ${goorgUserID}`);
    return;
  }

  const createResponse = await proxyFetch("/internal/admin/api-management/tenant-keys", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${adminToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      display_name: `dashboard-smoke-${goorgUserID}`,
      goorg_user_id: goorgUserID,
    }),
  });
  if (!createResponse.ok) {
    const text = await createResponse.text();
    throw new Error(`Create tenant key failed (${createResponse.status}): ${text}`);
  }

  console.log(`  Linked managed key for goorg user ${goorgUserID}`);
}

export async function ensureE2EUsers(): Promise<void> {
  console.log("Setting up E2E test users...");

  console.log("\n1. Ensuring admin user:");
  await ensureVerifiedUser(ADMIN_USER);
  setSuperAdminByEmail(ADMIN_USER.email);
  console.log("  Super admin flag set");
  const adminTokens = await loginUser(ADMIN_USER.email, ADMIN_USER.password);

  console.log("\n2. Ensuring dashboard user:");
  await ensureVerifiedUser(DASHBOARD_USER);
  const dashboardTokens = await loginUser(
    DASHBOARD_USER.email,
    DASHBOARD_USER.password
  );
  const dashboardProfile = await getUserProfile(dashboardTokens.access_token);
  await ensureManagedKeyLinked(adminTokens.access_token, dashboardProfile.id);

  console.log("\nSetup complete!");
  console.log(`  Admin:     ${ADMIN_USER.email} / ${ADMIN_USER.password}`);
  console.log(`  Dashboard: ${DASHBOARD_USER.email} / ${DASHBOARD_USER.password}`);
}

export default async function globalSetup(): Promise<void> {
  await ensureE2EUsers();
}

if (require.main === module) {
  ensureE2EUsers().catch((err) => {
    console.error("Setup failed:", err);
    process.exit(1);
  });
}
