#!/usr/bin/env node
'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const REPO_ROOT = path.resolve(__dirname, '..');
const ENV_FILE = path.join(REPO_ROOT, '.env.compose');
const ENV_EXAMPLE = path.join(REPO_ROOT, '.env.compose.example');

const AUTH_URL = 'http://localhost:3001';
const MAILPIT_API = 'http://localhost:8025/api/v1';

const ADMIN_USER = {
  name: 'Admin',
  email: 'admin@apiguard.local',
  password: 'AdminPass123!',
  orgName: 'APIGuard Admin',
};

const DASHBOARD_USER = {
  name: 'Dashboard User',
  email: 'dash@apiguard.local',
  password: 'DashPass123!',
  orgName: 'Dashboard Dev',
};

const COLORS = {
  blue: '\x1b[1;34m',
  green: '\x1b[1;32m',
  yellow: '\x1b[1;33m',
  red: '\x1b[1;31m',
  reset: '\x1b[0m',
};

function info(message) {
  console.log(`${COLORS.blue}==>${COLORS.reset} ${message}`);
}

function ok(message) {
  console.log(`${COLORS.green}  [ok]${COLORS.reset} ${message}`);
}

function warn(message) {
  console.log(`${COLORS.yellow}  [!]${COLORS.reset} ${message}`);
}

