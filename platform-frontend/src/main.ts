import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import Antd from "ant-design-vue";
import "ant-design-vue/dist/reset.css";

// Fix ResizeObserver loop limit exceeded error
// eslint-disable-next-line @typescript-eslint/ban-types
const debounce = (fn: Function, delay: number) => {
  let timeoutId: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (...args: any[]) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn(...args), delay);
  };
};

const _ResizeObserver = window.ResizeObserver;
window.ResizeObserver = class ResizeObserver extends _ResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    super(debounce(callback, 16));
  }
};

// Ignore runtime errors for ResizeObserver
const ignoreErrors = [
  "ResizeObserver loop completed with undelivered notifications",
  "ResizeObserver loop limit exceeded",
];

const originalOnError = window.onerror;
window.onerror = (msg, source, line, col, error) => {
  if (ignoreErrors.some((e) => msg.toString().includes(e))) {
    return true; // suppress error
  }
  if (originalOnError) {
    return originalOnError(msg, source, line, col, error);
  }
};

window.addEventListener("error", (event) => {
  if (ignoreErrors.some((e) => event.message.includes(e))) {
    event.stopImmediatePropagation();
    event.preventDefault();
  }
});

createApp(App).use(Antd).use(router).mount("#app");
