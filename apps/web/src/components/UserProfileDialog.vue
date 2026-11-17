<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, type UserProfile } from '../services/api'

const props = defineProps<{ profile: UserProfile; userId?: string; passwordOnly?: boolean }>()
const emit = defineEmits<{ close: []; saved: [message: string, profileChanged: boolean] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const busy = ref(false)
const error = ref('')
const draft = reactive({
  name: props.profile.member_name || props.profile.username,
  avatar: props.profile.avatar || '',
  birthday: props.profile.birthday || '',
  payday: props.profile.payday ?? '',
  email: props.profile.email || '',
})
const password = reactive({ current: '', next: '', confirm: '' })

onMounted(() => dialog.value?.showModal())

async function submit(action: () => Promise<unknown>, message: string, profileChanged: boolean) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await action()
    emit('saved', message, profileChanged)
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}

function saveProfile() {
  const name = draft.name.trim()
  const body = {
    ...(name === (props.profile.member_name || props.profile.username) ? {} : { name }),
    avatar: draft.avatar || null,
    birthday: draft.birthday || null,
    payday: Number(draft.payday) || null,
    email: draft.email || null,
  }
  void submit(() => props.userId ? api.updateUserProfile(props.userId, body) : api.updateMyProfile(body), '资料已保存', true)
}

function savePassword() {
  if (password.next !== password.confirm) {
    error.value = '两次输入的新密码不一致'
    return
  }
  void submit(
    () => props.userId ? api.resetUserPassword(props.userId, password.next) : api.changeMyPassword(password.current, password.next),
    props.userId ? '密码已重置，该账号的会话已退出' : '密码已修改',
    false,
  )
}
</script>

<template>
  <dialog ref="dialog" class="profile-dialog" aria-labelledby="profile-heading" @cancel.prevent="!busy && emit('close')">
    <div class="profile-heading">
      <h2 id="profile-heading">{{ passwordOnly ? (userId ? '重置密码' : '修改密码') : (userId ? '编辑用户资料' : '个人资料') }}</h2>
      <button class="ghost" type="button" :disabled="busy" @click="emit('close')">关闭</button>
    </div>
    <p>登录用户名 <strong>{{ profile.username }}</strong></p>
    <form v-if="!passwordOnly" class="profile-fields" @submit.prevent="saveProfile">
      <label>姓名 <input v-model="draft.name" required /></label>
      <label>头像（文字或表情） <input v-model="draft.avatar" /></label>
      <label>生日 <input v-model="draft.birthday" type="date" min="1900-01-01" /></label>
      <label>每月发薪日 <input v-model="draft.payday" type="number" min="1" max="31" /></label>
      <label>邮箱 <input v-model="draft.email" type="email" maxlength="254" /></label>
      <button class="primary" :disabled="busy">保存资料</button>
    </form>
    <form v-else class="profile-fields" @submit.prevent="savePassword">
      <p v-if="userId">重置后，该账号的所有会话将退出。</p>
      <label v-else>当前密码 <input v-model="password.current" type="password" autocomplete="current-password" required /></label>
      <label>新密码 <input v-model="password.next" type="password" autocomplete="new-password" minlength="8" maxlength="256" required /></label>
      <label>确认新密码 <input v-model="password.confirm" type="password" autocomplete="new-password" required /></label>
      <button class="primary" :disabled="busy">{{ userId ? '重置密码' : '修改密码' }}</button>
    </form>
    <p v-if="error" role="alert">{{ error }}</p>
  </dialog>
</template>

<style scoped>
.profile-dialog{width:min(420px,calc(100% - 32px));max-height:calc(100dvh - 32px);box-sizing:border-box;background:var(--card);color:var(--text);border:1px solid var(--border);border-radius:var(--radius);padding:22px;}
.profile-dialog::backdrop{background:rgba(62,48,40,.34);}
.profile-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;}
.profile-heading h2{margin:0;font-size:20px;}
.profile-fields,.profile-fields label{display:grid;gap:8px;}
.profile-fields{gap:12px;}
.profile-fields input{width:100%;box-sizing:border-box;}
</style>
