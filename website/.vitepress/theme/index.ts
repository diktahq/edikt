import DefaultTheme from 'vitepress/theme'
import Layout from './Layout.vue'
import Terminal from './Terminal.vue'
import T from './T.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component('Terminal', Terminal)
    app.component('T', T)
  }
}
