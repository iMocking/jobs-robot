import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from '@/App.vue'
import router from '@/router'
import '@/style.css'

// Pinia 必须先于 router 安装：路由守卫里会 useSessionStore()。
createApp(App).use(createPinia()).use(router).mount('#app')
