<script setup>
// Registration with a simulated email step.
//
// No mail is sent in this build, so the server returns the code and it is shown
// here. The rest of the flow is the real one: the account does not exist until
// the code is presented, so replacing this with a mailer is a server-side
// change only.
import { onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookOpen, Loader2, MailCheck } from 'lucide-vue-next'
import { api } from '../api'
import { useAuth } from '../store/auth'
import BrandMark from '../components/BrandMark.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuth()

const step = ref('form') // form | verify
const email = ref('')
const name = ref('')
const password = ref('')
const code = ref('')
const shownCode = ref('')
const busy = ref(false)
const error = ref('')
const seconds = ref(0)
let ticker = null

function startCountdown() {
  seconds.value = 60
  clearInterval(ticker)
  ticker = setInterval(() => {
    seconds.value -= 1
    if (seconds.value <= 0) clearInterval(ticker)
  }, 1000)
}
onBeforeUnmount(() => clearInterval(ticker))

async function send() {
  error.value = ''
  busy.value = true
  try {
    const r = await api.register(email.value.trim(), password.value, name.value.trim())
    shownCode.value = r.code || ''
    code.value = r.code || ''
    step.value = 'verify'
    startCountdown()
  } catch (e) {
    error.value = e.message || '注册失败'
  } finally {
    busy.value = false
  }
}

async function confirm() {
  error.value = ''
  busy.value = true
  try {
    const r = await api.verify(email.value.trim(), code.value.trim())
    auth.accept(r)
    router.push(String(route.query.next || '/'))
  } catch (e) {
    error.value = e.message || '验证失败'
  } finally {
    busy.value = false
  }
}

async function resend() {
  if (seconds.value > 0) return
  try {
    const r = await api.resend(email.value.trim())
    shownCode.value = r.code || ''
    code.value = r.code || ''
    startCountdown()
  } catch (e) {
    error.value = e.message
  }
}
</script>

<template>
  <div class="authwrap">
    <form class="card authbox" @submit.prevent="step === 'form' ? send() : confirm()">
      <router-link to="/" class="brand" style="color: inherit; justify-content: center">
        <span class="mark"><BrandMark :size="17" /></span>
        <span class="nm">巴利三藏阅读器</span>
      </router-link>

      <template v-if="step === 'form'">
        <h1 class="authtitle">注册</h1>
        <p class="authlede">用邮箱注册，即可保存你的选词、批注与翻译。</p>

        <label class="lbl" for="email">邮箱</label>
        <input id="email" v-model="email" class="field" type="email" autocomplete="email" required />

        <label class="lbl" for="name">昵称（可选）</label>
        <input id="name" v-model="name" class="field" type="text" autocomplete="nickname" />

        <label class="lbl" for="password">密码</label>
        <input
          id="password"
          v-model="password"
          class="field"
          type="password"
          autocomplete="new-password"
          minlength="6"
          required
        />
        <p class="hint">至少 6 位。</p>

        <p v-if="error" class="err">{{ error }}</p>

        <button class="btn btn-primary" style="width: 100%; margin-top: 16px" :disabled="busy">
          <Loader2 v-if="busy" :size="15" class="spin" />
          {{ busy ? '发送中…' : '发送验证码' }}
        </button>
        <p class="alt">已有账号？<router-link to="/login">直接登录</router-link></p>
      </template>

      <template v-else>
        <div style="display: flex; align-items: center; gap: 9px; margin-top: 22px">
          <MailCheck :size="20" style="color: var(--accent)" />
          <h1 class="authtitle" style="margin: 0">验证邮箱</h1>
        </div>
        <p class="authlede">
          验证码已发送至 <b>{{ email }}</b>。<br />
          演示环境不发送真实邮件，验证码直接显示在下方。
        </p>

        <div v-if="shownCode" class="codebox">
          <span class="eyebrow">演示验证码</span>
          <b class="num">{{ shownCode }}</b>
        </div>

        <label class="lbl" for="code">验证码</label>
        <input
          id="code"
          v-model="code"
          class="field num"
          type="text"
          inputmode="numeric"
          maxlength="6"
          style="letter-spacing: 6px; font-size: 18px; text-align: center"
          required
        />

        <p v-if="error" class="err">{{ error }}</p>

        <button class="btn btn-primary" style="width: 100%; margin-top: 16px" :disabled="busy">
          <Loader2 v-if="busy" :size="15" class="spin" />
          {{ busy ? '验证中…' : '完成注册' }}
        </button>
        <p class="alt">
          <button type="button" class="linkbtn" :disabled="seconds > 0" @click="resend">
            {{ seconds > 0 ? `重新发送（${seconds}s）` : '重新发送验证码' }}
          </button>
          ·
          <button type="button" class="linkbtn" @click="step = 'form'">返回修改</button>
        </p>
      </template>
    </form>
  </div>
</template>

<style scoped>
.authwrap {
  display: grid;
  place-items: center;
  min-height: 100vh;
  padding: 24px 16px;
}
.authbox {
  width: 100%;
  max-width: 400px;
  padding: 30px 28px 24px;
}
.authtitle {
  margin-top: 22px;
  font-size: 24px;
  font-weight: 500;
}
.authlede {
  margin-top: 8px;
  margin-bottom: 20px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--muted);
}
.lbl {
  display: block;
  margin: 12px 0 5px;
  font-size: 11.5px;
  letter-spacing: 0.5px;
  color: var(--meta);
}
.hint {
  margin-top: 5px;
  font-size: 11.5px;
  color: var(--meta);
}
.codebox {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 13px;
  margin-bottom: 6px;
  border-radius: var(--radius-md);
  background: var(--tag-soft);
}
.codebox b {
  font-size: 19px;
  letter-spacing: 4px;
  color: var(--accent);
}
.err {
  margin-top: 12px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: #f6ecea;
  color: var(--danger);
  font-size: 12.5px;
}
.alt {
  margin-top: 14px;
  text-align: center;
  font-size: 12.5px;
  color: var(--meta);
}
.linkbtn {
  color: var(--accent);
  font: inherit;
}
.linkbtn:disabled {
  color: var(--meta);
  cursor: default;
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
