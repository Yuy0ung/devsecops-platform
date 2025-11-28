// src/utils/request.ts
import axios from "axios";
import router from "@/router";
import { message } from "ant-design-vue";

const request = axios.create({
  //   baseURL: "http://160.30.231.213:5003/", // 后端地址
  baseURL: "http://127.0.0.1:5003/",
  timeout: 10000,
});

// 请求拦截器：每次发请求之前自动执行
request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem("token");
    if (token) {
      config.headers = config.headers || {};
      // 核心：自动加上 Authorization 头
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器：可以顺便统一处理 401 未登录
request.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // token 无效 / 过期：清理本地并跳到登录
      localStorage.removeItem("token");
      localStorage.removeItem("username");
      message.error("登录已过期，请重新登录");
      router.push("/login");
    }
    return Promise.reject(error);
  }
);

export default request;
