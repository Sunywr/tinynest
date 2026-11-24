<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ApiError, request } from '../services/api'

type MailboxConfig = {
  enabled: boolean
  masked_email?: string
  version: number
  key_ready: boolean
}

const path = '/credit-statement-mailbox'
const config = ref<MailboxConfig | null>(null)

const email = ref('')
const authCode = ref('')
const busy = ref(false)
const message = ref('')
const error = ref('')

function credentials() {
  const input = { email: email.value.trim(), auth_code: authCode.value }
  authCode.value = ''
  return input
}

function jsonBody(value: object) {
  return new Blob([JSON.stringify(value)], { type: 'application/json' })
}

async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  message.value = ''
  error.value = ''
  try {
    await action()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '邮箱操作失败'
    if (cause instanceof ApiError && cause.status === 409) {
      config.value = null
    }
  } finally {
    busy.value = false
  }
}

function refresh() {
  return run(async () => { config.value = await request<MailboxConfig>(path) })
}

function testConnection(savedVersion?: number) {
  return run(async () => {
    const input = savedVersion === undefined ? credentials() : { version: savedVersion }
    await request(path + '/test', { method: 'POST', body: jsonBody(input) })
    message.value = 'QQ 邮箱连接成功，收件箱可读。'
  })
}

function save() {
  const current = config.value
  if (!current?.key_ready) return
  return run(async () => {
    config.value = await request<MailboxConfig>(path, {
      method: 'PUT',
      body: jsonBody({ ...credentials(), version: current.version }),
    })
    email.value = ''
    message.value = 'QQ 邮箱配置已保存。'
  })
}

function disable() {
  const current = config.value
  if (!current?.enabled) return
  return run(async () => {
    config.value = await request<MailboxConfig>(path, {
      method: 'DELETE',
      body: jsonBody({ version: current.version }),
    })
    authCode.value = ''
    email.value = ''
    message.value = 'QQ 邮箱已停用，授权码已清除。'
  })
}

onMounted(refresh)
</script>

<template>
  <section class="card settings-card" aria-label="信用卡账单邮箱">
    <h2>信用卡账单邮箱</h2>
    <p class="form-hint">可配置 QQ 邮箱。信用卡绑定和账单导入尚未开放。</p>
    <template v-if="config">
      <p v-if="config.enabled">已启用 · {{ config.masked_email }}</p>
      <p v-else>尚未启用</p>
      <p v-if="!config.key_ready" class="form-hint">服务端配置邮件加密密钥后即可保存授权码。</p>
      <div v-if="config.enabled" class="mailbox-actions">
        <button class="btn-quiet" type="button" :disabled="busy || !config.key_ready" @click="testConnection(config.version)">测试已保存邮箱</button>
        <button class="btn-quiet" type="button" :disabled="busy" @click="disable">停用邮箱</button>
      </div>
      <form :key="config.version" class="mailbox-form" @submit.prevent="save">
        <label>QQ 邮箱<input v-model="email" type="email" maxlength="254" required :disabled="busy" @input="message = ''" /></label>
        <label>邮箱授权码<input v-model="authCode" type="password" autocomplete="off" maxlength="128" required :disabled="busy" /></label>
        <div class="mailbox-actions">
          <button class="btn-quiet" type="button" :disabled="busy || !email.trim() || !authCode" @click="testConnection()">测试连接</button>
          <button class="btn-primary" type="submit" :disabled="busy || !config.key_ready">{{ config.enabled ? '保存新授权码' : '验证并保存' }}</button>
        </div>
      </form>
    </template>
    <button v-else class="btn-quiet" type="button" :disabled="busy" @click="refresh">{{ busy ? '读取中…' : '刷新邮箱配置' }}</button>
    <p v-if="message" role="status">{{ message }}</p>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.mailbox-form {
  display: grid;
  gap: 12px;
}
.mailbox-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.mailbox-actions + .mailbox-form {
  margin-top: 12px;
}
</style>
