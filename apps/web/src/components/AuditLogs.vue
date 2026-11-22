<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { api, type AuditLog } from '../services/api'

const emit = defineEmits<{ close: [] }>()
const logs = ref<AuditLog[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    logs.value = (await api.auditLogs(50)).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '审计日志加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="sheet-backdrop form-backdrop" @click.self="emit('close')">
    <section class="sheet form-sheet wide-sheet" role="dialog" aria-modal="true" aria-labelledby="audit-title">
      <header class="sheet-header">
        <h2 id="audit-title" class="sheet-title">审计日志</h2>
        <button type="button" class="icon-btn" aria-label="关闭审计日志" @click="emit('close')"><X :size="20" /></button>
      </header>
      <div class="form-body">
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <p v-if="loading" class="inline-notice" role="status">正在加载审计日志…</p>
        <ul v-else class="editor-list audit-list">
          <li v-for="log in logs" :key="log.id" class="editor-row audit-row">
            <span class="audit-action">{{ log.action }}</span>
            <span class="audit-meta">{{ log.actor }} · {{ log.entity }}{{ log.entity_id ? ` #${log.entity_id}` : '' }}</span>
            <span class="audit-time">{{ log.created_at }}</span>
          </li>
          <li v-if="!logs.length" class="editor-empty">暂无审计记录</li>
        </ul>
        <p class="form-hint">审计日志为只读，按当前家庭隔离。</p>
      </div>
    </section>
  </div>
</template>
