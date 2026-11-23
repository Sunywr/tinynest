<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Check, Copy, Link2, UserRound, X } from 'lucide-vue-next'
import { api, type Invitation, type JoinApplication, type ManagedUser, type Member, type Role } from '../services/api'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const emit = defineEmits<{ close: []; saved: [] }>()
const session = useSession()
const { toast } = useToast()

const members = ref<Member[]>([])
const invitations = ref<Invitation[]>([])
const applications = ref<JoinApplication[]>([])
const users = ref<ManagedUser[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const newName = ref('')
const targetUsername = ref('')
const bindMemberID = ref('')
const bindUsername = ref('')
const issued = ref<{ code: string; memberName: string } | null>(null)
const confirmDelete = ref('')
const copied = ref('')

const isAdmin = computed(() => session.isAdmin.value)
const unbound = computed(() => members.value.filter(member => !member.bound && member.status === 'active'))
const unboundUsers = computed(() => users.value.filter(user => user.status === 'active' && !user.member_name))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [memberResult, inviteResult, applicationResult, userResult] = await Promise.all([
      api.members(),
      isAdmin.value ? api.invitations() : Promise.resolve({ items: [] as Invitation[] }),
      isAdmin.value ? api.joinApplications() : Promise.resolve({ items: [] as JoinApplication[] }),
      isAdmin.value ? api.users() : Promise.resolve({ items: [] as ManagedUser[] }),
    ])
    members.value = memberResult.items
    invitations.value = inviteResult.items
    applications.value = applicationResult.items
    users.value = userResult.items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '成员加载失败'
  } finally {
    loading.value = false
  }
}

async function refreshAll() {
  await load()
  await session.refreshHousehold()
  emit('saved')
}

async function issueForMember(member: Member) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  issued.value = null
  try {
    const result = await api.issueInvite({ member_id: member.id })
    issued.value = { code: result.code, memberName: member.name }
    await refreshAll()
    toast('邀请码已生成')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '生成邀请码失败'
  } finally {
    busy.value = false
  }
}

async function inviteNewMember() {
  const name = newName.value.trim()
  if (!name || busy.value) return
  busy.value = true
  error.value = ''
  issued.value = null
  try {
    const result = await api.issueInvite({
      member_name: name,
      username: targetUsername.value.trim() || undefined,
    })
    issued.value = { code: result.code, memberName: name }
    newName.value = ''
    targetUsername.value = ''
    await refreshAll()
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
    await refreshAll()
    toast('邀请已撤销')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '撤销失败'
  } finally {
    busy.value = false
  }
}

async function bindExisting() {
  if (!bindMemberID.value || !bindUsername.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.bindMember({ member_id: bindMemberID.value, username: bindUsername.value })
    bindMemberID.value = ''
    bindUsername.value = ''
    await refreshAll()
    toast('已有账号已绑定到成员')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '绑定失败'
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
    await refreshAll()
    toast(approve ? '已通过申请，家庭成员可登录了' : '已拒绝申请')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '审批失败'
  } finally {
    busy.value = false
  }
}

async function removeMember(member: Member) {
  if (confirmDelete.value !== member.id) {
    confirmDelete.value = member.id
    return
  }
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.deleteMember(member.id)
    confirmDelete.value = ''
    await refreshAll()
    toast('成员已删除')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除失败，请重试'
  } finally {
    busy.value = false
  }
}

/** Admins manage other members' roles; a member's own role is not editable here. */
function canManageRole(member: Member): boolean {
  return isAdmin.value && member.bound === true && member.status === 'active' &&
    member.id !== session.me.value?.member_id &&
    (member.user_status ?? 'active') === 'active' && Boolean(member.user_id)
}

async function changeRole(member: Member, event: Event) {
  const select = event.target as HTMLSelectElement
  const next = select.value as Role
  const previous = (member.role as Role) || 'member'
  if (next === previous) return
  if (busy.value || !member.user_id) {
    select.value = previous
    return
  }
  busy.value = true
  error.value = ''
  const self = member.id === session.me.value?.member_id
  try {
    await api.updateUserRole(member.user_id, next)
    await session.refreshMe()
    await load()
    await session.refreshHousehold()
    emit('saved')
    toast(self && next !== 'admin' ? '已更新自己的角色，管理入口已收起' : '角色已更新')
  } catch (e) {
    select.value = previous
    error.value = e instanceof Error ? e.message : '角色更新失败'
  } finally {
    busy.value = false
  }
}

