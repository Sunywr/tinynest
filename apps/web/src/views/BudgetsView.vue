<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import BudgetBar from '../components/BudgetBar.vue'
import BudgetEditor from '../components/BudgetEditor.vue'
import SmartSelect from '../components/SmartSelect.vue'
import { useSession } from '../composables/useSession'
import { api, type Budget, type ProjectBudget } from '../services/api'
import { formatMinor, parseMinor, today } from '../services/format'

const { canWrite } = useSession()
const month = ref(today().slice(0, 7))
const monthly = ref<Budget | null>(null)
const projects = ref<ProjectBudget[]>([])
const selection = ref('')
const options = computed(() => [{ id: '', name: '全部项目' }, ...projects.value])
const visible = computed(() => projects.value.filter(project => !selection.value || selection.value === project.id))
const editMonthly = ref(false)
const editor = ref<{ id: string; name: string; amount: string } | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const formError = ref('')

async function loadMonth() {
  const selected = month.value
  monthly.value = null
  if (!/^\d{4}-\d{2}$/.test(selected)) return
  try {
    const budget = await api.budget(selected)
    if (month.value === selected) monthly.value = budget
  } catch (e) {
    if (month.value === selected) error.value = e instanceof Error ? e.message : '月度预算加载失败'
  }
}

async function loadProjects() {
  loading.value = true
  error.value = ''
  try {
    projects.value = (await api.projectBudgets()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '项目预算加载失败'
  } finally {
    loading.value = false
  }
}

function edit(project?: ProjectBudget) {
  formError.value = ''
  editor.value = project
    ? { id: project.id, name: project.name, amount: (project.amount_minor / 100).toFixed(2) }
    : { id: '', name: '', amount: '' }
}

async function save() {
  const form = editor.value
  if (!form || saving.value) return
  const amount = parseMinor(form.amount)
  if (!form.name.trim() || !amount || amount <= 0) {
    formError.value = '请输入项目名称和有效的预算金额'
    return
  }
  saving.value = true
  formError.value = ''
  try {
    const result = await api.saveProjectBudget(form.id, form.name.trim(), amount)
    if (selection.value) selection.value = result.id
    editor.value = null
    await loadProjects()
  } catch (e) {
    formError.value = e instanceof Error ? e.message : '预算保存失败'
  } finally {
    saving.value = false
  }
}

onMounted(() => { void loadProjects(); void loadMonth() })
watch(month, () => { editMonthly.value = false; void loadMonth() })
</script>

<template>
  <div class="budgets-page">
    <header class="budget-head">
      <div>
        <RouterLink to="/me">返回我的</RouterLink>
        <h1>预算</h1>
      </div>
    </header>
    <section class="budget-section" aria-labelledby="monthly-heading">
      <div class="section-head">
        <h2 id="monthly-heading">月度预算</h2>
        <input v-model="month" type="month" aria-label="预算月份" />
      </div>
      <p class="budget-hint">当月所有支出自动计入，包括已关联项目的支出。</p>
      <BudgetBar :budget="monthly" :spent-minor="monthly?.spent_minor ?? 0"
        :editable="canWrite && !!monthly" @edit="editMonthly = true" />
    </section>

    <section class="budget-section" aria-labelledby="projects-heading">
      <div class="section-head">
        <h2 id="projects-heading">项目预算</h2>
        <button v-if="canWrite" class="btn-primary" type="button" @click="edit()">新增项目</button>
      </div>
      <p class="budget-hint">仅统计已关联的支出，可跨月累计。</p>
      <SmartSelect v-if="projects.length" v-model="selection" :options="options" placeholder="搜索项目预算" />
      <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
      <p v-if="loading" role="status">正在读取预算…</p>
      <p v-else-if="!projects.length && !error" class="budget-hint">还没有项目预算。</p>

      <form v-if="editor" class="project-editor" @submit.prevent="save">
        <h3>{{ editor.id ? '编辑项目预算' : '新增项目预算' }}</h3>
        <label class="field-label">项目名称
          <input v-model="editor.name" maxlength="80" placeholder="例如 新房装修" :disabled="saving" required />
        </label>
        <label class="field-label">预算金额（元）
          <input v-model="editor.amount" inputmode="decimal" :disabled="saving" required />
        </label>
        <p v-if="formError" class="inline-error" role="alert">{{ formError }}</p>
        <div class="form-actions">
          <button type="button" class="btn-quiet" :disabled="saving" @click="editor = null">取消</button>
          <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? '保存中…' : '保存项目预算' }}</button>
        </div>
      </form>
      <article v-for="project in visible" :key="project.id" class="project-budget">
        <div class="section-head">
          <h3>{{ project.name }}</h3>
          <button v-if="canWrite" type="button" class="btn-quiet" @click="edit(project)">编辑</button>
        </div>
        <progress :value="Math.min(project.spent_minor, project.amount_minor)" :max="project.amount_minor"
          :aria-label="project.name + '预算使用进度'" />
        <dl class="budget-figures">
          <div><dt>预算</dt><dd>{{ formatMinor(project.amount_minor) }}</dd></div>
          <div><dt>已用</dt><dd>{{ formatMinor(project.spent_minor) }}</dd></div>
          <div><dt>{{ project.spent_minor > project.amount_minor ? '超出' : '剩余' }}</dt>
            <dd>{{ formatMinor(Math.abs(project.amount_minor - project.spent_minor)) }}</dd></div>
        </dl>
      </article>
    </section>
    <BudgetEditor v-if="editMonthly" :month="month" @close="editMonthly = false" @saved="loadMonth" />
  </div>
</template>

<style scoped>
.budgets-page { display: grid; gap: 18px; max-width: 760px; margin-inline: auto; }
.budget-head h1, .section-head h2, h3 { margin: 0; overflow-wrap: anywhere; }
.budget-head a { color: var(--primary-deep); }
.budget-section { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: 20px; background: var(--card); }
.section-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; flex-wrap: wrap; }
.budget-hint, dt { margin: 0; color: var(--muted); font-size: .85rem; }
.project-editor, .project-budget { display: grid; gap: 10px; padding: 14px; border-radius: 14px; background: var(--soft); }
progress { width: 100%; accent-color: var(--primary-deep); }
.budget-figures { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 8px; margin: 0; }
dd { margin: 2px 0 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; }
</style>