function fail(message) {
  throw new Error(message);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function randomHex(bytes) {
  return crypto.randomBytes(bytes).toString('hex');
}

function replaceLine(content, pattern, replacement) {
  if (!pattern.test(content)) {
    return content;
  }
  pattern.lastIndex = 0;
  return content.replace(pattern, replacement);
}

function ensureTrailingNewline(content) {
  return content.endsWith('\n') ? content : `${content}\n`;
}

function ensureEnvFile() {
  info('Checking environment file');

  if (fs.existsSync(ENV_FILE)) {
    ok('.env.compose already exists - keeping it');
    return;
  }

  if (!fs.existsSync(ENV_EXAMPLE)) {
    fail('.env.compose.example not found');
  }

  let content = fs.readFileSync(ENV_EXAMPLE, 'utf8').replace(/\r\n/g, '\n');
  content = content.replaceAll('change-me-access-secret-at-least-32-chars', randomHex(32));
  content = content.replaceAll('change-me-refresh-secret-at-least-32-chars', randomHex(32));
  content = content.replaceAll('change-me-before-real-use', randomHex(32));
  content = replaceLine(content, /^GOORG_MODE=.*$/m, 'GOORG_MODE=multi_tenant');
  content = replaceLine(content, /^GOORG_EMAIL_PROVIDER=.*$/m, 'GOORG_EMAIL_PROVIDER=smtp');
  content = replaceLine(content, /^# GOORG_SMTP_HOST=.*$/m, 'GOORG_SMTP_HOST=mailpit');
  content = replaceLine(content, /^# GOORG_SMTP_PORT=.*$/m, 'GOORG_SMTP_PORT=1025');
  content = replaceLine(content, /^# GOORG_SMTP_USER=.*$/m, 'GOORG_SMTP_USER=');
  content = replaceLine(content, /^# GOORG_SMTP_PASSWORD=.*$/m, 'GOORG_SMTP_PASSWORD=');
  content = replaceLine(content, /^# GOORG_SMTP_FROM=.*$/m, 'GOORG_SMTP_FROM=noreply@apiguard.local');

  if (!/^GOORG_FRONTEND_URL=/m.test(content)) {
    content = `${ensureTrailingNewline(content)}GOORG_FRONTEND_URL=http://localhost:3000\n`;
  }

  fs.writeFileSync(ENV_FILE, content, 'utf8');
  ok('Created .env.compose with generated secrets');
}

function runCommand(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: REPO_ROOT,
    stdio: 'inherit',
    shell: false,
    ...options,
  });

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`Command failed: ${command} ${args.join(' ')}`);
  }
}

function captureCommand(command, args) {
  const result = spawnSync(command, args, {
    cwd: REPO_ROOT,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
    shell: false,
  });

  if (result.error) {
    throw result.error;
  }
  return result;
}

function request(method, rawUrl, { headers = {}, body, timeoutMs = 2000 } = {}) {
  return new Promise((resolve, reject) => {
    const url = new URL(rawUrl);
    const transport = url.protocol === 'https:' ? https : http;
    const payload = body == null
      ? null
      : Buffer.from(typeof body === 'string' ? body : JSON.stringify(body), 'utf8');

    const req = transport.request(
      {
        protocol: url.protocol,
        hostname: url.hostname,
        port: url.port,
        path: `${url.pathname}${url.search}`,
        method,
        headers: {
          ...(payload ? { 'Content-Length': String(payload.length) } : {}),
          ...headers,
        },
      },
      (res) => {
        const chunks = [];
        res.on('data', (chunk) => chunks.push(chunk));
        res.on('end', () => {
          resolve({
            status: res.statusCode || 0,
            headers: res.headers,
            text: Buffer.concat(chunks).toString('utf8'),
          });
        });
      },
    );

    req.setTimeout(timeoutMs, () => {
      req.destroy(new Error(`Timed out waiting for ${rawUrl}`));
    });
    req.on('error', reject);

    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

function printComposeDebug(serviceName) {
  console.log('');
  info('Docker Compose status');
  const ps = captureCommand('docker', ['compose', '--env-file', ENV_FILE, 'ps', '-a']);
  if (ps.stdout) {
    process.stdout.write(ps.stdout);
  }
  if (ps.stderr) {
    process.stderr.write(ps.stderr);
  }

  if (!serviceName) {
    return;
  }

  console.log('');
  info(`Recent ${serviceName} logs`);
  const logs = captureCommand('docker', ['compose', '--env-file', ENV_FILE, 'logs', serviceName, '--tail=80']);
  if (logs.stdout) {
    process.stdout.write(logs.stdout);
  }
  if (logs.stderr) {
    process.stderr.write(logs.stderr);
  }
}

async function waitForUrl(url, { timeoutSeconds = 120, serviceName } = {}) {
  const deadline = Date.now() + timeoutSeconds * 1000;

  while (Date.now() < deadline) {
    try {
      await request('GET', url, { timeoutMs: 2000 });
      return;
    } catch {
      await sleep(3000);
    }
  }

  printComposeDebug(serviceName);
  fail(`Timed out waiting for ${url}`);
}

async function tryLogin(email, password) {
  try {
    const response = await request('POST', `${AUTH_URL}/auth/login`, {
      timeoutMs: 3000,
      headers: { 'Content-Type': 'application/json' },
      body: { email, password },
    });
    return response.status === 200;
  } catch {
    return false;
  }
}

async function mailpitDeleteMessages() {
  try {
    await request('DELETE', `${MAILPIT_API}/messages`, { timeoutMs: 3000 });
  } catch {
    // Mailpit cleanup is best effort.
  }
}

async function mailpitGetCode(email) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    const response = await request('GET', `${MAILPIT_API}/search?query=${encodeURIComponent(`to:${email}`)}`, {
      timeoutMs: 3000,
    });
    const matches = response.text.match(/\b\d{6}\b/g);
    if (matches && matches.length > 0) {
      return matches[matches.length - 1];
    }
    await sleep(2000);
  }

  fail(`Could not get verification code for ${email} from Mailpit`);
}

async function registerAndVerify(user) {
  if (await tryLogin(user.email, user.password)) {
    ok(`${user.email} already set up - skipping`);
    return;
  }

  await mailpitDeleteMessages();

  const registerResponse = await request('POST', `${AUTH_URL}/auth/register`, {
    timeoutMs: 5000,
    headers: { 'Content-Type': 'application/json' },
    body: {
      name: user.name,
      email: user.email,
      password: user.password,
      org_name: user.orgName,
    },
  });

  if (registerResponse.status === 409) {
    warn(`${user.email} already registered but login failed - attempting password reset`);
    await request('POST', `${AUTH_URL}/auth/forgot-password`, {
      timeoutMs: 5000,
      headers: { 'Content-Type': 'application/json' },
      body: { email: user.email },
    });
    await sleep(1000);
    const resetCode = await mailpitGetCode(user.email);
    await request('POST', `${AUTH_URL}/auth/reset-password`, {
      timeoutMs: 5000,
      headers: { 'Content-Type': 'application/json' },
      body: {
        email: user.email,
        code: resetCode,
        new_password: user.password,
      },
    });
    ok(`${user.email} password reset`);
    return;
  }

  if (registerResponse.status !== 200 && registerResponse.status !== 201) {
    fail(`Register ${user.email} failed with status ${registerResponse.status}`);
  }
  ok(`Registered ${user.email}`);

  await sleep(1000);
  const code = await mailpitGetCode(user.email);
  ok(`Got verification code for ${user.email}`);

  await request('POST', `${AUTH_URL}/auth/verify-email`, {
    timeoutMs: 5000,
    headers: { 'Content-Type': 'application/json' },
    body: { code },
  });
  ok(`Verified ${user.email}`);
}

function setSuperAdmin(email) {
  runCommand('docker', [
    'compose',
    '--env-file',
    ENV_FILE,
    'exec',
    '-T',
    'postgres',
    'psql',
    '-U',
    'apiguard',
    '-d',
    'apiguard',
    '-c',
    `UPDATE goorg_users SET is_super_admin = true WHERE email = '${email}';`,
  ]);
  ok(`Super admin flag set for ${email}`);
}

async function main() {
  ensureEnvFile();

  info('Starting Docker Compose stack');
  runCommand('docker', ['compose', '--env-file', ENV_FILE, 'up', '--build', '-d']);

  info('Waiting for services to become healthy');

  process.stdout.write('  Waiting for auth service...');
  await waitForUrl(`${AUTH_URL}/healthz`, { timeoutSeconds: 120, serviceName: 'auth' });
  process.stdout.write(' ready\n');

  process.stdout.write('  Waiting for Mailpit...');
  await waitForUrl('http://localhost:8025', { timeoutSeconds: 60, serviceName: 'mailpit' });
  process.stdout.write(' ready\n');

  process.stdout.write('  Waiting for API Guard...');
  await waitForUrl('http://localhost:8080', { timeoutSeconds: 120, serviceName: 'api-guard' });
  process.stdout.write(' ready\n');

  process.stdout.write('  Waiting for Dashboard...');
  await waitForUrl('http://localhost:3000', { timeoutSeconds: 120, serviceName: 'dashboard' });
  process.stdout.write(' ready\n');

  ok('All services are up');

  info('Setting up admin user');
  await registerAndVerify(ADMIN_USER);
  setSuperAdmin(ADMIN_USER.email);

  info('Setting up dashboard user');
  await registerAndVerify(DASHBOARD_USER);

  console.log('');
  info('Setup complete!');
  console.log('');
  console.log('  Services:');
  console.log('    Dashboard        http://localhost:3000');
  console.log('    Admin Panel      http://localhost:3002');
  console.log('    Auth API         http://localhost:3001');
  console.log('    API Guard        http://localhost:8080');
  console.log('    Mailpit (email)  http://localhost:8025');
  console.log('');
  console.log('  Credentials:');
  console.log(`    Admin:     ${ADMIN_USER.email} / ${ADMIN_USER.password}`);
  console.log(`    Dashboard: ${DASHBOARD_USER.email} / ${DASHBOARD_USER.password}`);
  console.log('');
  console.log('  Useful commands:');
  console.log('    npm run dev:logs    Follow service logs');
  console.log('    npm run dev:down    Stop all services');
  console.log('    npm run dev:build   Rebuild and restart');
  console.log('');
}

main().catch((error) => {
  console.error(`${COLORS.red}  [x]${COLORS.reset} ${error.message}`);
  process.exitCode = 1;
});