function inviteLink(code: string): string {
  return `${window.location.origin}/login?invite=${encodeURIComponent(code)}`
}

async function copyInvite(code: string) {
  const text = inviteLink(code)
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const area = document.createElement('textarea')
    area.value = text
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    document.execCommand('copy')
    area.remove()
  }
  copied.value = code
  toast('邀请链接已复制')
  window.setTimeout(() => { if (copied.value === code) copied.value = '' }, 2000)
}

function roleLabel(role?: string): string {
  if (role === 'admin') return '管理员'
  if (role === 'viewer') return '只读'
  return '成员'
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet wide-sheet" role="dialog" aria-modal="true" aria-labelledby="members-title">
      <header class="sheet-header">
        <h2 id="members-title" class="sheet-title">家庭成员</h2>
        <button type="button" class="icon-btn" aria-label="关闭成员管理" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p class="form-hint">
          邀请对方注册或使用已有账号加入家庭；批准后共享同一家庭的账户、流水、预算与资产视图。
          当前登录：{{ session.currentMember.value?.name || session.me.value?.username || '未绑定' }}。
        </p>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载成员…</p>

        <template v-else>
          <section aria-label="成员列表">
            <h3 class="inline-form-title">成员</h3>
            <p v-if="isAdmin" class="form-hint">可调整其他已加入成员的角色；管理员不能修改自己的角色。</p>
            <ul class="editor-list">
              <li v-for="member in members" :key="member.id" class="editor-row member-row">
                <span class="editor-emoji" aria-hidden="true"><UserRound :size="16" /></span>
                <span class="member-body">
                  <span class="member-name-line">
                    <span class="editor-name">{{ member.name }}</span>
                    <span v-if="member.id === session.me.value?.member_id" class="editor-tag">当前</span>
                    <span v-if="member.bound" class="editor-tag">已加入</span>
                    <span v-else class="editor-tag muted">待邀请</span>
                  </span>
                  <span class="member-meta">
                    <template v-if="member.bound">
                      登录账号 {{ member.username }}<template v-if="!canManageRole(member)"> · {{ roleLabel(member.role) }}</template>
                    </template>
                    <template v-else>尚未绑定登录账号；生成邀请码后由对方注册或加入。</template>
                  </span>
                </span>
                <span class="editor-actions">
                  <select v-if="canManageRole(member)" class="member-role" :value="member.role || 'member'"
                    :disabled="busy" :aria-label="`${member.name} 的角色`" @change="changeRole(member, $event)">
                    <option value="admin">管理员</option>
                    <option value="member">成员</option>
                    <option value="viewer">只读</option>
                  </select>
                  <button v-if="isAdmin && !member.bound && member.status === 'active'" type="button" class="mini-btn"
                    :disabled="busy" @click="issueForMember(member)">邀请加入</button>
                  <button v-if="isAdmin && !member.bound && member.id !== session.me.value?.member_id" type="button"
                    class="mini-btn danger" :disabled="busy" @click="removeMember(member)">
                    {{ confirmDelete === member.id ? '确认删除' : '删除' }}
                  </button>
                </span>
              </li>
              <li v-if="!members.length" class="editor-empty">还没有成员</li>
            </ul>
          </section>

          <form v-if="isAdmin" class="inline-form" @submit.prevent="inviteNewMember">
            <h3 class="inline-form-title">邀请新成员</h3>
            <label class="field-label">成员名称
              <input v-model="newName" maxlength="100" required placeholder="例如：妈妈" :disabled="busy" />
            </label>
            <label class="field-label">对方登录名（可选）
              <input v-model="targetUsername" maxlength="64" autocomplete="off"
                placeholder="填写后邀请码只能由该用户名使用" :disabled="busy" />
            </label>
            <p class="form-hint">不填写登录名时，任何拿到邀请码的待审批账号都可以加入。邀请码 48 小时内有效，仍需管理员批准。</p>
            <div class="form-actions">
              <button type="submit" class="btn-primary" :disabled="busy || !newName.trim()">
                {{ busy ? '生成中…' : '生成邀请码' }}
              </button>
            </div>
          </form>
          <p v-else class="form-hint">只有家庭管理员可以生成邀请码或审批加入申请。</p>

          <form v-if="isAdmin && unbound.length && unboundUsers.length" class="inline-form" @submit.prevent="bindExisting">
            <h3 class="inline-form-title">绑定已有账号</h3>
            <p class="form-hint">本家庭中已注册但尚未绑定成员的账号，可直接绑定到待邀请成员；不会创建新账号。</p>
            <label class="field-label">绑定成员
              <select v-model="bindMemberID" :disabled="busy" required>
                <option value="" disabled>选择待邀请成员</option>
                <option v-for="member in unbound" :key="member.id" :value="member.id">{{ member.name }}</option>
              </select>
            </label>
            <label class="field-label">已有账号
              <select v-model="bindUsername" :disabled="busy" required>
                <option value="" disabled>选择未绑定账号</option>
                <option v-for="user in unboundUsers" :key="user.id" :value="user.username">
                  {{ user.username }}（{{ roleLabel(user.role) }}）
                </option>
              </select>
            </label>
            <div class="form-actions">
              <button type="submit" class="btn-primary" :disabled="busy || !bindMemberID || !bindUsername">
                {{ busy ? '绑定中…' : '绑定账号' }}
              </button>
            </div>
          </form>

          <section v-if="issued" class="invite-output" aria-label="新邀请" role="status">
            <p class="invite-output-head"><Check :size="16" /> {{ issued.memberName }} 的邀请码</p>
            <code class="invite-code">{{ issued.code }}</code>
            <p class="invite-link"><Link2 :size="13" /> <span>{{ inviteLink(issued.code) }}</span></p>
            <button type="button" class="mini-btn" @click="copyInvite(issued.code)">
              <Copy :size="13" /> {{ copied === issued.code ? '已复制' : '复制邀请链接' }}
            </button>
            <p class="form-hint">把链接发给对方：新用户注册时自动带入邀请码；已注册但未加入的账号可从登录页选择“用邀请码加入”。</p>
          </section>

          <section v-if="isAdmin" aria-label="邀请记录">
            <h3 class="inline-form-title">待接受邀请</h3>
            <ul class="editor-list">
              <li v-for="invitation in invitations" :key="invitation.id" class="editor-row">
                <span class="editor-name">{{ invitation.member_name }} → {{ invitation.target_username || '任何待审批账号' }}</span>
                <span class="editor-tag muted">{{ invitation.expires_at.slice(0, 10) }} 截止</span>
                <span class="editor-actions">
                  <button type="button" class="mini-btn danger" :disabled="busy" @click="revoke(invitation)">撤销</button>
                </span>
              </li>
              <li v-if="!invitations.length" class="editor-empty">暂无待接受邀请</li>
            </ul>
          </section>

          <section v-if="isAdmin" aria-label="加入申请">
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

          <p v-if="isAdmin && !unbound.length" class="form-hint">所有成员都已绑定登录账号；新成员可通过上方邀请加入。</p>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.member-row { align-items: flex-start; }
.member-body { flex: 1; min-width: 0; display: grid; gap: 3px; }
.member-name-line { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.member-meta { color: var(--muted); font-size: 11px; line-height: 1.5; }
.member-role {
  flex: none;
  min-height: 32px;
  max-width: 100%;
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: 9px;
  background: var(--card);
  color: var(--primary-deep);
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  line-height: 1.4;
}
.member-role:disabled { opacity: .5; cursor: not-allowed; }
.invite-output { display: grid; gap: 7px; padding: 12px; border: 1px solid var(--border); border-radius: 14px; background: var(--soft); }
.invite-output-head { margin: 0; display: flex; align-items: center; gap: 6px; color: var(--primary-deep); font-size: 13px; font-weight: 700; }
.invite-link { margin: 0; display: flex; align-items: flex-start; gap: 4px; color: var(--muted); font-size: 10px; line-height: 1.5; overflow-wrap: anywhere; }
.invite-link span { min-width: 0; overflow-wrap: anywhere; }
.invite-output .mini-btn { justify-self: start; display: inline-flex; align-items: center; gap: 4px; }
@media (max-width: 560px) {
  .member-row { flex-wrap: wrap; }
  .member-row .editor-actions { margin-inline-start: 0; flex-basis: 100%; padding-inline-start: 40px; }
}
</style>
