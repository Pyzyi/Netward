import '@fontsource-variable/schibsted-grotesk/index.css';
import '@fontsource-variable/jetbrains-mono/index.css';
import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
