<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { request } from '../services/api'

type Option = { id: string; name: string }
type Mailbox = {
  id: string; host: string; username: string; folder: string
  enabled: boolean; version: number; tested_at: string
}
type Binding = {
  instrument_id: string; mailbox_id: string; category_id: string
  subject_keyword: string; card_tail: string
}

defineProps<{ cards: Option[]; categories: Option[] }>()
defineEmits<{ close: [] }>()
const fresh = () => ({ id: '', host: '', username: '', folder: 'INBOX', enabled: true, version: 0, tested_at: '', password: '' })
const box = ref(fresh())
const boxes = ref<Mailbox[]>([])
const bindings = ref<Binding[]>([])
const keyReady = ref(false)
const loaded = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const binding = ref<Binding>({ instrument_id: '', mailbox_id: '', category_id: '', subject_keyword: '', card_tail: '' })
const available = computed(() => boxes.value.filter(item => item.enabled || item.id === binding.value.mailbox_id))

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  notice.value = ''
  try { await action() }
  catch (err) { error.value = err instanceof Error ? err.message : '操作失败' }
  finally { busy.value = false }
}
async function load() {
  const data = await request<{ mailboxes: Mailbox[]; bindings: Binding[]; key_ready: boolean }>('/credit-mail')
  boxes.value = data.mailboxes
  bindings.value = data.bindings
  keyReady.value = data.key_ready
  loaded.value = true
}
onMounted(() => void run(load))

function edit(item?: Mailbox) {
  box.value = item ? { ...item, password: '' } : fresh()
}

function saveMailbox(enabled = true) {
  void run(async () => {
    const id = box.value.id
    const item = await request<Mailbox>(id ? '/credit-mail/mailboxes/' + encodeURIComponent(id) : '/credit-mail/mailboxes', {
      method: id ? 'PUT' : 'POST',
      body: JSON.stringify({ ...box.value, enabled, password: enabled ? box.value.password : '' }),
    })
    edit(item)
    await load()
    notice.value = enabled ? '连接测试通过，邮箱已保存' : '邮箱已停用'
  })
}

function selectCard() {
  const id = binding.value.instrument_id
  const current = bindings.value.find(item => item.instrument_id === id)
  binding.value = current ? { ...current } : { instrument_id: id, mailbox_id: '', category_id: '', subject_keyword: '', card_tail: '' }
}

function saveBinding() {
  void run(async () => {
    await request('/credit-mail/bindings/' + encodeURIComponent(binding.value.instrument_id), {
      method: 'PUT', body: JSON.stringify(binding.value),
    })
    await load()
    selectCard()
    notice.value = '信用卡绑定已保存'
  })
}
</script>

<template>
  <section class="panel formpanel credit-mail-settings">
    <div class="paneltitle">
      <h2>账单邮箱</h2>
      <button type="button" class="ghost" @click="$emit('close')">返回列表</button>
    </div>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
    <p v-if="notice" class="subinfo" role="status">{{ notice }}</p>
    <button v-if="!loaded && !busy" type="button" class="ghost" @click="run(load)">重试</button>
    <p v-if="loaded && !keyReady" role="status">服务器尚未配置账单邮箱加密密钥。</p>
    <template v-if="loaded">
      <form @submit.prevent="saveMailbox()">
        <fieldset :disabled="busy">
          <legend>{{ box.id ? '编辑邮箱' : '新增邮箱' }}</legend>
          <div v-if="boxes.length" class="actions-bar">
            <button type="button" class="ghost" @click="edit()">新增邮箱</button>
            <button v-for="item in boxes" :key="item.id" type="button" class="ghost" @click="edit(item)">
              {{ item.username }} · {{ item.enabled ? '已启用' : '已停用' }}
            </button>
          </div>
          <div class="formgrid">
            <label>IMAP 主机（TLS 993）<input v-model.trim="box.host" required maxlength="253" placeholder="imap.example.com"></label>
            <label>邮箱账号<input v-model.trim="box.username" required maxlength="320" autocomplete="off"></label>
            <label>文件夹<input v-model.trim="box.folder" maxlength="255" placeholder="INBOX"></label>
            <label>授权码<input v-model="box.password" type="password" autocomplete="new-password" maxlength="1024" :required="!box.id" :placeholder="box.id ? '留空沿用已保存的授权码' : ''"></label>
          </div>
          <p v-if="box.tested_at">上次连接通过 {{ new Date(box.tested_at).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) }}</p>
          <div class="actions-bar">
            <button class="primary" type="submit" :disabled="!keyReady">测试并保存</button>
            <button v-if="box.id && box.enabled" type="button" class="ghost" @click="saveMailbox(false)">停用邮箱</button>
          </div>
        </fieldset>
      </form>
      <form @submit.prevent="saveBinding">
        <fieldset :disabled="busy">
          <legend>信用卡绑定</legend>
          <div class="formgrid">
            <label>信用卡<select v-model="binding.instrument_id" required @change="selectCard">
              <option value="">请选择</option><option v-for="card in cards" :key="card.id" :value="card.id">{{ card.name }}</option>
            </select></label>
            <label>账单邮箱<select v-model="binding.mailbox_id">
              <option value="">不绑定</option><option v-for="item in available" :key="item.id" :value="item.id" :disabled="!item.enabled">{{ item.username }}{{ item.enabled ? '' : '（已停用）' }}</option>
            </select></label>
            <template v-if="binding.mailbox_id">
              <label>标题关键字<input v-model.trim="binding.subject_keyword" required maxlength="255"></label>
              <label>卡尾号<input v-model.trim="binding.card_tail" required inputmode="numeric" pattern="[0-9]{4}" maxlength="4"></label>
              <label>默认支出分类<select v-model="binding.category_id" required>
                <option value="">请选择</option><option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</option>
              </select></label>
            </template>
          </div>
          <button class="primary" type="submit" :disabled="!binding.instrument_id">保存绑定</button>
        </fieldset>
      </form>
    </template>
  </section>
</template>

<style scoped>
fieldset { min-width: 0; border: 0; margin: 0 0 1rem; padding: 0; }
legend { font-weight: 700; padding: 0; }
form + form { padding-top: 18px; border-top: 1px solid var(--border); }
.actions-bar button { max-width: 100%; overflow-wrap: anywhere; }
.actions-bar .primary { width: auto; margin-top: 0; }
</style>
