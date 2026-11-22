<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { X } from 'lucide-vue-next'
import CategoryEmojiPicker from './CategoryEmojiPicker.vue'
import { api, type Category } from '../services/api'
import { useToast } from '../composables/useToast'

const props = withDefaults(defineProps<{ initialKind?: 'expense' | 'income' }>(), { initialKind: 'expense' })
const emit = defineEmits<{ close: []; saved: [] }>()
const { toast } = useToast()

const kind = ref<'expense' | 'income'>(props.initialKind)
const items = ref<Category[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const formOpen = ref(false)
const form = reactive({ id: '', name: '', emoji: '🍙' })
const confirmRemove = ref('')

const visible = computed(() =>
  items.value.filter(c => c.kind === kind.value).slice().sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0)),
)
const isEditing = computed(() => !!form.id)
const title = computed(() => (isEditing.value ? '编辑分类' : '新建分类'))

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await api.categories()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '分类加载失败'
  } finally {
    loading.value = false
  }
}

function switchKind(next: 'expense' | 'income') {
  kind.value = next
  formOpen.value = false
  confirmRemove.value = ''
}

function startCreate() {
  form.id = ''
  form.name = ''
  form.emoji = kind.value === 'income' ? '💰' : '🍙'
  formOpen.value = true
}

function startEdit(category: Category) {
  form.id = category.id
  form.name = category.name
  form.emoji = category.emoji || ''
  formOpen.value = true
}

function cancelForm() {
  formOpen.value = false
  form.id = ''
  form.name = ''
}

async function save() {
  const name = form.name.trim()
  if (!name || busy.value) return
  busy.value = true
  error.value = ''
  try {
    if (form.id) await api.updateCategory(form.id, { name, emoji: form.emoji })
    else await api.createCategory({ kind: kind.value, name, emoji: form.emoji, sort: visible.value.length })
    await load()
    formOpen.value = false
    form.id = ''
    toast('分类已保存')
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败，请重试'
  } finally {
    busy.value = false
  }
}

async function toggleActive(category: Category) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.updateCategory(category.id, { active: Number(category.active) === 0 })
    await load()
    toast(Number(category.active) === 0 ? '分类已启用' : '分类已停用')
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败，请重试'
  } finally {
    busy.value = false
  }
}

async function remove(category: Category) {
  if (confirmRemove.value !== category.id) {
    confirmRemove.value = category.id
    return
  }
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.deleteCategory(category.id)
    confirmRemove.value = ''
    await load()
    toast('分类已停用，历史流水保留')
    emit('saved')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '停用失败，请重试'
  } finally {
    busy.value = false
  }
}

async function move(category: Category, delta: number) {
  const list = visible.value
  const index = list.findIndex(c => c.id === category.id)
  const target = list[index + delta]
  if (!target || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api.updateCategory(category.id, { sort: target.sort })
    await api.updateCategory(target.id, { sort: category.sort })
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '排序失败，请重试'
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet category-editor" role="dialog" aria-modal="true" aria-labelledby="cat-editor-title">
      <header class="sheet-header">
        <h2 id="cat-editor-title" class="sheet-title">分类管理</h2>
        <button type="button" class="icon-btn" aria-label="关闭分类管理" @click="emit('close')"><X :size="20" /></button>
      </header>

      <div class="form-body">
        <div class="segmented" role="tablist" aria-label="分类类型">
          <button type="button" role="tab" class="segmented-btn" :class="{ active: kind === 'expense' }"
            :aria-selected="kind === 'expense'" @click="switchKind('expense')">支出分类</button>
          <button type="button" role="tab" class="segmented-btn" :class="{ active: kind === 'income' }"
            :aria-selected="kind === 'income'" @click="switchKind('income')">收入分类</button>
        </div>

        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载分类…</p>

        <ul v-if="!loading" class="editor-list">
          <li v-for="category in visible" :key="category.id" class="editor-row" :class="{ inactive: Number(category.active) === 0 }">
            <span class="editor-emoji" aria-hidden="true">{{ category.emoji || '·' }}</span>
            <span class="editor-name">{{ category.name }}</span>
            <span v-if="Number(category.active) === 0" class="editor-tag">已停用</span>
            <span class="editor-actions">
              <button type="button" class="mini-btn" :aria-label="`上移 ${category.name}`" :disabled="busy" @click="move(category, -1)">↑</button>
              <button type="button" class="mini-btn" :aria-label="`下移 ${category.name}`" :disabled="busy" @click="move(category, 1)">↓</button>
              <button type="button" class="mini-btn" :aria-label="`编辑 ${category.name}`" :disabled="busy" @click="startEdit(category)">改</button>
              <button type="button" class="mini-btn" :disabled="busy" @click="toggleActive(category)">
                {{ Number(category.active) === 0 ? '启用' : '停用' }}
              </button>
              <button type="button" class="mini-btn danger" :disabled="busy" @click="remove(category)">
                {{ confirmRemove === category.id ? '确认停用' : '删除' }}
              </button>
            </span>
          </li>
          <li v-if="!visible.length" class="editor-empty">这一组还没有分类</li>
        </ul>

        <button v-if="!formOpen" type="button" class="btn-primary" @click="startCreate">＋ 新建分类</button>

        <form v-else class="inline-form" @submit.prevent="save">
          <h3 class="inline-form-title">{{ title }}</h3>
          <label class="field-label">名称
            <input v-model="form.name" maxlength="40" required placeholder="例如：餐饮" />
          </label>
          <CategoryEmojiPicker v-model="form.emoji" />
          <div class="form-actions">
            <button type="button" class="btn-quiet" :disabled="busy" @click="cancelForm">取消</button>
            <button type="submit" class="btn-primary" :disabled="busy || !form.name.trim()">
              {{ busy ? '保存中…' : '保存分类' }}
            </button>
          </div>
        </form>
      </div>
    </section>
  </div>
</template>
