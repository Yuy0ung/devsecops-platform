<template>
  <div class="dast-task-page">
    <div class="page-header">
      <h2 class="page-title">DAST 扫描任务</h2>
      <a-space>
        <a-button type="primary" @click="openCreateModal"> 新增任务 </a-button>
        <a-button @click="fetchTasks" :loading="loading"> 刷新 </a-button>
        <a-button
          danger
          :disabled="selectedRowKeys.length === 0"
          @click="handleBatchDelete"
        >
          删除任务
        </a-button>
      </a-space>
    </div>

    <a-table
      :columns="columns"
      :data-source="tasks"
      :loading="loading"
      row-key="taskId"
      :row-selection="rowSelection"
      :pagination="false"
      :custom-row="customRow"
    >
      <template #bodyCell="{ column, record }">
        <!-- 状态列 -->
        <template v-if="column.dataIndex === 'status'">
          <a-tag :color="statusColor(record.status)">
            {{ renderStatusText(record.status) }}
          </a-tag>
        </template>

        <!-- 操作列 -->
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a-button
              type="link"
              size="small"
              @click.stop="handleStartTask(record)"
              :disabled="record.status === 'running'"
            >
              开始
            </a-button>
            <a-button
              type="link"
              size="small"
              danger
              @click.stop="handleStopTask(record)"
              :disabled="record.status !== 'running'"
            >
              强制停止
            </a-button>
          </a-space>
        </template>

        <!-- 任务 ID（截断显示，hover 显示完整） -->
        <template v-else-if="column.dataIndex === 'taskId'">
          <a-tooltip :title="record.taskId">
            <span class="task-id-text">
              {{ shortTaskId(record.taskId) }}
            </span>
          </a-tooltip>
        </template>

        <!-- 默认展示 -->
        <template v-else>
          {{ record[column.dataIndex] }}
        </template>
      </template>
    </a-table>

    <!-- 新增任务弹窗 -->
    <a-modal
      v-model:open="createModalVisible"
      title="新增任务"
      :confirm-loading="createLoading"
      @ok="handleCreateTask"
      @cancel="handleCancelCreate"
      ok-text="创建"
      cancel-text="取消"
      destroy-on-close
    >
      <a-form layout="vertical">
        <a-form-item label="任务名称">
          <a-input v-model:value="form.taskName" placeholder="请输入任务名称" />
        </a-form-item>

        <a-form-item
          label="扫描目标 IP:Port"
          extra="每行一个，例如：1.2.3.4:80"
        >
          <a-textarea
            v-model:value="form.targetsText"
            :rows="6"
            placeholder="例如：
1.2.3.4:80
5.6.7.8:443"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import request from "@/utils/request";
import { message, Modal } from "ant-design-vue";

interface TaskItem {
  taskId: string;
  taskName: string; // 新增：任务名称
  status: string;
  created_at: string;
  updated_at: string;
}

const router = useRouter();

const loading = ref(false);
const tasks = ref<TaskItem[]>([]);
const selectedRowKeys = ref<string[]>([]);

const createModalVisible = ref(false);
const createLoading = ref(false);
const form = ref({
  taskName: "",
  targetsText: "",
});

const columns = [
  { title: "任务名称", dataIndex: "taskName" }, // 第一列显示名称
  { title: "任务 ID", dataIndex: "taskId" },
  { title: "状态", dataIndex: "status" },
  { title: "创建时间", dataIndex: "created_at" },
  { title: "更新时间", dataIndex: "updated_at" },
  { title: "操作", key: "actions" },
];

// 是否可以删除：仅 stopped/stoped/finished
const canDeleteTask = (task: TaskItem) => {
  const s = (task.status || "").toLowerCase();
  return (
    s === "finished" || s === "stopped" || s === "pending" || s === "error"
  );
};

const rowSelection = computed(() => ({
  selectedRowKeys: selectedRowKeys.value,
  onChange: (keys: (string | number)[]) => {
    selectedRowKeys.value = keys as string[];
  },
  getCheckboxProps: (record: TaskItem) => ({
    disabled: !canDeleteTask(record),
  }),
}));

// 行点击跳转任务详情
const handleRowClick = (record: TaskItem) => {
  router.push(`/dast/task/targets?taskId=${record.taskId}`);
};

