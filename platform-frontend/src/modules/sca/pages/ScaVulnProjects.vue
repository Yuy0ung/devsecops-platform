<template>
  <div class="sca-vuln-projects">
    <a-card title="Vulnerability Dashboard" :bordered="false">
      <template #extra>
        <a-button @click="fetchTasks">Refresh</a-button>
      </template>

      <a-table
        :columns="columns"
        :data-source="vulnerableTasks"
        :loading="loading"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'target'">
            <span style="font-weight: bold">{{ record.target }}</span>
          </template>

          <template v-else-if="column.key === 'projectLanguage'">
            <a-tag
              v-for="lang in record.projectLanguage
                ? record.projectLanguage.split(', ')
                : []"
              :key="lang"
              color="blue"
            >
              {{ lang }}
            </a-tag>
          </template>

          <template v-else-if="column.key === 'vulnCount'">
            <span style="color: #ff4d4f; font-weight: bold">{{
              record.vulnCount
            }}</span>
          </template>

          <template v-else-if="column.key === 'maxSeverity'">
            <a-tag :color="getSeverityColor(record.maxSeverity)">
              {{ record.maxSeverity || "None" }}
            </a-tag>
          </template>

          <template v-else-if="column.key === 'action'">
            <a-button
              type="primary"
              size="small"
              @click="viewDetails(record.id)"
            >
              Analysis
            </a-button>
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

interface ScaTask {
  id: string;
  target: string;
  type: string;
  status: string;
  startTime: string;
  result: string;
  maxSeverity?: string;
  vulnCount?: number;
  projectLanguage?: string;
}

export default defineComponent({
  name: "ScaVulnProjects",
  setup() {
    const router = useRouter();
    const tasks = ref<ScaTask[]>([]);
    const loading = ref(false);

    const columns = [
      {
        title: "Project / File",
        dataIndex: "target",
        key: "target",
      },
      {
        title: "Language",
        dataIndex: "projectLanguage",
        key: "projectLanguage",
      },
      {
        title: "Vulnerabilities",
        dataIndex: "vulnCount",
        key: "vulnCount",
        sorter: (a: ScaTask, b: ScaTask) =>
          (a.vulnCount || 0) - (b.vulnCount || 0),
      },
      {
        title: "Highest Severity",
        dataIndex: "maxSeverity",
        key: "maxSeverity",
      },
      {
        title: "Started At",
        dataIndex: "startTime",
        key: "startTime",
      },
      {
        title: "Action",
        key: "action",
      },
    ];

    const vulnerableTasks = computed(() => {
      // Only show tasks that have vulnerabilities
      return tasks.value.filter((t) => (t.vulnCount || 0) > 0);
    });

    const fetchTasks = async () => {
      loading.value = true;
      try {
        const res = await request.get("/api/sca/list");
        tasks.value = res.data || [];
      } catch (error) {
        message.error("Failed to fetch tasks");
      } finally {
        loading.value = false;
      }
    };

    const viewDetails = (id: string) => {
      router.push(`/sca/vuln/${id}`);
    };

    const getSeverityColor = (severity: string) => {
      switch (severity?.toLowerCase()) {
        case "critical":
          return "purple";
        case "high":
          return "red";
        case "medium":
          return "orange";
        case "low":
          return "blue";
        default:
          return "green";
      }
    };

    onMounted(() => {
      fetchTasks();
    });

    return {
      loading,
      columns,
      vulnerableTasks,
      fetchTasks,
      viewDetails,
      getSeverityColor,
    };
  },
});
</script>
