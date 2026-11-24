<script setup lang="ts">
import SurfaceTransition from "../components/SurfaceTransition.vue"
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChevronRight } from 'lucide-vue-next'
import AdminUsers from '../components/AdminUsers.vue'
import AuditLogs from '../components/AuditLogs.vue'
import CategoryEditor from '../components/CategoryEditor.vue'
import ConfirmSheet from '../components/ConfirmSheet.vue'
import ExportMenu from '../components/ExportMenu.vue'
import MembersEditor from '../components/MembersEditor.vue'
import UserProfileDialog from '../components/UserProfileDialog.vue'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'
import { monthKey, monthRange } from '../services/format'
import { api, type UserProfile } from '../services/api'

const router = useRouter()
const session = useSession()
const { toast } = useToast()

const profileOpen = ref(false)
const membersOpen = ref(false)
const categoryOpen = ref(false)
const exportOpen = ref(false)
const usersOpen = ref(false)
const auditOpen = ref(false)
const logoutOpen = ref(false)
const loggingOut = ref(false)
const householdName = ref<string | null>(null)
const householdBusy = ref(false)
const archiveOpen = ref(false)
const archiveID = ref('')
const householdError = ref('')

async function saveHousehold() {
  const current = session.household.value
  if (!current || householdName.value === null || householdBusy.value) return
  householdBusy.value = true
  householdError.value = ''
  try {
    const updated = await api.updateHousehold(current.id, householdName.value.trim())
    if (session.household.value?.id === updated.id) {
      session.household.value = { ...current, name: updated.name }
    }
    householdName.value = null
    toast('家庭设置已保存')
  } catch (error) {
    householdError.value = error instanceof Error ? error.message : '保存失败，请重试'
  } finally {
    householdBusy.value = false
  }
}

async function archiveHousehold() {
  if (!archiveID.value || householdBusy.value) return
  householdBusy.value = true
  householdError.value = ''
  try {
    await api.archiveHousehold(archiveID.value)
    archiveOpen.value = false
    householdName.value = null
    toast('家庭已删除，历史账本已归档')
    await session.refreshMe()
  } catch (error) {
    archiveOpen.value = false
    householdError.value = error instanceof Error ? error.message : '删除失败，请重试'
  } finally {
    householdBusy.value = false
  }
}

const me = computed(() => session.me.value)
const profile = computed<UserProfile>(() => ({
  username: me.value?.username || '',
  member_name: me.value?.member_name || session.currentMember.value?.name,
  avatar: me.value?.avatar,
  email: me.value?.email,
  birthday: (me.value?.birthday as string | undefined) || '',
  payday: (me.value?.payday as number | undefined) ?? undefined,
}))
const month = monthKey()
const [from, to] = monthRange(month)
const canWrite = computed(() => session.canWrite.value)
const isAdmin = computed(() => session.isAdmin.value)

function openSettings() {
  void router.push('/me/settings')
}

async function logout() {
  if (loggingOut.value) return
  loggingOut.value = true
  try {
    await session.logout()
    toast('已退出登录')
    logoutOpen.value = false
  } finally {
    loggingOut.value = false
  }
}

async function profileSaved() {
  profileOpen.value = false
  toast('资料已保存')
  await session.refreshMe()
}
</script>