// 让整行可点击（勾选框、操作按钮内部事件通过 .stop 阻止冒泡）
const customRow = (record: TaskItem) => {
  return {
    onClick: () => handleRowClick(record),
    style: { cursor: "pointer" },
  };
};

// 状态显示文本
const renderStatusText = (status: string) => {
  const s = (status || "").toLowerCase();
  if (s === "running") return "运行中";
  if (s === "finished") return "已完成";
  if (s === "pending") return "待开始";
  if (s === "stopped") return "已停止";
  if (s === "error") return "错误";
  return status;
};

// 状态颜色
const statusColor = (status: string) => {
  const s = (status || "").toLowerCase();
  if (s === "running") return "processing";
  if (s === "finished") return "success";
  if (s === "stopped") return "default";
  if (s === "pending") return "default";
  if (s === "error") return "error";
  return "default";
};

// taskId 截断显示
const shortTaskId = (id: string) => {
  if (!id) return "";
  if (id.length <= 8) return id;
  return id.slice(0, 8) + "...";
};

// 获取任务列表
const fetchTasks = async () => {
  try {
    loading.value = true;
    const res = await request.get("/api/task/list");
    tasks.value = res.data?.tasks || [];
  } catch (e) {
    message.error("获取任务列表失败");
  } finally {
    loading.value = false;
  }
};

onMounted(fetchTasks);

// 打开 / 关闭新增弹窗
const openCreateModal = () => {
  createModalVisible.value = true;
};

const handleCancelCreate = () => {
  createModalVisible.value = false;
};

// 创建任务
const handleCreateTask = async () => {
  const taskName = form.value.taskName.trim();
  const targets = form.value.targetsText
    .split(/\r?\n/)
    .map((t) => t.trim())
    .filter(Boolean);

  if (!taskName) {
    message.warning("请填写任务名称");
    return;
  }

  if (!targets.length) {
    message.warning("请至少填写一个扫描目标");
    return;
  }

  createLoading.value = true;
  try {
    const res = await request.post("/api/task/create", {
      taskName, // 按后端要求携带任务名称
      targets,
    });

    message.success(res.data?.message || "任务创建成功");
    createModalVisible.value = false;
    form.value.taskName = "";
    form.value.targetsText = "";

    // 新增后被动刷新
    await fetchTasks();
  } catch (e) {
    message.error("任务创建失败");
  } finally {
    createLoading.value = false;
  }
};

// 开始任务
const handleStartTask = async (task: TaskItem) => {
  if (task.status === "running") return;
  try {
    const res = await request.get("/api/task/start", {
      params: { taskId: task.taskId },
    });
    message.success(res.data?.message || "任务已开始扫描");
    await fetchTasks();
  } catch (e) {
    message.error("启动任务失败");
  }
};

// 强制停止任务
const handleStopTask = async (task: TaskItem) => {
  if (task.status !== "running") return;
  try {
    const res = await request.get("/api/task/stop", {
      params: { taskId: task.taskId },
    });
    message.success(res.data?.message || "任务已停止");
    await fetchTasks();
  } catch (e) {
    message.error("停止任务失败");
  }
};

// 删除多个任务
const deleteTasks = async (taskIds: string[]) => {
  try {
    await Promise.all(
      taskIds.map((id) =>
        request.get("/api/task/delete", {
          params: { taskId: id },
        })
      )
    );
    message.success("删除成功");
    selectedRowKeys.value = [];
    // 删除后被动刷新
    await fetchTasks();
  } catch (e) {
    message.error("删除任务失败");
  }
};

// 批量删除按钮
const handleBatchDelete = () => {
  const deletableIds = tasks.value
    .filter((t) => selectedRowKeys.value.includes(t.taskId) && canDeleteTask(t))
    .map((t) => t.taskId);

  if (!deletableIds.length) {
    message.warning("只有已停止或已完成的任务可以删除");
    return;
  }

  Modal.confirm({
    title: "确认删除选中的任务吗？",
    content: "删除后不可恢复。",
    okText: "删除",
    okType: "danger",
    cancelText: "取消",
    async onOk() {
      await deleteTasks(deletableIds);
    },
  });
};
</script>

<style scoped>
.dast-task-page {
  min-height: 100vh;
  padding: 16px 24px;
  background-color: #ffffff; /* 白色为主色 */
  box-sizing: border-box;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #111827;
}

.task-id-text {
  color: #1677ff;
  text-decoration: underline;
  word-break: break-all;
}
</style>
