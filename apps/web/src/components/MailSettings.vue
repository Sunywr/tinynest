<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { request } from '../services/api'
const props=defineProps<{ role: string }>()
const emit=defineEmits<{identityUpdated:[]}>()
const identity=ref<any>(null), config=ref<any>(null), error=ref(''), message=ref(''), busy=ref(false)
const credentials=reactive({password:'',current_password:'',test_recipient:''})
const binding=reactive({email:'',current_password:'',code:'',old_code:'',new_password:'',confirm_password:''})
const proofs=reactive<Record<string,string>>({})
const announcement=reactive({title:'',body:''}), preview=ref<number|null>(null)
const cooldown=reactive<Record<string,number>>({})
const statuses=ref<any[]>([])
const post=(p:string,b:any)=>request(p,{method:'POST',body:JSON.stringify(b)})
async function run(fn:()=>Promise<void>){if(busy.value)return;busy.value=true;error.value='';message.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function load(){identity.value=await request('/auth/me/mail');if(identity.value.site_admin)config.value=await request('/site/mail');if(props.role==='admin')statuses.value=(await request('/household/mail/announcements')).items}
onMounted(()=>run(load))
async function challenge(purpose:string){await run(async()=>{if(Date.now()<(cooldown[purpose]||0))throw Error('请在60秒后重发');const r=await post('/auth/me/mail/challenge',{email:binding.email,purpose,current_password:binding.current_password});proofs[purpose]=r.challenge_id;cooldown[purpose]=Date.now()+60000;message.value='验证码已排队发送，10分钟内有效'})}
function proof(purpose:string,code:string){return{challenge_id:proofs[purpose]||'',code}}
async function bind(){await run(async()=>{await post('/auth/me/mail/confirm',{email:binding.email,current_password:binding.current_password,proof:proof('bind',binding.code),old_proof:proof('old_email',binding.old_code)});binding.code='';binding.old_code='';binding.current_password='';await load();emit('identityUpdated');message.value='邮箱已验证'})}
async function password(){await run(async()=>{if(binding.new_password!==binding.confirm_password)throw Error('两次密码不一致');await post('/auth/me/password',{current_password:binding.current_password,new_password:binding.new_password,proof:proof('password',binding.code)});binding.new_password='';binding.current_password='';binding.code='';window.dispatchEvent(new CustomEvent('tinynest:session-expired'))})}
async function saveConfig(){await run(async()=>{try{await post('/site/mail',{...credentials,config:config.value.config,version:config.value.version});await load();message.value='测试邮件已被SMTP接受，配置已生效'}finally{credentials.password='';credentials.current_password=''}})}
async function sendAnnouncement(isPreview:boolean){await run(async()=>{const r=await post('/household/mail/announcements',{...announcement,preview:isPreview,expected_count:preview.value});if(isPreview){preview.value=r.recipient_count}else{preview.value=null;announcement.title='';announcement.body='';await load();message.value='公告已加入发送队列'}})}
</script>
<template>
<section class="card mail-settings" aria-label="邮箱与安全">
<div class="mail-heading"><div><span class="mail-eyebrow">ACCOUNT & MAIL</span><h2>邮箱与安全</h2></div><button type="button" :disabled="busy" @click="run(load)">刷新邮件状态</button></div>
<p v-if="error" class="err" role="alert">{{error}}</p><p v-if="message" class="mail-notice" role="status">{{message}}</p>
<template v-if="identity">
<p>{{identity.verified ? `已验证：${identity.email}` : '尚未验证邮箱。记账不受影响，请验证后再修改或找回密码。'}}</p>
<p v-if="!identity.mail_available">系统邮件尚未启用，请联系站点管理员。</p>
<form @submit.prevent="bind"><h3>绑定或修改邮箱</h3>
<label>当前登录密码<input v-model="binding.current_password" type="password" autocomplete="current-password" required maxlength="256"></label>
<label>新邮箱<input v-model="binding.email" type="email" required maxlength="254"></label>
<button type="button" :disabled="busy||!identity.mail_available" @click="challenge('bind')">发送新邮箱验证码</button>
<label>新邮箱验证码<input v-model="binding.code" inputmode="numeric" maxlength="6" required autocomplete="one-time-code"></label>
<template v-if="identity.verified"><button type="button" :disabled="busy||!identity.mail_available" @click="challenge('old_email')">发送原邮箱验证码</button><label>原邮箱验证码<input v-model="binding.old_code" inputmode="numeric" maxlength="6" required></label></template>
<button class="btn-primary" :disabled="busy||!identity.mail_available">确认邮箱</button></form>
<form v-if="identity.verified" @submit.prevent="password"><h3>修改密码</h3><p>修改后所有设备需要重新登录。</p>
<label>当前登录密码<input v-model="binding.current_password" type="password" required autocomplete="current-password"></label>
<button type="button" :disabled="busy" @click="challenge('password')">发送密码修改验证码</button>
<label>验证码<input v-model="binding.code" inputmode="numeric" maxlength="6" required></label>
<label>新密码<input v-model="binding.new_password" type="password" minlength="12" maxlength="256" required autocomplete="new-password"></label>
<label>确认新密码<input v-model="binding.confirm_password" type="password" minlength="12" maxlength="256" required autocomplete="new-password"></label>
<button class="btn-primary" :disabled="busy">验证并修改密码</button></form>
<label class="mail-toggle"><input v-model="identity.announcements" type="checkbox" :disabled="busy" @change="run(async()=>{await post('/auth/me/mail/preferences',{announcements:identity.announcements});message='偏好已保存'})">接收家庭公告邮件</label>
</template>
<form class="system-mail" v-if="config" @submit.prevent="saveConfig"><h2>系统邮件 · 仅站点管理员</h2>
<p>配置版本 {{config.version}} · 今日已用 {{config.used_today}} 封 · 公告 {{config.announcements_today}} 封</p>
<p v-if="config.last_test?.at">最近测试：{{config.last_test.result==='mail_test_failed'?'失败，旧配置继续生效':'SMTP已接受'}} · {{config.last_test.at}}</p>
<p v-if="!config.key_ready" class="err">服务器尚未配置邮件加密密钥，暂不能保存凭据。</p>
<label class="mail-toggle"><input v-model="config.config.enabled" type="checkbox">启用邮件服务</label>
<label>飞书 SMTP<input v-model="config.config.host" readonly></label>
<label>加密连接<select v-model.number="config.config.port"><option :value="465">465 SSL/TLS</option><option :value="587">587 STARTTLS</option></select></label>
<label>公共邮箱账号<input v-model="config.config.username" type="email" required @input="config.config.from=config.config.username"></label>
<label>发件名称<input v-model="config.config.name" maxlength="100" required></label>
<label>SMTP 密码<input v-model="credentials.password" type="password" autocomplete="new-password" maxlength="1024" :placeholder="config.password_configured?'已配置，留空保留':'请输入专用密码'"></label>
<label>每日总额度<input v-model.number="config.config.daily_limit" type="number" min="1" required></label>
<label>每日公告额度<input v-model.number="config.config.announcement_limit" type="number" min="0" :max="config.config.daily_limit" required></label>
<label>测试收件邮箱<input v-model="credentials.test_recipient" type="email" required></label>
<label>当前登录密码<input v-model="credentials.current_password" type="password" autocomplete="current-password" required></label>
<button class="btn-primary" :disabled="busy||!config.key_ready">测试并保存配置</button><p>测试失败保留旧配置；密码保存后不再显示。</p>
</form>
<form class="announcement-mail" v-if="role==='admin'" @submit.prevent="sendAnnouncement(true)"><h2>家庭公告邮件</h2>
<p>只发送给本家庭已验证且接收公告的成员。</p>
<label>标题<input v-model="announcement.title" required maxlength="200" @input="preview=null"></label>
<label>正文<textarea v-model="announcement.body" required maxlength="20000" rows="5" @input="preview=null"></textarea></label>
<button :disabled="busy">预览收件人数与额度</button>
<div v-if="preview!==null"><h3>{{announcement.title}}</h3><p class="mail-preview">{{announcement.body}}</p><p>将发送给 {{preview}} 位成员。</p><button type="button" class="btn-primary" :disabled="busy||preview===0" @click="sendAnnouncement(false)">确认发送公告</button></div>
<p v-for="s in statuses" :key="s.status">{{({'queued':'排队中','sending':'发送中','smtp_accepted':'SMTP已接受','failed':'失败','expired':'已过期','cancelled':'已取消'} as Record<string,string>)[s.status]||s.status}}：{{s.count}}</p>
</form></section>
</template>
<style scoped>
.mail-settings{margin:24px 0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px;align-items:start;color:var(--text)}
.mail-heading{grid-column:1/-1;display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:2px}
.mail-heading h2{font-size:20px;line-height:1.4;margin:4px 0 0}.mail-eyebrow{color:var(--muted);font-size:10px;letter-spacing:.14em;font-weight:700}
.mail-settings>p,.mail-settings>.mail-toggle{grid-column:1/-1;margin:0;line-height:1.7;font-size:13px}
.mail-settings form{display:grid;gap:14px;min-width:0;margin:0;padding:24px;background:var(--card);border:1px solid var(--border);border-radius:var(--radius);box-shadow:var(--shadow)}
.mail-settings form h2,.mail-settings form h3{margin:0 0 2px;font-size:16px;line-height:1.5;letter-spacing:.01em}
.mail-settings form p{margin:0;color:var(--muted);font-size:12px;line-height:1.7;overflow-wrap:anywhere}
.mail-settings label{display:grid;gap:7px;font-size:13px;font-weight:600;line-height:1.5;min-width:0}
.mail-settings input,.mail-settings select,.mail-settings textarea{box-sizing:border-box;min-width:0;width:100%;min-height:44px;padding:10px 12px;border:1px solid var(--border);border-radius:11px;background:var(--card);color:var(--text);font:inherit;font-size:16px;transition:border-color .15s,box-shadow .15s}
.mail-settings input[readonly]{background:var(--soft);color:var(--muted)}
.mail-settings textarea{resize:vertical;line-height:1.7;min-height:150px}
.mail-settings input:focus-visible,.mail-settings select:focus-visible,.mail-settings textarea:focus-visible{outline:2px solid var(--primary);outline-offset:2px}
.mail-settings button{justify-self:start;min-height:44px;max-width:100%;padding:10px 16px;border:1px solid var(--border);border-radius:12px;background:var(--soft);color:var(--primary-deep);font:inherit;font-size:13px;font-weight:600;line-height:1.5;cursor:pointer;transition:background .15s,box-shadow .15s}
.mail-settings button:hover:enabled{background:var(--primary-pale);box-shadow:0 3px 10px #765b650a}
.mail-settings button.btn-primary{border-color:var(--primary);background:var(--primary);color:#fff}
.mail-settings button.btn-primary:hover:enabled{background:var(--primary-deep);border-color:var(--primary-deep)}
.mail-settings button:disabled{opacity:.5;cursor:not-allowed}
.mail-settings .mail-toggle{display:flex;align-items:center;gap:10px;font-weight:500;padding:12px 14px;border-radius:12px;background:var(--soft)}
.mail-settings input[type=checkbox]{width:18px;height:18px;min-height:18px;flex:none;margin:0;accent-color:var(--primary)}
.mail-settings .err,.mail-settings .mail-notice{padding:12px 16px;border:1px solid var(--border);border-radius:12px;background:var(--soft);overflow-wrap:anywhere}
.mail-settings .err{color:#a04e43}.mail-settings .mail-notice{color:#567661}
.mail-preview{white-space:pre-wrap;overflow-wrap:anywhere;padding:14px;background:var(--soft);border-radius:12px;margin:10px 0!important}
@media(max-width:760px){.mail-settings{grid-template-columns:minmax(0,1fr);gap:14px;margin:18px 0}.mail-settings form{padding:18px}.mail-heading{align-items:flex-start;flex-wrap:wrap}.mail-heading h2{font-size:18px}.mail-settings button{width:100%;justify-self:stretch}.mail-heading button{width:auto;min-height:40px;padding:8px 12px;font-size:12px}}
@media(prefers-reduced-motion:reduce){.mail-settings button,.mail-settings input,.mail-settings select,.mail-settings textarea{transition:none}}
</style>
