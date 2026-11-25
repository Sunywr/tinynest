<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ApiError, request } from '../services/api'

type MailboxConfig = {
  enabled: boolean
  masked_email?: string
  version: number
  key_ready: boolean
}

type BindingCard = { id: string; name: string; status: string; bound: boolean; taken: boolean }

const path = '/credit-statement-mailbox'
const props = defineProps<{ provider: 'qq' | '163' }>()
const providerQuery = props.provider === '163' ? '?provider=163' : ''
const providerName = props.provider === '163' ? '163' : 'QQ'
const config = ref<MailboxConfig | null>(null)
const cards = ref<BindingCard[] | null>(null)

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
      cards.value = null
      if (cause.code !== 'card_already_bound') config.value = null
    }
  } finally {
    busy.value = false
  }
}

function refresh() {
  return run(async () => { config.value = await request<MailboxConfig>(path + providerQuery) })
}

function testConnection(savedVersion?: number) {
  return run(async () => {
    const input = savedVersion === undefined ? credentials() : { version: savedVersion }
    await request(path + '/test' + providerQuery, { method: 'POST', body: jsonBody(input) })
    message.value = providerName + ' 邮箱连接成功，收件箱可读。'
  })
}

function save() {
  const current = config.value
  if (!current?.key_ready) return
  return run(async () => {
    config.value = await request<MailboxConfig>(path + providerQuery, {
      method: 'PUT',
      body: jsonBody({ ...credentials(), version: current.version }),
    })
    email.value = ''
    cards.value = null
    message.value = providerName + ' 邮箱配置已保存。'
  })
}

function disable() {
  const current = config.value
  if (!current?.enabled) return
  return run(async () => {
    config.value = await request<MailboxConfig>(path + providerQuery, {
      method: 'DELETE',
      body: jsonBody({ version: current.version }),
    })
    authCode.value = ''
    email.value = ''
    cards.value = null
    message.value = providerName + ' 邮箱已停用，授权码已清除。'
  })
}

async function loadCards() {
  const result = await request<{ items: BindingCard[] }>(path + '/cards' + providerQuery)
  cards.value = result.items
}

function updateBinding(card: BindingCard) {
  const current = config.value
  if (!current?.enabled) return
  return run(async () => {
    const query = new URLSearchParams({ version: String(current.version), provider: props.provider })
    const endpoint = `/accounts/${encodeURIComponent(card.id)}/credit-statement-binding?${query}`
    await request(endpoint, { method: card.bound ? 'DELETE' : 'PUT' })
    message.value = card.bound ? '已解除信用卡绑定。' : '信用卡已绑定到当前邮箱。'
    cards.value = null
    await loadCards()
  })
}

onMounted(refresh)
</script>

<template>
  <section class="card settings-card" aria-label="信用卡账单邮箱">
    <h2>信用卡账单邮箱</h2>
    <p class="form-hint">账单拉取和导入尚未开放。</p>
    <template v-if="config">
      <p v-if="config.enabled">已启用 · {{ config.masked_email }}</p>
      <p v-else>尚未启用</p>
      <p v-if="!config.key_ready" class="form-hint">服务端配置邮件加密密钥后即可保存授权码。</p>
      <div v-if="config.enabled" class="mailbox-actions">
        <button class="btn-quiet" type="button" :disabled="busy || !config.key_ready" @click="testConnection(config.version)">测试已保存邮箱</button>
        <button class="btn-quiet" type="button" :disabled="busy" @click="run(loadCards)">信用卡绑定</button>
        <button class="btn-quiet" type="button" :disabled="busy" @click="disable">停用邮箱</button>
      </div>
      <div v-if="cards" class="binding-cards">
        <p v-if="!cards.length" class="form-hint">暂无可绑定的信用卡。</p>
        <ul v-else aria-label="信用卡绑定">
          <li v-for="card in cards" :key="card.id">
            <span>{{ card.name }}<small v-if="card.status === 'disabled'"> · 已停用</small></span>
            <small v-if="card.taken">已绑定其他邮箱</small>
            <button v-else class="btn-quiet" type="button" :disabled="busy" @click="updateBinding(card)">
              {{ card.bound ? '解绑' : '绑定' }}
            </button>
          </li>
        </ul>
      </div>
      <form :key="config.version" class="mailbox-form" @submit.prevent="save">
        <label>{{ providerName }} 邮箱<input v-model="email" type="email" maxlength="254" required :disabled="busy" @input="message = ''" /></label>
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
.mailbox-actions + .mailbox-form,
.binding-cards {
  margin-top: 12px;
}
.binding-cards ul {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
}
.binding-cards li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding-block: 4px;
}
.binding-cards li span {
  overflow-wrap: anywhere;
}
</style>
