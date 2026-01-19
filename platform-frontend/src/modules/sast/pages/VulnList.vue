<template>
  <div class="vuln-list-container">
    <a-card title="SAST 任务列表" :bordered="false" style="width: 100%">
      <template #extra>
        <a-space>
          <a-popconfirm
            v-if="hasSelected"
            title="是否删除？"
            ok-text="是"
            cancel-text="否"
            @confirm="handleBulkDelete"
          >
            <a-button type="primary" danger :loading="loading">
              Delete
            </a-button>
          </a-popconfirm>
          <a-button type="primary" @click="fetchTasks">Refresh</a-button>
        </a-space>
      </template>

      <div style="margin-bottom: 16px" v-if="hasSelected">
        <span style="margin-left: 8px">
          Selected {{ selectedRowKeys.length }} items
        </span>
      </div>

      <a-table
        :columns="columns"
        :data-source="tasks"
        :loading="loading"
        row-key="id"
        :scroll="{ x: 1000 }"
        :row-selection="{
          selectedRowKeys: selectedRowKeys,
          onChange: onSelectChange,
        }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="getStatusColor(record.status)">
              {{ record.status.toUpperCase() }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="viewDetails(record.id)"
              >查看结果</a-button
            >
          </template>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script lang="ts">
import { defineComponent, ref, onMounted, computed } from "vue";
import { useRouter } from "vue-router";
import request from "@/utils/request";
import { message } from "ant-design-vue";

interface ScanTask {
  id: string;
  target: string;
  type: "Git" | "Upload";
  status: "pending" | "running" | "completed" | "failed";
  startTime: string;
  vulnerabilities: number;
}

export default defineComponent({
  name: "VulnList",
  setup() {
    const router = useRouter();
    const loading = ref(false);
    const tasks = ref<ScanTask[]>([]);
    const selectedRowKeys = ref<string[]>([]);

    const hasSelected = computed(() => selectedRowKeys.value.length > 0);

    const onSelectChange = (keys: string[]) => {
      selectedRowKeys.value = keys;
    };

    const columns = [
      // {
      //   title: "ID",
      //   dataIndex: "id",
      //   key: "id",
      // },
      {
        title: "Target",
        dataIndex: "target",
        key: "target",
        ellipsis: true,
      },
      {
        title: "Type",
        dataIndex: "type",
        key: "type",
        width: 100,
      },
      {
        title: "Status",
        dataIndex: "status",
        key: "status",
        width: 125,
      },
      {
        title: "Start Time",
        dataIndex: "startTime",
        key: "startTime",
        width: 200,
      },
      {
        title: "Vulnerabilities",
        dataIndex: "vulnerabilities",
        key: "vulnerabilities",
        width: 150,
      },
      {
        title: "Action",
        key: "action",
        width: 180,
        fixed: "right",
      },
    ];

    const getStatusColor = (status: string) => {
      switch (status) {
        case "completed":
          return "success";
        case "running":
          return "processing";
        case "failed":
          return "error";
        default:
          return "default";
      }
    };

    const fetchTasks = async () => {
      loading.value = true;
      try {
        const res = await request.get("/api/sast/vuln/list");
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        tasks.value = res.data.map((item: any) => {
          let vulns = 0;
          if (item.result && item.result.startsWith("Found ")) {
            const match = item.result.match(/Found (\d+) vulnerabilities/);
            if (match) {
              vulns = parseInt(match[1]);
            }
          }
          return {
            id: item.id,
            target: item.target,
            type: item.type,
            status: item.status,
            startTime: item.startTime, // formatted by backend or raw string
            vulnerabilities: vulns,
          };
        });
      } catch (error) {
        message.error("Failed to fetch tasks");
        console.error(error);
      } finally {
        loading.value = false;
      }
    };

    const viewDetails = (id: string) => {
      router.push(`/sast/vuln/${id}`);
    };

    const handleBulkDelete = async () => {
      loading.value = true;
      try {
        // Delete tasks sequentially or in parallel
        await Promise.all(
          selectedRowKeys.value.map((id) =>
            request.post(`/api/sast/vuln/delete/${id}`)
          )
        );
        message.success("Selected tasks deleted successfully");
        selectedRowKeys.value = [];
        fetchTasks();
      } catch (error) {
        message.error("Failed to delete selected tasks");
        console.error(error);
      } finally {
        loading.value = false;
      }
    };

    onMounted(() => {
      fetchTasks();
    });

    return {
      tasks,
      loading,
      columns,
      fetchTasks,
      getStatusColor,
      viewDetails,
      selectedRowKeys,
      onSelectChange,
      hasSelected,
      handleBulkDelete,
    };
  },
});
</script>

<style scoped>
.vuln-list-container {
  padding: 24px;
}
</style>
