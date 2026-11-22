<script setup lang="ts">
const props = withDefaults(defineProps<{
  disabled?: boolean
  busy?: boolean
  doneLabel?: string
}>(), { disabled: false, busy: false, doneLabel: '完成' })

const emit = defineEmits<{
  press: [key: string]
  back: []
  done: []
}>()

const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '0'] as const

function onBackspace() {
  if (!props.disabled && !props.busy) emit('back')
}
</script>

<template>
  <div class="keypad" role="group" aria-label="金额数字键盘" :aria-busy="busy">
    <div class="keypad-grid">
      <button v-for="key in keys" :key="key" type="button" class="keypad-key" :disabled="disabled || busy"
        :aria-label="key === '.' ? '小数点' : key" @click="emit('press', key)">
        {{ key }}
      </button>
      <button type="button" class="keypad-key backspace" :disabled="disabled || busy" aria-label="删除末位"
        @click="onBackspace">⌫</button>
    </div>
    <button type="button" class="keypad-done" :disabled="disabled || busy" :aria-busy="busy"
      :aria-label="busy ? '正在保存' : doneLabel" @click="emit('done')">
      <span v-if="busy" class="spinner" aria-hidden="true"></span>
      <span>{{ busy ? '保存中…' : doneLabel }}</span>
    </button>
  </div>
</template>

<style scoped>
.keypad { display: grid; gap: 8px; padding: 10px 12px calc(10px + env(safe-area-inset-bottom)); background: var(--card); }
.keypad-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.keypad-key {
  min-height: var(--keypad-key);
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--card);
  color: var(--text);
  font-size: 22px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.keypad-key:disabled { opacity: .45; }
.keypad-key:enabled:active { background: var(--soft); }
.backspace { color: var(--primary-deep); font-size: 20px; }
.keypad-done {
  min-height: var(--keypad-key);
  border: 1px solid var(--primary);
  border-radius: 16px;
  background: var(--primary);
  color: #fff;
  font-size: 16px;
  font-weight: 750;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}
.keypad-done:disabled { opacity: .6; }
</style>