<template>
  <section class="page me-page">
    <header class="page-head">
      <div>
        <h1 class="page-title">我的</h1>
        <p class="page-sub">{{ session.household.value?.name || '我的小家' }} · 家庭账本</p>
      </div>
    </header>

    <section class="card me-profile" aria-label="账户信息">
      <span class="me-avatar" aria-hidden="true">
        <img v-if="me?.avatar?.startsWith?.('data:image/jpeg;base64,')" :src="me.avatar" alt="" />
        <template v-else>{{ me?.avatar || '♡' }}</template>
      </span>
      <span class="me-identity">
        <strong>{{ me?.member_name || me?.username }}</strong>
        <span>{{ me?.username }} · {{ me?.role === 'admin' ? '管理员' : me?.role === 'viewer' ? '只读成员' : '家庭成员' }}</span>
      </span>
      <button type="button" class="mini-btn" @click="profileOpen = true">个人资料</button>
    </section>

    <p v-if="me && !me.email_verified" class="inline-notice" role="status">
      邮箱尚未验证。验证后可用于找回密码与安全提醒。
      <button type="button" class="link-btn" @click="openSettings">去验证</button>
    </p>

    <nav class="me-menu" aria-label="设置菜单">
      <button type="button" class="me-menu-item" @click="membersOpen = true">
        <span>家庭成员</span><span class="me-menu-meta">{{ session.members.value.length }} 人 <ChevronRight :size="16" /></span>
      </button>
      <button v-if="canWrite" type="button" class="me-menu-item" @click="categoryOpen = true">
        <span>分类管理</span><span class="me-menu-meta">改名 · 图标 · 排序 <ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="router.push('/budgets')">
        <span>预算设置</span><ChevronRight :size="16" />
      </button>
      <button type="button" class="me-menu-item" @click="exportOpen = true">
        <span>导出账单</span><span class="me-menu-meta">xlsx <ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="openSettings">
        <span>邮箱与密码</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
      <button v-if="isAdmin && session.household.value" type="button" class="me-menu-item"
        @click="householdName = session.household.value.name; householdError = ''">
        <span>家庭设置</span><ChevronRight :size="16" />
      </button>
      <form v-if="isAdmin && householdName !== null" class="card settings-card flow-in" @submit.prevent="saveHousehold">
        <label>家庭名称<input v-model="householdName" maxlength="100" required :disabled="householdBusy" /></label>
        <p v-if="householdError" class="err" role="alert">{{ householdError }}</p>
        <button class="btn-primary" :disabled="householdBusy || !householdName.trim()">保存</button>
        <button type="button" class="btn-quiet" :disabled="householdBusy" @click="householdName = null">取消</button>
        <button type="button" class="btn-danger" :disabled="householdBusy"
          @click="archiveID = session.household.value?.id || ''; archiveOpen = true">删除家庭</button>
      </form>
      <div class="me-household">
        <span>家庭</span>
        <strong>{{ session.household.value?.name || '—' }}</strong>
        <span>{{ session.household.value?.currency || 'CNY' }} · {{ session.household.value?.timezone || 'Asia/Shanghai' }}</span>
      </div>
    </nav>

    <nav v-if="isAdmin" class="me-menu admin-menu" aria-label="管理员菜单">
      <h2 class="section-title">管理员</h2>
      <button type="button" class="me-menu-item" @click="usersOpen = true">
        <span>用户管理</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="auditOpen = true">
        <span>审计日志</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
    </nav>

    <button type="button" class="btn-danger logout-btn" @click="logoutOpen = true">退出登录</button>

    <UserProfileDialog v-if="profileOpen" :profile="profile" @close="profileOpen = false"
      @saved="profileSaved" />
    <SurfaceTransition>
      <MembersEditor v-if="membersOpen" @close="membersOpen = false" />
    </SurfaceTransition>
    <SurfaceTransition>
      <CategoryEditor v-if="categoryOpen" @close="categoryOpen = false" />
    </SurfaceTransition>
    <SurfaceTransition>
      <ExportMenu v-if="exportOpen" :from="from" :to="to" @close="exportOpen = false" />
    </SurfaceTransition>
    <SurfaceTransition>
      <AdminUsers v-if="usersOpen" @close="usersOpen = false" />
    </SurfaceTransition>
    <SurfaceTransition>
      <AuditLogs v-if="auditOpen" @close="auditOpen = false" />
    </SurfaceTransition>
    <ConfirmSheet :open="archiveOpen" title="确认删除家庭？"
      message="历史账本将归档，所有成员将回到创建或加入家庭页面，未使用的邀请码将失效。"
      confirm-label="删除家庭" danger :busy="householdBusy"
      @confirm="archiveHousehold" @cancel="archiveOpen = false" />
    <ConfirmSheet :open="logoutOpen" title="要退出登录吗？" message="退出后需要重新登录才能查看家庭账目。"
      confirm-label="退出登录" danger :busy="loggingOut"
      @confirm="logout" @cancel="logoutOpen = false" />
  </section>
</template>
