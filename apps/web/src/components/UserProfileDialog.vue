<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type UserProfile } from '../services/api'

const props = defineProps<{ profile: UserProfile; userId?: string; passwordOnly?: boolean }>()
const emit = defineEmits<{ close: []; saved: [message: string, profileChanged: boolean] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const busy = ref(false)
const processingAvatar = ref(false)
const error = ref('')
const draft = reactive({
  name: props.profile.member_name || props.profile.username,
  avatar: props.profile.avatar || '',
  birthday: props.profile.birthday || '',
  payday: props.profile.payday ?? '',
  email: props.profile.email || '',
})
const password = reactive({ current: '', next: '', confirm: '' })
const avatarImage = computed(() => draft.avatar.startsWith('data:image/jpeg;base64,'))

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
  if (processingAvatar.value) return
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

async function uploadAvatar(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  error.value = ''
  if (file.size > 10 * 1024 * 1024) {
    error.value = '头像图片须小于或等于 10 MB'
    return
  }
  processingAvatar.value = true
  try {
    const bitmap = await createImageBitmap(file)
    try {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 64
      const context = canvas.getContext('2d')
      if (!context) throw new Error()
      const side = Math.min(bitmap.width, bitmap.height)
      context.fillStyle = '#fffefa'
      context.fillRect(0, 0, 64, 64)
      context.drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, 64, 64)
      draft.avatar = canvas.toDataURL('image/jpeg', 0.85)
    } finally {
      bitmap.close()
    }
  } catch {
    error.value = '无法读取图片，请选择其他图片'
  } finally {
    processingAvatar.value = false
  }
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
      <div class="profile-avatar">
        <img v-if="avatarImage" :src="draft.avatar" width="64" height="64" alt="头像预览" />
        <button v-if="avatarImage" class="ghost" type="button" :disabled="busy || processingAvatar" @click="draft.avatar = ''">移除图片</button>
      </div>
      <label v-if="!avatarImage">头像（文字或表情） <input v-model="draft.avatar" :disabled="busy || processingAvatar" /></label>
      <label>上传头像图片 <input type="file" accept="image/*" :disabled="busy || processingAvatar" @change="uploadAvatar" /></label>
      <label>生日 <input v-model="draft.birthday" type="date" min="1900-01-01" /></label>
      <label>每月发薪日 <input v-model="draft.payday" type="number" min="1" max="31" /></label>
      <label>邮箱 <input v-model="draft.email" type="email" maxlength="254" /></label>
      <button class="primary" :disabled="busy || processingAvatar">保存资料</button>
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
.profile-dialog{width:min(420px,calc(100% - 32px));max-height:calc(100dvh - 32px);box-sizing:border-box;background:var(--card);color:var(--text);border:1px solid var(--border);border-radius:var(--radius);padding:22px;box-shadow:var(--shadow);}
.profile-dialog::backdrop{background:rgba(62,48,40,.34);}
.profile-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;}
.profile-heading h2{margin:0;font-size:20px;}
.profile-fields,.profile-fields label{display:grid;gap:8px;}
.profile-fields{gap:12px;}
.profile-fields label{color:var(--text);font-size:14px;font-weight:600;}
.profile-fields input{width:100%;min-width:0;font:inherit;font-size:16px;font-weight:400;color:var(--text);background:var(--card);border:1px solid var(--border);border-radius:11px;padding:10px 12px;}
.profile-fields input:focus-visible{outline:3px solid var(--primary);outline-offset:1px;}
.primary,.ghost{min-height:44px;font-size:14px;border-radius:12px;padding:8px 12px;}
.primary{border:0;background:var(--primary);color:#fff;font-weight:700;}
.ghost{border:1px solid var(--border);background:var(--soft);color:var(--primary-deep);}
button:disabled{opacity:.55;}
.profile-avatar{display:flex;align-items:center;gap:12px;}
.profile-avatar:empty{display:none;}
.profile-avatar img{border-radius:14px;}
</style>
