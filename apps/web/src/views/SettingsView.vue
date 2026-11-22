<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import MailSettings from '../components/MailSettings.vue'
import UserProfileDialog from '../components/UserProfileDialog.vue'
import { useSession } from '../composables/useSession'
import { useToast } from '../composables/useToast'

const router = useRouter()
const session = useSession()
const { toast } = useToast()
const profileOpen = ref(false)

const me = computed(() => session.me.value)
const profile = computed(() => ({
  username: me.value?.username || '',
  member_name: me.value?.member_name || session.currentMember.value?.name,
  avatar: me.value?.avatar,
  email: me.value?.email,
  birthday: (me.value?.birthday as string | undefined) || '',
  payday: (me.value?.payday as number | undefined) ?? undefined,
}))

function back() {
  void router.push('/me')
}

async function profileSaved(message: string) {
  profileOpen.value = false
  toast(message)
  await session.refreshMe()
}
</script>

<template>
  <section class="page settings-page">
    <header class="page-head settings-head">
      <button type="button" class="icon-btn" aria-label="返回我的" @click="back"><ArrowLeft :size="20" /></button>
      <div>
        <h1 class="page-title">邮箱与密码</h1>
        <p class="page-sub">账号安全与邮件通知</p>
      </div>
    </header>

    <section class="card settings-card" aria-label="个人资料">
      <div class="settings-row">
        <div>
          <strong>个人资料</strong>
          <p>昵称、头像、生日与发薪日。</p>
        </div>
        <button type="button" class="btn-quiet" @click="profileOpen = true">编辑资料</button>
      </div>
      <p class="form-hint">登录用户名：{{ me?.username }} · 角色：{{ me?.role === 'admin' ? '管理员' : me?.role === 'viewer' ? '只读成员' : '成员' }}</p>
    </section>

    <MailSettings @identity-updated="session.refreshMe()" />

    <button type="button" class="btn-quiet back-btn" @click="back">返回我的</button>

    <UserProfileDialog v-if="profileOpen" :profile="profile" @close="profileOpen = false" @saved="profileSaved" />
  </section>
</template>
