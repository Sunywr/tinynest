<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { X } from 'lucide-vue-next'
import UserProfileDialog from './UserProfileDialog.vue'
import { api, type ManagedUser } from '../services/api'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const emit = defineEmits<{ close: [] }>()
const session = useSession()
const { toast } = useToast()

const users = ref<ManagedUser[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const confirmDelete = ref('')
const resetFor = ref('')
const resetPassword = reactive({ value: '' })
const editProfile = ref<ManagedUser | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    users.value = (await api.users()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '用户列表加载失败'
  } finally {
    loading.value = false
  }
}

async function setRole(user: ManagedUser, role: 'member' | 'viewer') {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.updateUserRole(user.id, role)
    await load()
    toast('角色已更新')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '角色更新失败'
  } finally {
    busy.value = false
  }
}

async function disable(user: ManagedUser) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.disableUser(user.id)
    await load()
    toast('用户已停用')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '停用失败'
  } finally {
    busy.value = false
  }
}

async function remove(user: ManagedUser) {
  if (confirmDelete.value !== user.id) {
    confirmDelete.value = user.id
    return
  }
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.deleteUser(user.id)
    confirmDelete.value = ''
    await load()
    toast('用户已删除')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除失败'
  } finally {
    busy.value = false
  }
}

function beginReset(user: ManagedUser) {
  resetFor.value = user.id
  resetPassword.value = ''
}

async function submitReset(user: ManagedUser) {
  if (resetPassword.value.length < 12 || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.resetUserPassword(user.id, resetPassword.value)
    resetFor.value = ''
    resetPassword.value = ''
    toast('密码已重置')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '重置失败'
  } finally {
    busy.value = false
  }
}

function profileSaved(message: string) {
  editProfile.value = null
  toast(message)
  void load()
  void session.refreshMe()
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet wide-sheet" role="dialog" aria-modal="true" aria-labelledby="users-title">
      <header class="sheet-header">
        <h2 id="users-title" class="sheet-title">用户管理</h2>
        <button type="button" class="icon-btn" aria-label="关闭用户管理" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载用户…</p>
        <ul v-else class="editor-list">
          <li v-for="user in users" :key="user.id" class="editor-row user-row">
            <span class="editor-name">{{ user.username }}</span>
            <span class="editor-tag">{{ user.role === 'admin' ? '管理员' : user.role === 'viewer' ? '只读' : '成员' }}</span>
            <span v-if="user.status !== 'active'" class="editor-tag muted">{{ user.status }}</span>
            <span class="editor-actions">
              <button type="button" class="mini-btn" :disabled="busy" @click="editProfile = user">资料</button>
              <button type="button" class="mini-btn" :disabled="busy || user.role === 'admin'"
                @click="setRole(user, user.role === 'viewer' ? 'member' : 'viewer')">
                {{ user.role === 'viewer' ? '设为成员' : '设为只读' }}
              </button>
              <button type="button" class="mini-btn" :disabled="busy" @click="beginReset(user)">重置密码</button>
              <button type="button" class="mini-btn" :disabled="busy" @click="disable(user)">停用</button>
              <button type="button" class="mini-btn danger" :disabled="busy" @click="remove(user)">
                {{ confirmDelete === user.id ? '确认删除' : '删除' }}
              </button>
            </span>
            <form v-if="resetFor === user.id" class="reset-row" @submit.prevent="submitReset(user)">
              <label class="field-label">新密码（至少 12 位）
                <input v-model="resetPassword.value" type="password" minlength="12" maxlength="256" required autocomplete="new-password" />
              </label>
              <button type="submit" class="mini-btn" :disabled="busy || resetPassword.value.length < 12">确认</button>
            </form>
          </li>
          <li v-if="!users.length" class="editor-empty">暂无用户</li>
        </ul>
      </div>
    </section>

    <UserProfileDialog v-if="editProfile" :profile="editProfile" :user-id="editProfile.id"
      @close="editProfile = null" @saved="profileSaved" />
  </div>
</template>
