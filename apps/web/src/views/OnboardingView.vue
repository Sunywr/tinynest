<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useSession } from '../composables/useSession'
import { request } from '../services/api'

const route = useRoute()
const session = useSession()
const verified = computed(() => session.me.value?.email_verified)
const email = ref('')
const password = ref('')
const code = ref('')
const sent = ref<{ id: string; email: string; until: number }>()
const challenge = computed(() => sent.value?.email === email.value.trim() ? sent.value : undefined)
const household = ref('')
const invite = ref(typeof route.query.invite === 'string' ? route.query.invite : '')
const busy = ref(false)
const error = ref('')
const notice = ref('')

function post<T>(path: string, body: object) {
  return request<T>(path, { method: 'POST', body: JSON.stringify(body) })
}

async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await action()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '操作失败，请重试'
  } finally {
    busy.value = false
  }
}

function sendCode() {
  return run(async () => {
    if (challenge.value && Date.now() < challenge.value.until) throw new Error('请在60秒后重发')
    const address = email.value.trim()
    const result = await post<{ challenge_id: string }>('/auth/me/mail/challenge', {
      email: address, current_password: password.value, purpose: 'bind',
    })
    sent.value = { id: result.challenge_id, email: address, until: Date.now() + 60_000 }
    code.value = ''
    notice.value = '验证码已排队发送，10分钟内有效。'
  })
}

function verify() {
  return run(async () => {
    if (!challenge.value) throw new Error('请先为当前邮箱发送验证码')
    await post('/auth/me/mail/confirm', {
      email: challenge.value.email, current_password: password.value,
      proof: { challenge_id: challenge.value.id, code: code.value },
    })
    password.value = ''
    await session.refreshMe()
  })
}

function create() {
  return run(async () => {
    await post('/household/current', { name: household.value.trim() })
    await session.refreshMe()
  })
}

function join() {
  return run(async () => {
    await post('/auth/join', { code: invite.value.trim() })
    await session.refreshMe()
  })
}

function refresh() {
  return run(async () => {
    await session.refreshMe()
    if (verified.value && !session.hasHousehold.value && !session.me.value?.join_pending) notice.value = '当前没有待审批的加入申请。'
  })
}
</script>

<template>
  <main class="onboarding">
    <section class="login-dialog" aria-labelledby="onboarding-title">
      <img src="/icons/tinynest-original.png" alt="" class="dialog-logo" />
      <h2 id="onboarding-title">{{ verified ? '创建或加入家庭' : '验证邮箱' }}</h2>
      <p>{{ verified ? '邮箱已验证，可使用密码找回。创建或加入家庭后即可记账、查看资产与报表。' : '你已登录。验证邮箱后可找回密码，也可以创建或申请加入家庭。' }}</p>
      <form v-if="!verified" @submit.prevent="verify">
        <label>邮箱<input v-model="email" :disabled="busy" type="email" autocomplete="email" required /></label>
        <label>当前密码<input v-model="password" :disabled="busy" type="password" autocomplete="current-password" required /></label>
        <button type="button" class="btn-quiet" :disabled="busy || !email || !password" @click="sendCode">发送验证码</button>
        <p class="email-flow-hint">同一邮箱每天最多3封邮件，上海时间零点重置。</p>
        <label>邮箱验证码<input v-model="code" :disabled="busy" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" maxlength="6" required /></label>
        <button class="btn-primary" :disabled="busy || !challenge">验证邮箱</button>
      </form>
      <template v-else>
        <form @submit.prevent="create">
          <label>家庭名称<input v-model="household" :disabled="busy" maxlength="100" required /></label>
          <button class="btn-primary" :disabled="busy">创建家庭</button>
        </form>
        <form @submit.prevent="join">
          <label>邀请码<input v-model="invite" :disabled="busy" maxlength="160" autocomplete="off" required /></label>
          <button class="btn-primary" :disabled="busy">申请加入家庭</button>
        </form>
      </template>
      <p v-if="session.me.value?.join_pending" role="status">加入申请等待管理员审批。批准后即可使用家庭账本。</p>
      <p v-if="error" class="err" role="alert">{{ error }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <div class="onboarding-actions">
        <button type="button" class="btn-quiet" :disabled="busy" @click="refresh">刷新进度</button>
        <button type="button" class="btn-quiet" :disabled="busy" @click="run(session.logout)">退出登录</button>
      </div>
    </section>
  </main>
</template>

<style scoped>
.onboarding { min-height: 100dvh; display: grid; place-items: center; padding: 16px; }
.onboarding .login-dialog { max-height: none; }
.onboarding form + form { margin-top: 16px; }
.onboarding-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
</style>
