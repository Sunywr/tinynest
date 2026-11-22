<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { api, type Member } from '../services/api'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const emit = defineEmits<{ close: []; saved: [] }>()
const session = useSession()
const { toast } = useToast()

const members = ref<Member[]>([])
const newName = ref('')
const busy = ref(false)
const loading = ref(true)
const error = ref('')
const confirmDelete = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    members.value = (await api.members()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '成员加载失败'
  } finally {
    loading.value = false
  }
}

async function addMember() {
  const name = newName.value.trim()
  if (!name || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.createMember(name)
    newName.value = ''
    await load()
    await session.refreshHousehold()
    toast('成员已添加')
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '添加失败，请重试'
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
    await load()
    await session.refreshHousehold()
    toast('成员已删除')
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除失败，请重试'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet" role="dialog" aria-modal="true" aria-labelledby="members-title">
      <header class="sheet-header">
        <h2 id="members-title" class="sheet-title">家庭成员</h2>
        <button type="button" class="icon-btn" aria-label="关闭成员管理" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p class="form-hint">成员用于标记「这笔账是谁记的」。当前登录成员：{{ session.currentMember.value?.name || '未绑定' }}</p>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载成员…</p>
        <ul v-else class="editor-list">
          <li v-for="member in members" :key="member.id" class="editor-row">
            <span class="editor-emoji" aria-hidden="true">♡</span>
            <span class="editor-name">{{ member.name }}</span>
            <span v-if="member.id === session.me.value?.member_id" class="editor-tag">当前</span>
            <span v-if="member.status !== 'active'" class="editor-tag muted">已停用</span>
            <span class="editor-actions">
              <button type="button" class="mini-btn danger" :disabled="busy" @click="removeMember(member)">
                {{ confirmDelete === member.id ? '确认删除' : '删除' }}
              </button>
            </span>
          </li>
          <li v-if="!members.length" class="editor-empty">还没有成员</li>
        </ul>
        <form class="inline-form" @submit.prevent="addMember">
          <label class="field-label">新成员名称
            <input v-model="newName" maxlength="100" required placeholder="例如：妈妈" :disabled="busy" />
          </label>
          <div class="form-actions">
            <button type="submit" class="btn-primary" :disabled="busy || !newName.trim()">
              {{ busy ? '添加中…' : '添加成员' }}
            </button>
          </div>
        </form>
      </div>
    </section>
  </div>
</template>
