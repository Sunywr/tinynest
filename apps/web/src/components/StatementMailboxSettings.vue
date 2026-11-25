<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ApiError, request } from '../services/api'
import { formatMinor } from '../services/format'

type MailboxConfig = {
  enabled: boolean
  masked_email?: string
  version: number
  key_ready: boolean
}

type BindingCard = { id: string; name: string; status: string; bound: boolean; taken: boolean }
type MailPreview = {
  received_on?: string
  statement_on?: string
  due_on?: string
  amount_due_minor?: number
  needs_review?: string
}
type SyncItem = { statement_on: string; due_on: string; result: string }
type SyncResult = {
  cardName: string
  since: string
  created: number
  linked: number
  conflicts: number
  needs_review: number
  items: SyncItem[]
}

const path = '/credit-statement-mailbox'
const props = defineProps<{ provider: 'qq' | '163' }>()
const providerQuery = props.provider === '163' ? '?provider=163' : ''
const providerName = props.provider === '163' ? '163' : 'QQ'
const config = ref<MailboxConfig | null>(null)
const cards = ref<BindingCard[] | null>(null)
const preview = ref<{ cardName: string; since: string; items: MailPreview[] } | null>(null)
const sync = ref<SyncResult | null>(null)
const cardGroups = computed(() => [
  { label: '已绑定信用卡', items: cards.value?.filter(card => card.bound) ?? [] },
  { label: '未绑定信用卡', items: cards.value?.filter(card => !card.bound) ?? [] },
])

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
      preview.value = null
      sync.value = null
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
    preview.value = null
    sync.value = null
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
    preview.value = null
    sync.value = null
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
    preview.value = null
    sync.value = null
    message.value = card.bound ? '已解除信用卡绑定。' : '信用卡已绑定到当前邮箱。'
    cards.value = null
    await loadCards()
  })
}

function previewStatements(card: BindingCard) {
  const current = config.value
  if (!current?.enabled || !card.bound || card.status !== 'active') return
  return run(async () => {
    preview.value = null
    sync.value = null
    const query = new URLSearchParams({ version: String(current.version), provider: props.provider })
    const endpoint = `/accounts/${encodeURIComponent(card.id)}/credit-statement-preview?${query}`
    const result = await request<{ since: string; items: MailPreview[] }>(endpoint, { method: 'POST' })
    preview.value = { cardName: card.name, ...result }
  })
}

// Syncing stores what the bank reported. It writes no transactions and no
// journal entries, so the repayment plan on the assets page stays untouched.
function syncStatements(card: BindingCard) {
  const current = config.value
  if (!current?.enabled || !card.bound || card.status !== 'active') return
  return run(async () => {
    preview.value = null
    sync.value = null
    const query = new URLSearchParams({ version: String(current.version), provider: props.provider })
    const endpoint = `/accounts/${encodeURIComponent(card.id)}/credit-statement-sync?${query}`
    const result = await request<Omit<SyncResult, 'cardName'>>(endpoint, { method: 'POST' })
    sync.value = { cardName: card.name, ...result }
    message.value = providerName + ' 账单已同步（搜索自 ' + result.since + '），未新增流水。'
  })
}

function syncLine(item: SyncItem): string {
  if (item.result === 'needs_review') return '有邮件未能识别，未入库'
  const period = item.statement_on ? item.statement_on + ' 期' : '该账单'
  if (item.result === 'created') return item.due_on ? period + '已入库，到期 ' + item.due_on : period + '已入库'
  if (item.result === 'conflict') return period + '金额与已记录不一致，保留首次金额'
  if (item.result === 'linked' || item.result === 'unchanged') return period + '已存在，仅新增邮件来源'
  return period + '未入库'
}

onMounted(refresh)
</script>

<template>
  <section class="card settings-card" aria-label="信用卡账单邮箱">
    <h2>信用卡账单邮箱</h2>
    <p class="form-hint">浦发和中信账单支持只读预览。</p>
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
        <template v-else>
          <section v-for="group in cardGroups" :key="group.label" class="binding-group">
            <h3>{{ group.label }}</h3>
            <p v-if="!group.items.length" class="form-hint">暂无{{ group.label }}。</p>
            <ul v-else :aria-label="group.label">
              <li v-for="card in group.items" :key="card.id">
                <span>{{ card.name }}<small v-if="card.status === 'disabled'"> · 已停用</small></span>
                <small v-if="card.taken">已绑定其他邮箱</small>
                <div v-else class="mailbox-actions card-actions">
                  <button v-if="card.bound && card.status === 'active'" class="btn-quiet" type="button"
                    :disabled="busy" @click="syncStatements(card)">同步账单</button>
                  <button v-if="card.bound && card.status === 'active'" class="btn-quiet" type="button"
                    :disabled="busy" @click="previewStatements(card)">预览账单</button>
                  <button :class="card.bound ? 'btn-danger' : 'btn-primary'" type="button"
                    :disabled="busy" @click="updateBinding(card)">{{ card.bound ? '解绑' : '绑定' }}</button>
                </div>
              </li>
            </ul>
          </section>
        </template>
      </div>
      <section v-if="preview" class="mailbox-preview" aria-label="账单预览" aria-live="polite">
        <h3>{{ preview.cardName }} · 账单预览</h3>
        <p class="form-hint">搜索范围从 {{ preview.since }} 开始。预览不会新增流水。</p>
        <p v-if="!preview.items.length">没有找到符合条件的邮件。</p>
        <ul v-else>
          <li v-for="(item, index) in preview.items" :key="index">
            <span v-if="item.statement_on">账单日 {{ item.statement_on }}　</span>
            <span v-else-if="item.received_on">邮件日期 {{ item.received_on }}　</span>
            <span v-if="item.due_on">到期日 {{ item.due_on }}　</span>
            <strong v-if="item.amount_due_minor !== undefined">应还 {{ formatMinor(item.amount_due_minor) }}</strong>
            <p v-if="item.needs_review" class="form-hint">{{ item.needs_review }}</p>
          </li>
        </ul>
      </section>
      <section v-if="sync" class="mailbox-preview" aria-label="账单同步结果">
        <h3>{{ sync.cardName }} · 账单同步结果</h3>
        <p class="sync-counts">新增 {{ sync.created }} 期，重复 {{ sync.linked }} 期，冲突 {{ sync.conflicts }} 期，待核对 {{ sync.needs_review }} 期</p>
        <p v-if="!sync.items.length" class="form-hint">没有找到符合条件的账单邮件。</p>
        <ul v-else>
          <li v-for="(item, index) in sync.items" :key="index">{{ syncLine(item) }}</li>
        </ul>
        <p class="form-hint">同步只记录银行报送的账单汇总，不会新增流水或改变还款计划。</p>
      </section>
      <form :key="config.version" class="mailbox-form" @submit.prevent="save">
        <label><span>{{ providerName }} 邮箱</span><input v-model="email" type="email" maxlength="254" required :disabled="busy" @input="message = ''" /></label>
        <label><span>邮箱授权码</span><input v-model="authCode" type="password" autocomplete="off" maxlength="128" required :disabled="busy" /></label>
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
  width: min(100%, 30rem);
}
.mailbox-form label {
  display: grid;
  gap: 6px;
  min-width: 0;
}
.mailbox-form input {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
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
.binding-group + .binding-group {
  border-top: 1px solid var(--border);
  padding-top: 12px;
}
.binding-group h3 {
  font-size: 1rem;
  margin: 0 0 6px;
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
.card-actions {
  justify-content: flex-end;
}
.mailbox-preview {
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow-wrap: anywhere;
}
.sync-counts {
  margin: 0;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
</style>
