<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { X } from 'lucide-vue-next'
import SmartSelect from './SmartSelect.vue'
import { api, type Invitation, type JoinApplication, type Member } from '../services/api'
import { useToast } from '../composables/useToast'

const emit = defineEmits<{ close: [] }>()
const { toast } = useToast()

const invitations = ref<Invitation[]>([])
const applications = ref<JoinApplication[]>([])
const members = ref<Member[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const issued = ref('')
const draft = reactive({ member_id: '', member_name: '', username: '' })

const memberOptions = computed(() => members.value.filter(m => m.status === 'active').map(m => ({ id: m.id, name: m.name })))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [invitationResult, applicationResult, memberResult] = await Promise.all([
      api.invitations(), api.joinApplications(), api.members(),
    ])
    invitations.value = invitationResult.items
    applications.value = applicationResult.items
    members.value = memberResult.items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '邀请数据加载失败'
  } finally {
    loading.value = false
  }
}

async function issue() {
  const member = members.value.find(m => m.id === draft.member_id)
  if (!draft.member_id || !draft.username.trim() || busy.value) return
  busy.value = true
  error.value = ''
  issued.value = ''
  try {
    const result = await api.issueInvite({
      member_id: draft.member_id,
      member_name: member?.name || draft.member_name || draft.username.trim(),
      username: draft.username.trim(),
    })
    issued.value = result.code
    draft.username = ''
    await load()
    toast('邀请码已生成')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '生成邀请码失败'
  } finally {
    busy.value = false
  }
}

async function revoke(invitation: Invitation) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.revokeInvitation(invitation.id)
    await load()
    toast('邀请已撤销')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '撤销失败'
  } finally {
    busy.value = false
  }
}

async function decide(application: JoinApplication, approve: boolean) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.decideApplication(application.id, approve)
    await load()
    toast(approve ? '已通过申请' : '已拒绝申请')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '审批失败'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet wide-sheet" role="dialog" aria-modal="true" aria-labelledby="invite-title">
      <header class="sheet-header">
        <h2 id="invite-title" class="sheet-title">邀请与审批</h2>
        <button type="button" class="icon-btn" aria-label="关闭邀请与审批" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载…</p>

        <form class="inline-form" @submit.prevent="issue">
          <h3 class="inline-form-title">生成家庭邀请码</h3>
          <div class="form-row">
            <span class="field-label">对应成员</span>
            <SmartSelect v-model="draft.member_id" :options="memberOptions" placeholder="选择成员档案" :disabled="busy" />
          </div>
          <label class="field-label">受邀人用户名
            <input v-model="draft.username" required maxlength="64" placeholder="受邀人注册后使用的用户名" :disabled="busy" />
          </label>
          <p v-if="issued" class="inline-notice" role="status">邀请码：<strong class="invite-code">{{ issued }}</strong>（请通过安全渠道转交）</p>
          <div class="form-actions">
            <button type="submit" class="btn-primary" :disabled="busy || !draft.member_id || !draft.username.trim()">
              生成邀请码
            </button>
          </div>
        </form>

        <section aria-label="待处理邀请">
          <h3 class="inline-form-title">邀请记录</h3>
          <ul class="editor-list">
            <li v-for="invitation in invitations" :key="invitation.id" class="editor-row">
              <span class="editor-name">{{ invitation.member_name }} → {{ invitation.target_username }}</span>
              <span class="editor-tag muted">{{ invitation.expires_at }}</span>
              <span class="editor-actions">
                <button type="button" class="mini-btn danger" :disabled="busy" @click="revoke(invitation)">撤销</button>
              </span>
            </li>
            <li v-if="!invitations.length" class="editor-empty">暂无有效邀请</li>
          </ul>
        </section>

        <section aria-label="加入申请">
          <h3 class="inline-form-title">加入申请</h3>
          <ul class="editor-list">
            <li v-for="application in applications" :key="application.id" class="editor-row">
              <span class="editor-name">{{ application.username }}（{{ application.member_name }}）</span>
              <span class="editor-actions">
                <button type="button" class="mini-btn" :disabled="busy" @click="decide(application, true)">通过</button>
                <button type="button" class="mini-btn danger" :disabled="busy" @click="decide(application, false)">拒绝</button>
              </span>
            </li>
            <li v-if="!applications.length" class="editor-empty">暂无待审批申请</li>
          </ul>
        </section>
      </div>
    </section>
  </div>
</template>
