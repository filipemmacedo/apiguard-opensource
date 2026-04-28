import { execFileSync } from "child_process";

const AUTH_URL = "http://localhost:3001";
const MAILPIT_API = "http://localhost:8025/api/v1";
export const PROXY_URL = "http://localhost:8080";

export type AuthTokens = {
  access_token: string;
  refresh_token: string;
};

export type UserProfile = {
  id: number;
  name: string;
  email: string;
  avatar_url: string;
  email_verified: boolean;
  created_at: string;
  updated_at: string;
};

export type E2EUser = {
  name: string;
  email: string;
  password: string;
  orgName?: string;
};

/**
 * Fetch the most recent 6-digit verification code for an email from Mailpit.
 * Retries up to 5 times with a 2-second delay between attempts.
 */
export async function getVerificationCodeFromMailpit(
  email: string
): Promise<string> {
  for (let attempt = 0; attempt < 5; attempt++) {
    const res = await fetch(
      `${MAILPIT_API}/search?query=to:${encodeURIComponent(email)}`
    );
    if (res.ok) {
      const data = await res.json();
      if (data.messages && data.messages.length > 0) {
        const snippet: string = data.messages[0].Snippet ?? "";
        const match = snippet.match(/\b(\d{6})\b/);
        if (match) return match[1];
      }
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  throw new Error(`Could not find verification code for ${email} in Mailpit`);
}

/**
 * Delete all messages in Mailpit so subsequent code lookups are clean.
 */
export async function clearMailpit(): Promise<void> {
  await fetch(`${MAILPIT_API}/messages`, { method: "DELETE" }).catch(() => {});
}

/**
 * Register a user via the auth API directly and return the user info.
 */
export async function registerUser(
  email: string,
  password: string,
  name: string,
  orgName?: string
): Promise<{ id: number; email: string }> {
  const body: Record<string, string> = { name, email, password };
  if (orgName) body.org_name = orgName;

  const res = await fetch(`${AUTH_URL}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (res.status === 409) {
    return { id: 0, email };
  }
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Register failed (${res.status}): ${text}`);
  }
  const json = await res.json();
  return json.data;
}

/**
 * Login via auth API directly and return tokens.
 */
export async function loginUser(
  email: string,
  password: string
): Promise<AuthTokens> {
  const res = await fetch(`${AUTH_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Login failed (${res.status}): ${text}`);
  }
  const json = await res.json();
  return json.data;
}

export async function canLogin(
  email: string,
  password: string
): Promise<boolean> {
  try {
    await loginUser(email, password);
    return true;
  } catch {
    return false;
  }
}

/**
 * Verify email via auth API directly.
 */
export async function verifyEmail(code: string): Promise<void> {
  const res = await fetch(`${AUTH_URL}/auth/verify-email`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Verify email failed (${res.status}): ${text}`);
  }
}

export async function resendVerification(email: string): Promise<void> {
  const res = await fetch(`${AUTH_URL}/auth/resend-verification`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Resend verification failed (${res.status}): ${text}`);
  }
}

export async function ensureVerifiedUser(user: E2EUser): Promise<void> {
  if (await canLogin(user.email, user.password)) {
    return;
  }

  await clearMailpit();

  const result = await registerUser(
    user.email,
    user.password,
    user.name,
    user.orgName
  );

  if (result.id === 0) {
    await resendVerification(user.email);
  }

  const code = await getVerificationCodeFromMailpit(user.email);
  await verifyEmail(code);

  if (!(await canLogin(user.email, user.password))) {
    throw new Error(`User ${user.email} could not log in after verification`);
  }
}

export async function getUserProfile(accessToken: string): Promise<UserProfile> {
  const res = await fetch(`${AUTH_URL}/me`, {
    headers: {
      Authorization: `Bearer ${accessToken}`,
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Get profile failed (${res.status}): ${text}`);
  }
  const json = await res.json();
  return json.data;
}

export async function proxyFetch(
  path: string,
  init?: RequestInit
): Promise<Response> {
  return fetch(`${PROXY_URL}${path}`, init);
}

export async function createProxyLog(
  rawAPIKey: string,
  requestID: string,
  body: Record<string, unknown> = {}
): Promise<Response> {
  return proxyFetch("/v1/chat/completions", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${rawAPIKey}`,
      "Content-Type": "application/json",
      "X-Request-ID": requestID,
    },
    body: JSON.stringify(body),
  });
}

export function runPostgresSQL(sql: string): string {
  return execFileSync(
    "docker",
    [
      "compose",
      "--env-file",
      ".env.compose",
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "apiguard",
      "-d",
      "apiguard",
      "-t",
      "-A",
      "-c",
      sql,
    ],
    {
      encoding: "utf-8",
    }
  ).trim();
}

export function setSuperAdminByEmail(email: string): void {
  runPostgresSQL(
    `UPDATE goorg_users SET is_super_admin = true WHERE email = ${sqlString(
      email
    )};`
  );
}

export function setUserRoleByEmail(email: string, role: string): void {
  runPostgresSQL(
    `
UPDATE goorg_members AS members
SET role = ${sqlString(role)}
FROM goorg_users AS users
WHERE members.user_id = users.id
  AND users.email = ${sqlString(email)};
`
  );
}

function sqlString(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}
