<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChevronRight } from 'lucide-vue-next'
import AdminInvites from '../components/AdminInvites.vue'
import AdminUsers from '../components/AdminUsers.vue'
import AuditLogs from '../components/AuditLogs.vue'
import BudgetEditor from '../components/BudgetEditor.vue'
import CategoryEditor from '../components/CategoryEditor.vue'
import ConfirmSheet from '../components/ConfirmSheet.vue'
import ExportMenu from '../components/ExportMenu.vue'
import MembersEditor from '../components/MembersEditor.vue'
import UserProfileDialog from '../components/UserProfileDialog.vue'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'
import { monthKey, monthLabel, monthRange } from '../services/format'
import type { UserProfile } from '../services/api'

const router = useRouter()
const session = useSession()
const { toast } = useToast()

const profileOpen = ref(false)
const membersOpen = ref(false)
const categoryOpen = ref(false)
const budgetOpen = ref(false)
const exportOpen = ref(false)
const invitesOpen = ref(false)
const usersOpen = ref(false)
const auditOpen = ref(false)
const logoutOpen = ref(false)
const loggingOut = ref(false)

const me = computed(() => session.me.value)
const profile = computed<UserProfile>(() => ({
  username: me.value?.username || '',
  member_name: me.value?.member_name || session.currentMember.value?.name,
  avatar: me.value?.avatar,
  email: me.value?.email,
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
      <button v-if="canWrite" type="button" class="me-menu-item" @click="budgetOpen = true">
        <span>预算设置</span><span class="me-menu-meta">{{ monthLabel(month) }} <ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="exportOpen = true">
        <span>导出账单</span><span class="me-menu-meta">xlsx <ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="openSettings">
        <span>邮箱与密码</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
      <div class="me-household">
        <span>家庭</span>
        <strong>{{ session.household.value?.name || '—' }}</strong>
        <span>{{ session.household.value?.currency || 'CNY' }} · {{ session.household.value?.timezone || 'Asia/Shanghai' }}</span>
      </div>
    </nav>

    <nav v-if="isAdmin" class="me-menu admin-menu" aria-label="管理员菜单">
      <h2 class="section-title">管理员</h2>
      <button type="button" class="me-menu-item" @click="invitesOpen = true">
        <span>邀请与审批</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="usersOpen = true">
        <span>用户管理</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
      <button type="button" class="me-menu-item" @click="auditOpen = true">
        <span>审计日志</span><span class="me-menu-meta"><ChevronRight :size="16" /></span>
      </button>
    </nav>

    <button type="button" class="btn-danger logout-btn" @click="logoutOpen = true">退出登录</button>

    <UserProfileDialog v-if="profileOpen" :profile="profile" @close="profileOpen = false"
      @saved="profileOpen = false; toast('资料已保存')" />
    <MembersEditor v-if="membersOpen" @close="membersOpen = false" />
    <CategoryEditor v-if="categoryOpen" @close="categoryOpen = false" />
    <BudgetEditor v-if="budgetOpen" :month="month" @close="budgetOpen = false" @saved="budgetOpen = false" />
    <ExportMenu v-if="exportOpen" :from="from" :to="to" @close="exportOpen = false" />
    <AdminInvites v-if="invitesOpen" @close="invitesOpen = false" />
    <AdminUsers v-if="usersOpen" @close="usersOpen = false" />
    <AuditLogs v-if="auditOpen" @close="auditOpen = false" />
    <ConfirmSheet :open="logoutOpen" title="要退出登录吗？" message="退出后需要重新登录才能查看家庭账目。"
      confirm-label="退出登录" danger :busy="loggingOut"
      @confirm="logout" @cancel="logoutOpen = false" />
  </section>
</template>
