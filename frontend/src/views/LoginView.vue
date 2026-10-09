<script setup>
// Sign in. One form, one error slot, no decoration.
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookOpen, Loader2 } from 'lucide-vue-next'
import { api } from '../api'
import { useAuth } from '../store/auth'
import BrandMark from '../components/BrandMark.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuth()

const email = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  busy.value = true
  try {
    const r = await api.login(email.value.trim(), password.value)
    auth.accept(r)
    router.push(String(route.query.next || '/'))
  } catch (e) {
    error.value = e.message || '登录失败'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="authwrap">
    <form class="card authbox" @submit.prevent="submit">
      <router-link to="/" class="brand" style="color: inherit; justify-content: center">
        <span class="mark"><BrandMark :size="17" /></span>
        <span class="nm">巴利三藏阅读器</span>
      </router-link>

      <h1 class="authtitle">登录</h1>
      <p class="authlede">登录后可以保存选词、批注与翻译。</p>

      <label class="lbl" for="email">邮箱</label>
      <input id="email" v-model="email" class="field" type="email" autocomplete="email" required />

      <label class="lbl" for="password">密码</label>
      <input
        id="password"
        v-model="password"
        class="field"
        type="password"
        autocomplete="current-password"
        required
      />

      <p v-if="error" class="err">{{ error }}</p>

      <button class="btn btn-primary" style="width: 100%; margin-top: 16px" :disabled="busy">
        <Loader2 v-if="busy" :size="15" class="spin" />
        {{ busy ? '登录中…' : '登录' }}
      </button>

      <p class="alt">还没有账号？<router-link to="/register">注册一个</router-link></p>
      <p class="alt"><router-link to="/">先不登录，直接阅读</router-link></p>
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
  max-width: 380px;
  padding: 30px 28px 24px;
}
.authtitle {
  margin-top: 22px;
  font-size: 24px;
  font-weight: 500;
}
.authlede {
  margin-top: 6px;
  margin-bottom: 20px;
  font-size: 13px;
  color: var(--muted);
}
.lbl {
  display: block;
  margin: 12px 0 5px;
  font-size: 11.5px;
  letter-spacing: 0.5px;
  color: var(--meta);
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
  margin-top: 12px;
  text-align: center;
  font-size: 12.5px;
  color: var(--meta);
}
.spin {
  animation: spin 700ms linear infinite;
}
</style>
