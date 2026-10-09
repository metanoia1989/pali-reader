<script setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuth } from './store/auth'
import { setUnauthorizedHandler } from './api'

const auth = useAuth()
const router = useRouter()

onMounted(async () => {
  setUnauthorizedHandler(() => {
    auth.user = null
    if (router.currentRoute.value.meta?.requiresAuth) router.push('/login')
  })
  await auth.restore()
})
</script>

<template>
  <router-view v-slot="{ Component }">
    <component :is="Component" />
  </router-view>
</template>
