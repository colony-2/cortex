import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import 'wx-svelte-filemanager/dist/filemanager.css'

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
