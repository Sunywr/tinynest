import { defineConfig } from '@playwright/test'
import fs from 'node:fs'
import {randomBytes} from 'node:crypto'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
// A fresh SQLite per run: POST /api/v1/setup is single-use by design.
const dataDir = process.env.TINYNEST_E2E_RUN_DIR || path.join(os.tmpdir(), `tinynest-e2e-data-${Date.now()}`)
process.env.TINYNEST_E2E_RUN_DIR=dataDir
fs.mkdirSync(dataDir, { recursive: true })
const mailKey=process.env.TINYNEST_E2E_MAIL_KEY ? Buffer.from(process.env.TINYNEST_E2E_MAIL_KEY,'hex') : randomBytes(32)
fs.writeFileSync(path.join(dataDir,'mail.key'),mailKey,{mode:0o600})
process.env.TINYNEST_E2E_DB_PATH=path.join(dataDir,'tinynest.sqlite3')
process.env.TINYNEST_E2E_MAIL_KEY=mailKey.toString('hex')

// The Go toolchain is NOT on the global PATH of this workstation; CI provides
// it via PATH. Both are honored here without touching the app code.
const goBinDir = process.env.TINYNEST_GO_BIN_DIR || ''
const env = {
  ...process.env,
  ...(goBinDir ? { PATH: `${goBinDir}${path.delimiter}${process.env.PATH}` } : {}),
}
const setupToken = process.env.E2E_SETUP_TOKEN || 'e2e-setup-token-0123456789abcdef'

export default defineConfig({
  testDir: './e2e',
  timeout: 40_000,
  expect: { timeout: 8_000 },
  workers: 1,
  fullyParallel: false,
  reporter: [['list']],
  use: {
    browserName: process.env.TINYNEST_E2E_WEBKIT === '1' ? 'webkit' : 'chromium',
    baseURL: 'http://127.0.0.1:5173',
    timezoneId: 'Asia/Shanghai',
    channel: process.env.TINYNEST_E2E_BROWSER_CHANNEL || undefined,
    screenshot: 'only-on-failure',
    trace: 'off',
  },
  webServer: [
    {
      command: 'go run ./cmd/server',
      cwd: path.resolve(here, '../api'),
      env: {
        ...env,
        TINYNEST_LISTEN_ADDR: '127.0.0.1:8787',
        TINYNEST_DATA_DIR: dataDir,
        TINYNEST_DB_PATH: path.join(dataDir, 'tinynest.sqlite3'),
        TINYNEST_SETUP_TOKEN: setupToken,
        TINYNEST_COOKIE_SECURE: 'false',
        TINYNEST_MAIL_KEY_FILE: path.join(dataDir,'mail.key'),
        TINYNEST_TRUSTED_PROXIES: '',
      },
      url: 'http://127.0.0.1:8787/api/v1/healthz',
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'npm run dev',
      cwd: here,
      env,
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: false,
      timeout: 60_000,
    },
  ],
})
