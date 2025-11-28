<template>
  <div id="designing">
    <div class="content">
      <h1 class="title">{{ msg }}</h1>
      <p class="subtitle">敬请等待</p>
    </div>
  </div>
</template>

<script setup lang="ts">
const msg = "功能开发中...";
</script>

<style scoped>
#designing {
  /* 高度随视口自适应 */
  min-height: 80vh;
  background: #ffffff;

  /* 让内容在页面中上下左右居中 */
  display: flex;
  align-items: center; /* 垂直居中 */
  justify-content: center; /* 水平居中 */

  padding: 0 24px;
  box-sizing: border-box;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI",
    sans-serif;
}

.content {
  text-align: center;
}

.title {
  margin: 0 0 12px;
  font-size: 32px;
  line-height: 1.3;
  color: #111827;
  font-weight: 600;
}

.subtitle {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
  color: #6b7280;
}

/* 小屏适配 */
@media (max-width: 600px) {
  #homePage {
    padding: 0 16px;
  }

  .title {
    font-size: 24px;
  }
}
</style>
