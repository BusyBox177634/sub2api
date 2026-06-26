<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="grid gap-6 xl:grid-cols-[360px_minmax(0,1fr)]">
        <section class="card p-5">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">AI 生成配置</h2>
          <div class="mt-4 space-y-4">
            <label class="block">
              <span class="input-label">Base URL</span>
              <input v-model.trim="settingsForm.base_url" class="input" placeholder="https://api.openai.com" />
            </label>
            <label class="block">
              <span class="input-label">API Key</span>
              <input v-model.trim="settingsForm.api_key" type="password" class="input" :placeholder="settings?.api_key_configured ? '已配置，留空不修改' : 'sk-...'" />
            </label>
            <label class="block">
              <span class="input-label">模型</span>
              <input v-model.trim="settingsForm.model" class="input" placeholder="gpt-5.5" />
            </label>
            <div class="grid grid-cols-2 gap-3">
              <label class="block">
                <span class="input-label">上下文</span>
                <input v-model.number="settingsForm.context_tokens" type="number" class="input" min="16000" />
              </label>
              <label class="block">
                <span class="input-label">输出保留</span>
                <input v-model.number="settingsForm.output_reserved_tokens" type="number" class="input" min="4096" />
              </label>
            </div>
            <label class="block">
              <span class="input-label">并发</span>
              <input v-model.number="settingsForm.concurrency" type="number" class="input" min="1" max="32" />
            </label>
            <div class="flex gap-2">
              <button class="btn btn-primary" :disabled="savingSettings" @click="saveSettings">保存配置</button>
              <button class="btn btn-secondary" :disabled="savingSettings" @click="clearAPIKey">清除 Key</button>
            </div>
          </div>
        </section>

        <section class="card p-5">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">手动触发</h2>
          <div class="mt-4 grid gap-4 lg:grid-cols-2">
            <div class="space-y-3">
              <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">生产生成</h3>
              <div class="grid grid-cols-2 gap-3">
                <select v-model="productionForm.period_type" class="input">
                  <option value="daily">日报</option>
                  <option value="weekly">周报</option>
                  <option value="monthly">月报</option>
                </select>
                <input v-model="productionForm.period_date" type="date" class="input" />
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">生产生成会为所有普通用户创建任务；管理员账号不参与。</p>
              <button class="btn btn-primary" :disabled="triggering" @click="triggerProduction">加入生产队列</button>
            </div>
            <div class="space-y-3">
              <h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">测试生成</h3>
              <div class="flex gap-2">
                <div class="relative flex-1">
                  <input
                    v-model="testUserQuery"
                    type="text"
                    autocomplete="off"
                    class="input w-full"
                    placeholder="输入普通用户邮箱"
                    @input="handleTestUserInput"
                    @focus="openTestUserDropdown"
                    @blur="scheduleCloseTestUserDropdown"
                  />
                  <div
                    v-if="showTestUserDropdown && (testUserLoading || testUserOptions.length > 0 || testUserQuery.trim())"
                    class="absolute left-0 right-0 top-full z-20 mt-1 max-h-56 overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                  >
                    <div v-if="testUserLoading" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">搜索中...</div>
                    <button
                      v-for="user in testUserOptions"
                      :key="user.id"
                      type="button"
                      class="flex w-full flex-col px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
                      @mousedown.prevent="selectTestUser(user)"
                    >
                      <span class="text-sm font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                      <span v-if="user.username" class="text-xs text-gray-500 dark:text-gray-400">{{ user.username }}</span>
                    </button>
                    <div v-if="!testUserLoading && testUserQuery.trim() && !testUserOptions.length" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">未找到用户</div>
                  </div>
                </div>
                <button v-if="testUserQuery || testForm.user_id" type="button" class="btn btn-secondary shrink-0" @mousedown.prevent @click="clearTestUser">清除</button>
              </div>
              <select v-model.number="testForm.group_id" class="input">
                <option :value="undefined">全部分组</option>
                <option v-for="group in groupOptions" :key="group.id" :value="group.id">
                  {{ group.name }}
                </option>
              </select>
              <div class="grid grid-cols-2 gap-3">
                <input v-model="testForm.range_start" type="date" class="input" />
                <input v-model="testForm.range_end" type="date" class="input" />
              </div>
              <button class="btn btn-secondary" :disabled="triggering" @click="createTestJob">加入测试队列</button>
            </div>
          </div>
        </section>
      </div>

      <section class="card overflow-hidden">
        <div class="flex flex-wrap items-center gap-3 border-b border-gray-200 px-5 py-3 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">生产与测试队列</h2>
          <select v-model="jobFilters.scope" class="input ml-auto w-36" @change="handleJobFilterChange">
            <option value="">全部队列</option>
            <option value="production">生产</option>
            <option value="test">测试</option>
          </select>
          <select v-model="jobFilters.status" class="input w-36" @change="handleJobFilterChange">
            <option value="">全部状态</option>
            <option value="queued">排队中</option>
            <option value="running">生成中</option>
            <option value="failed">失败</option>
            <option value="canceled">已取消</option>
            <option value="succeeded">已完成</option>
            <option value="paused">已暂停</option>
            <option value="partial">部分完成</option>
          </select>
          <button class="btn btn-secondary" :disabled="loadingJobs" @click="refreshJobs">
            {{ loadingJobs ? '刷新中' : '刷新' }}
          </button>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">ID</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">队列</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">周期/范围</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">状态</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">用户进度</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">Token 进度</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <template v-for="batch in batches" :key="batch.id">
                <tr>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                    <button type="button" class="font-medium text-primary-600 hover:text-primary-700" @click="toggleBatch(batch)">
                      {{ isBatchExpanded(batch.id) ? '收起' : '展开' }} #{{ batch.id }}
                    </button>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                    <div>{{ batch.batch_scope === 'production' ? '生产' : '测试' }}</div>
                    <div class="text-xs text-gray-500">{{ batch.trigger_kind === 'auto' ? '自动触发' : '手动触发' }}</div>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                    <div class="max-w-[260px] truncate font-medium text-gray-800 dark:text-gray-100">{{ batch.title }}</div>
                    <div class="text-xs text-gray-500">{{ batchPeriod(batch) }}</div>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ statusLabel(batch.status) }}</td>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                    {{ batch.succeeded_count }}/{{ batch.job_count }}
                    <span v-if="batch.running_count || batch.queued_count || batch.retrying_count" class="text-xs text-gray-500">
                      · 运行 {{ batch.running_count }} / 排队 {{ batch.queued_count }} / 重试 {{ batch.retrying_count }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ tokenProgress(batch) }}</td>
                  <td class="px-4 py-3 text-right">
                    <div class="flex justify-end gap-2">
                      <button
                        v-if="batch.status === 'queued' || batch.status === 'running' || batch.status === 'partial'"
                        class="btn btn-secondary btn-sm"
                        :disabled="isBatchActionPending(batch.id)"
                        @click="pauseBatch(batch.id)"
                      >
                        暂停
                      </button>
                      <button
                        v-if="batch.status === 'paused'"
                        class="btn btn-secondary btn-sm"
                        :disabled="isBatchActionPending(batch.id)"
                        @click="resumeBatch(batch.id)"
                      >
                        恢复
                      </button>
                      <button
                        v-if="batch.status === 'queued' || batch.status === 'running' || batch.status === 'paused' || batch.status === 'partial'"
                        class="btn btn-secondary btn-sm"
                        :disabled="isBatchActionPending(batch.id)"
                        @click="cancelBatch(batch.id)"
                      >
                        取消
                      </button>
                      <button
                        v-if="batch.status === 'failed' || batch.status === 'canceled' || batch.status === 'partial'"
                        class="btn btn-secondary btn-sm"
                        :disabled="isBatchActionPending(batch.id)"
                        @click="resetBatch(batch.id)"
                      >
                        恢复
                      </button>
                      <button
                        v-if="batch.status === 'failed' || batch.status === 'canceled' || batch.status === 'partial' || batch.status === 'paused' || batch.status === 'succeeded'"
                        class="btn btn-secondary btn-sm"
                        :disabled="isBatchActionPending(batch.id)"
                        @click="rerunBatch(batch.id)"
                      >
                        重新生成
                      </button>
                      <button class="btn btn-danger btn-sm" :disabled="isBatchActionPending(batch.id)" @click="deleteBatch(batch.id)">
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
                <tr v-if="isBatchExpanded(batch.id)" class="bg-gray-50/70 dark:bg-dark-800/60">
                  <td colspan="7" class="px-4 py-3">
                    <div v-if="batchJobsLoading[batch.id]" class="py-4 text-center text-sm text-gray-500">加载子任务中...</div>
                    <div v-else class="overflow-x-auto rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
                      <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
                        <thead class="bg-gray-50 dark:bg-dark-800">
                          <tr>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">用户</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">状态</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">阶段</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">进度</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">Token</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">邮件</th>
                            <th class="px-3 py-2 text-left text-xs font-medium text-gray-500">重试</th>
                            <th class="px-3 py-2 text-right text-xs font-medium text-gray-500">操作</th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                          <tr v-for="job in batchJobs[batch.id] || []" :key="job.id">
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ job.user_email || job.user_id || '-' }}</td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ statusLabel(job.status) }}</td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ jobStageLabel(job) }}</td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">
                              {{ job.progress_current }}/{{ job.progress_total || '-' }}
                              <span v-if="job.chunk_total" class="text-xs text-gray-500">· 分片 {{ job.chunk_current }}/{{ job.chunk_total }}</span>
                            </td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ tokenProgress(job) }}</td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">
                              <div>{{ emailStatusLabel(job.email_status) }}</div>
                              <div v-if="job.email_sent_at" class="text-xs text-gray-500">{{ formatDateTime(job.email_sent_at) }}</div>
                              <div v-else-if="job.email_error_message" class="max-w-[220px] truncate text-xs text-red-500" :title="job.email_error_message">
                                {{ job.email_error_message }}
                              </div>
                            </td>
                            <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">
                              {{ job.retry_count }} 次
                              <span v-if="job.next_retry_at" class="text-xs text-gray-500">· {{ formatDateTime(job.next_retry_at) }}</span>
                            </td>
                            <td class="px-3 py-2 text-right">
                              <div class="flex justify-end gap-2">
                                <button
                                  v-if="job.status === 'queued' || job.status === 'running'"
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="cancelJob(job.id)"
                                >
                                  取消
                                </button>
                                <button
                                  v-if="job.status === 'failed' || job.status === 'canceled' || job.status === 'running'"
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="resetJob(job.id)"
                                >
                                  恢复
                                </button>
                                <button
                                  v-if="job.status === 'failed' || job.status === 'canceled' || job.status === 'succeeded' || job.status === 'partial'"
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="rerunJob(job.id)"
                                >
                                  重新生成
                                </button>
                                <button class="btn btn-secondary btn-sm" :disabled="isJobActionPending(job.id) || !canSendJobEmail(job)" @click="sendJobEmail(job.id)">
                                  发送邮件
                                </button>
                                <button
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="selectJobConversations(job)"
                                >
                                  查看对话
                                </button>
                                <button
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="selectJobChunks(job)"
                                >
                                  查看分片
                                </button>
                                <button
                                  v-if="canViewTestResult(job)"
                                  class="btn btn-secondary btn-sm"
                                  :disabled="isJobActionPending(job.id)"
                                  @click="selectTestResult(job)"
                                >
                                  查看结果
                                </button>
                                <button class="btn btn-danger btn-sm" :disabled="isJobActionPending(job.id)" @click="deleteJob(job.id)">
                                  删除
                                </button>
                              </div>
                            </td>
                          </tr>
                          <tr v-if="!(batchJobs[batch.id] || []).length">
                            <td colspan="8" class="px-3 py-5 text-center text-sm text-gray-500">暂无子任务</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </td>
                </tr>
              </template>
              <tr v-if="!batches.length">
                <td colspan="7" class="px-4 py-8 text-center text-sm text-gray-500">暂无队列批次</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="jobTotal > 0"
          :page="jobPage"
          :page-size="jobPageSize"
          :total="jobTotal"
          show-jump
          @update:page="handleJobPageChange"
          @update:page-size="handleJobPageSizeChange"
        />
      </section>

      <section class="grid gap-6 xl:grid-cols-[minmax(0,460px)_minmax(0,1fr)]">
        <div class="card overflow-hidden">
          <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">报告</h2>
              <button class="btn btn-secondary btn-sm" :disabled="loadingReports" @click="refreshReports">
                {{ loadingReports ? '刷新中' : '刷新' }}
              </button>
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <div class="relative col-span-2">
                <input
                  v-model="reportUserQuery"
                  type="text"
                  autocomplete="off"
                  class="input w-full pr-14"
                  placeholder="输入用户邮箱"
                  @input="handleReportUserInput"
                  @focus="openReportUserDropdown"
                  @blur="scheduleCloseReportUserDropdown"
                />
                <button
                  v-if="reportUserQuery || reportFilters.user_id"
                  type="button"
                  class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                  @mousedown.prevent
                  @click="clearReportUser"
                >
                  清除
                </button>
                <div
                  v-if="showReportUserDropdown && (reportUserLoading || reportUserOptions.length > 0 || reportUserQuery.trim())"
                  class="absolute left-0 right-0 top-full z-20 mt-1 max-h-56 overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                >
                  <div v-if="reportUserLoading" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">搜索中...</div>
                  <button
                    v-for="user in reportUserOptions"
                    :key="user.id"
                    type="button"
                    class="flex w-full flex-col px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
                    @mousedown.prevent="selectReportUser(user)"
                  >
                    <span class="text-sm font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                    <span v-if="user.username" class="text-xs text-gray-500 dark:text-gray-400">{{ user.username }}</span>
                  </button>
                  <div v-if="!reportUserLoading && reportUserQuery.trim() && !reportUserOptions.length" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">未找到用户</div>
                </div>
              </div>
              <div class="col-span-2 inline-flex rounded-md border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900">
                <button
                  type="button"
                  class="flex-1 rounded px-3 py-1.5 text-sm transition"
                  :class="reportGroupBy === 'period' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'"
                  @click="setReportGroupBy('period')"
                >
                  按日期
                </button>
                <button
                  type="button"
                  class="flex-1 rounded px-3 py-1.5 text-sm transition"
                  :class="reportGroupBy === 'user' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100'"
                  @click="setReportGroupBy('user')"
                >
                  按用户
                </button>
              </div>
              <select v-model="reportFilters.period_type" class="input" @change="handleReportFilterChange">
                <option value="">全部</option>
                <option value="daily">日报</option>
                <option value="weekly">周报</option>
                <option value="monthly">月报</option>
              </select>
              <div class="grid grid-cols-2 gap-2">
                <input v-model="reportFilters.start_date" type="date" class="input min-w-0" title="开始日期" @change="handleReportFilterChange" />
                <input v-model="reportFilters.end_date" type="date" class="input min-w-0" title="结束日期" @change="handleReportFilterChange" />
              </div>
            </div>
          </div>
          <div class="max-h-[680px] overflow-y-auto divide-y divide-gray-100 dark:divide-dark-700">
            <div v-for="group in reportGroups" :key="group.key" class="divide-y divide-gray-100 dark:divide-dark-700">
              <div class="flex items-center gap-3 bg-gray-50 px-5 py-3 hover:bg-gray-100 dark:bg-dark-800/70 dark:hover:bg-dark-800">
                <button type="button" class="min-w-0 flex-1 text-left" @click="toggleReportGroup(group.key)">
                  <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ groupTitle(group) }}</p>
                  <p class="mt-1 truncate text-xs text-gray-500">{{ groupSubtitle(group) }}</p>
                </button>
                <button type="button" class="shrink-0 text-xs text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200" @click="toggleReportGroup(group.key)">
                  {{ isReportGroupExpanded(group.key) ? '收起' : '展开' }}
                </button>
                <button
                  type="button"
                  class="btn btn-danger btn-sm shrink-0"
                  :disabled="isReportGroupDeletePending(group.key)"
                  @click.stop="deleteReportGroup(group)"
                >
                  删除
                </button>
              </div>
              <div v-if="isReportGroupExpanded(group.key)" class="divide-y divide-gray-100 dark:divide-dark-700">
                <button
                  v-for="report in group.reports"
                  :key="report.id"
                  class="block w-full px-5 py-3 text-left hover:bg-gray-50 dark:hover:bg-dark-800"
                  @click="selectReport(report.id)"
                >
                  <div class="flex items-center justify-between gap-3">
                    <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ report.title }}</p>
                    <span class="text-xs text-gray-500">{{ reportGroupBy === 'period' ? report.user_email || report.user_id : periodLabel(report.period_type) }}</span>
                  </div>
                  <p class="mt-1 truncate text-xs text-gray-500">{{ report.user_email || report.user_id }} · {{ formatDate(report.period_start) }} 至 {{ formatDate(report.period_end) }}</p>
                </button>
              </div>
            </div>
            <div v-if="!reportGroups.length" class="px-5 py-8 text-center text-sm text-gray-500">暂无报告</div>
          </div>
          <div
            v-if="reportTotal > 0"
            class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 bg-white px-4 py-2 text-xs text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300"
          >
            <span>显示 {{ reportFromItem }}-{{ reportToItem }} / 共 {{ reportTotal }} 个分组</span>
            <div class="flex items-center gap-2">
              <span>每页</span>
              <select v-model.number="reportPageSize" class="input h-8 w-20 py-1 text-xs" @change="handleReportPageSizeChange(reportPageSize)">
                <option v-for="size in reportPageSizeOptions" :key="size" :value="size">{{ size }}</option>
              </select>
              <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="reportPage <= 1" @click="handleReportPageChange(reportPage - 1)">上一页</button>
              <span class="min-w-[4rem] text-center">{{ reportPage }} / {{ reportTotalPages }}</span>
              <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="reportPage >= reportTotalPages" @click="handleReportPageChange(reportPage + 1)">下一页</button>
            </div>
          </div>
        </div>

        <div class="card p-5">
          <div v-if="selectedConversationJob" class="space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">去重后对话 JSON #{{ selectedConversationJob.id }}</h2>
                <p class="mt-1 text-xs text-gray-500">
                  {{ selectedConversationJob.user_email || selectedConversationJob.user_id || '-' }}
                  <span v-if="selectedConversationJob.group_name"> · {{ selectedConversationJob.group_name }}</span>
                  · 显示 {{ selectedConversationFromItem }}-{{ selectedConversationToItem }} / 共 {{ selectedConversationTotal }} 个对话
                </p>
              </div>
              <button class="btn btn-secondary" @click="clearSelection">关闭</button>
            </div>
            <div v-if="selectedConversationsLoading" class="py-12 text-center text-sm text-gray-500">加载对话中...</div>
            <div v-else-if="selectedConversations.length" class="space-y-3">
              <details v-for="conversation in selectedConversations" :key="conversation.id" class="rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
                <summary class="cursor-pointer px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200">
                  对话 {{ conversation.conversation_index }} · usage #{{ conversation.usage_log_id || '-' }} · 覆盖 {{ conversation.covered_request_count }} 条 · 估算 {{ conversation.token_estimated }} token
                </summary>
                <pre class="max-h-[480px] overflow-auto border-t border-gray-100 p-3 text-xs leading-relaxed text-gray-700 dark:border-dark-700 dark:text-gray-200">{{ formatChunkJSON(conversation.conversation_json) }}</pre>
              </details>
            </div>
            <div v-else class="py-12 text-center text-sm text-gray-500">暂无去重后对话。任务准备 source 分片后会写入独立对话列表。</div>
            <div
              v-if="selectedConversationTotal > 0"
              class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 pt-3 text-xs text-gray-600 dark:border-dark-700 dark:text-gray-300"
            >
              <span>显示 {{ selectedConversationFromItem }}-{{ selectedConversationToItem }} / 共 {{ selectedConversationTotal }} 个对话</span>
              <div class="flex items-center gap-2">
                <span>每页</span>
                <select v-model.number="selectedConversationPageSize" class="input h-8 w-20 py-1 text-xs" :disabled="selectedConversationsLoading" @change="handleSelectedConversationPageSizeChange(selectedConversationPageSize)">
                  <option v-for="size in selectedConversationPageSizeOptions" :key="size" :value="size">{{ size }}</option>
                </select>
                <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="selectedConversationsLoading || selectedConversationPage <= 1" @click="handleSelectedConversationPageChange(selectedConversationPage - 1)">上一页</button>
                <span class="min-w-[4rem] text-center">{{ selectedConversationPage }} / {{ selectedConversationTotalPages }}</span>
                <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="selectedConversationsLoading || selectedConversationPage >= selectedConversationTotalPages" @click="handleSelectedConversationPageChange(selectedConversationPage + 1)">下一页</button>
              </div>
            </div>
          </div>
          <div v-else-if="selectedChunkJob" class="space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">压缩后请求 JSON 分片 #{{ selectedChunkJob.id }}</h2>
                <p class="mt-1 text-xs text-gray-500">
                  {{ selectedChunkJob.user_email || selectedChunkJob.user_id || '-' }}
                  <span v-if="selectedChunkJob.group_name"> · {{ selectedChunkJob.group_name }}</span>
                  · 显示 {{ selectedChunkFromItem }}-{{ selectedChunkToItem }} / 共 {{ selectedChunkTotal }} 个请求分片
                </p>
              </div>
              <button class="btn btn-secondary" @click="clearSelection">关闭</button>
            </div>
            <div v-if="selectedChunksLoading" class="py-12 text-center text-sm text-gray-500">加载请求分片中...</div>
            <div v-else-if="selectedChunks.length" class="space-y-3">
              <details v-for="chunk in selectedChunks" :key="chunk.id" class="rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
                <summary class="cursor-pointer px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200">
                  {{ chunkSummaryText(chunk) }}
                </summary>
                <div v-if="chunk.error_message" class="border-t border-red-100 bg-red-50 px-3 py-3 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-200">
                  <div class="font-medium">错误原因</div>
                  <div class="mt-1 whitespace-pre-wrap leading-relaxed">{{ chunk.error_message }}</div>
                  <div class="mt-2 text-red-600 dark:text-red-300">
                    重试 {{ chunk.retry_count || 0 }}/15
                    <span v-if="chunk.last_error_at"> · 最近错误 {{ formatDateTime(chunk.last_error_at) }}</span>
                    <span> · 输入/输出 {{ formatCompactNumber(chunk.input_tokens) }} / {{ formatCompactNumber(chunk.output_tokens) }} token</span>
                  </div>
                </div>
                <pre class="max-h-[480px] overflow-auto border-t border-gray-100 p-3 text-xs leading-relaxed text-gray-700 dark:border-dark-700 dark:text-gray-200">{{ formatChunkJSON(chunk.content_json) }}</pre>
              </details>
            </div>
            <div v-else class="py-12 text-center text-sm text-gray-500">暂无请求分片。任务开始生成后会写入压缩后的请求 JSON。</div>
            <div
              v-if="selectedChunkTotal > 0"
              class="flex flex-wrap items-center justify-between gap-2 border-t border-gray-200 pt-3 text-xs text-gray-600 dark:border-dark-700 dark:text-gray-300"
            >
              <span>显示 {{ selectedChunkFromItem }}-{{ selectedChunkToItem }} / 共 {{ selectedChunkTotal }} 个请求分片</span>
              <div class="flex items-center gap-2">
                <span>每页</span>
                <select v-model.number="selectedChunkPageSize" class="input h-8 w-20 py-1 text-xs" :disabled="selectedChunksLoading" @change="handleSelectedChunkPageSizeChange(selectedChunkPageSize)">
                  <option v-for="size in selectedChunkPageSizeOptions" :key="size" :value="size">{{ size }}</option>
                </select>
                <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="selectedChunksLoading || selectedChunkPage <= 1" @click="handleSelectedChunkPageChange(selectedChunkPage - 1)">上一页</button>
                <span class="min-w-[4rem] text-center">{{ selectedChunkPage }} / {{ selectedChunkTotalPages }}</span>
                <button class="btn btn-secondary btn-sm h-8 px-2" :disabled="selectedChunksLoading || selectedChunkPage >= selectedChunkTotalPages" @click="handleSelectedChunkPageChange(selectedChunkPage + 1)">下一页</button>
              </div>
            </div>
          </div>
          <div v-else-if="selectedTestJob" class="space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">测试生成结果 #{{ selectedTestJob.id }}</h2>
                <p class="mt-1 text-xs text-gray-500">
                  {{ selectedTestJob.user_email || selectedTestJob.user_id || '-' }}
                  <span v-if="selectedTestJob.group_name"> · {{ selectedTestJob.group_name }}</span>
                  · {{ formatDate(selectedTestJob.range_start) }} 至 {{ formatDate(selectedTestJob.range_end) }}
                </p>
              </div>
              <button class="btn btn-secondary" @click="clearSelection">关闭</button>
            </div>
            <MarkdownContent :content="selectedTestJob.result_md || ''" />
          </div>
          <div v-else-if="selectedReport" class="space-y-4">
            <div class="flex flex-wrap items-center gap-3">
              <input v-model="editForm.title" class="input flex-1" />
              <button class="btn btn-primary" :disabled="savingReport" @click="saveReport">保存</button>
              <button class="btn btn-danger" :disabled="savingReport" @click="deleteReport">删除</button>
            </div>
            <textarea v-model="editForm.content_md" rows="14" class="input font-mono text-sm"></textarea>
            <MarkdownContent :content="editForm.content_md" />
          </div>
          <div v-else class="py-24 text-center text-sm text-gray-500">请选择报告、测试结果、对话或请求分片</div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import MarkdownContent from '@/components/common/MarkdownContent.vue'
import {
  adminUsageBriefAPI,
  type UsageBriefBatch,
  type UsageBriefJob,
  type UsageBriefJobChunk,
  type UsageBriefJobConversation,
  type UsageBriefPeriodType,
  type UsageBriefReportGroup,
  type UsageBriefReportGroupBy,
  type UsageBriefReportGroupQuery,
  type UsageBriefReport,
  type UsageBriefSettings
} from '@/api/usageBrief'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminGroup, AdminUser } from '@/types'

const appStore = useAppStore()
const reportGroupStorageKey = 'usageBriefReportGroupBy'
const settings = ref<UsageBriefSettings | null>(null)
const settingsForm = reactive({
  base_url: '',
  api_key: '',
  model: 'gpt-5.5',
  context_tokens: 400000,
  output_reserved_tokens: 128000,
  concurrency: 4
})
const savingSettings = ref(false)
const triggering = ref(false)
const loadingJobs = ref(false)
const batchActionIds = ref<Set<number>>(new Set())
const jobActionIds = ref<Set<number>>(new Set())
const jobPage = ref(1)
const jobPageSize = ref(50)
const jobTotal = ref(0)

const productionForm = reactive<{ period_type: UsageBriefPeriodType; period_date: string }>({
  period_type: 'daily',
  period_date: yesterday()
})
const testForm = reactive<{ user_id?: number; group_id?: number; range_start: string; range_end: string }>({
  range_start: yesterday(),
  range_end: new Date().toISOString().slice(0, 10)
})

const jobFilters = reactive({ scope: '', status: '' })
const batches = ref<UsageBriefBatch[]>([])
const expandedBatchIds = ref<Set<number>>(new Set())
const batchJobs = ref<Record<number, UsageBriefJob[]>>({})
const batchJobsLoading = ref<Record<number, boolean>>({})
const groupOptions = ref<AdminGroup[]>([])
const testUserQuery = ref('')
const testUserOptions = ref<AdminUser[]>([])
const testUserLoading = ref(false)
const showTestUserDropdown = ref(false)
const reportUserQuery = ref('')
const reportUserOptions = ref<AdminUser[]>([])
const reportUserLoading = ref(false)
const showReportUserDropdown = ref(false)
const selectedReportUser = ref<AdminUser | null>(null)
const reportFilters = reactive<{ period_type: '' | UsageBriefPeriodType; user_id: '' | number; start_date: string; end_date: string }>({
  period_type: '',
  user_id: '',
  start_date: '',
  end_date: ''
})
const reportGroups = ref<UsageBriefReportGroup[]>([])
const reportGroupBy = ref<UsageBriefReportGroupBy>(readStoredReportGroupBy())
const expandedReportGroupKeys = ref<Set<string>>(new Set())
const reportGroupDeleteKeys = ref<Set<string>>(new Set())
const loadingReports = ref(false)
const reportPage = ref(1)
const reportPageSize = ref(50)
const reportTotal = ref(0)
const reportPageSizeOptions = [10, 20, 50, 100]
const selectedReport = ref<UsageBriefReport | null>(null)
const selectedTestJob = ref<UsageBriefJob | null>(null)
const selectedChunkJob = ref<UsageBriefJob | null>(null)
const selectedChunks = ref<UsageBriefJobChunk[]>([])
const selectedChunksLoading = ref(false)
const selectedChunkPage = ref(1)
const selectedChunkPageSize = ref(50)
const selectedChunkTotal = ref(0)
const selectedChunkPageSizeOptions = [10, 20, 50, 100]
const selectedConversationJob = ref<UsageBriefJob | null>(null)
const selectedConversations = ref<UsageBriefJobConversation[]>([])
const selectedConversationsLoading = ref(false)
const selectedConversationPage = ref(1)
const selectedConversationPageSize = ref(50)
const selectedConversationTotal = ref(0)
const selectedConversationPageSizeOptions = [10, 20, 50, 100]
const editForm = reactive({ title: '', content_md: '' })
const savingReport = ref(false)
let testUserSearchTimeout: ReturnType<typeof setTimeout> | null = null
let testUserCloseTimeout: ReturnType<typeof setTimeout> | null = null
let testUserAbortController: AbortController | null = null
let reportUserSearchTimeout: ReturnType<typeof setTimeout> | null = null
let reportUserCloseTimeout: ReturnType<typeof setTimeout> | null = null
let reportUserAbortController: AbortController | null = null
let reportLoadRequestID = 0

const reportTotalPages = computed(() => Math.max(1, Math.ceil(reportTotal.value / reportPageSize.value)))
const reportFromItem = computed(() => (reportTotal.value === 0 ? 0 : (reportPage.value - 1) * reportPageSize.value + 1))
const reportToItem = computed(() => Math.min(reportPage.value * reportPageSize.value, reportTotal.value))
const selectedChunkTotalPages = computed(() => Math.max(1, Math.ceil(selectedChunkTotal.value / selectedChunkPageSize.value)))
const selectedChunkFromItem = computed(() => (selectedChunkTotal.value === 0 ? 0 : (selectedChunkPage.value - 1) * selectedChunkPageSize.value + 1))
const selectedChunkToItem = computed(() => Math.min(selectedChunkPage.value * selectedChunkPageSize.value, selectedChunkTotal.value))
const selectedConversationTotalPages = computed(() => Math.max(1, Math.ceil(selectedConversationTotal.value / selectedConversationPageSize.value)))
const selectedConversationFromItem = computed(() => (selectedConversationTotal.value === 0 ? 0 : (selectedConversationPage.value - 1) * selectedConversationPageSize.value + 1))
const selectedConversationToItem = computed(() => Math.min(selectedConversationPage.value * selectedConversationPageSize.value, selectedConversationTotal.value))

function yesterday() {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  return d.toISOString().slice(0, 10)
}

function readStoredReportGroupBy(): UsageBriefReportGroupBy {
  if (typeof window === 'undefined') return 'period'
  return window.localStorage.getItem(reportGroupStorageKey) === 'user' ? 'user' : 'period'
}

function formatDate(value?: string) {
  return value ? value.slice(0, 10) : '-'
}

function formatDateTime(value?: string) {
  if (!value) return '-'
  return value.replace('T', ' ').slice(0, 16)
}

function periodLabel(value: string) {
  return value === 'weekly' ? '周报' : value === 'monthly' ? '月报' : '日报'
}

function groupTitle(group: UsageBriefReportGroup) {
  if (group.group_by === 'user') {
    return group.user_email || group.username || `用户 #${group.user_id || '-'}`
  }
  return group.label || `${periodLabel(group.period_type || 'daily')} ${formatDate(group.period_start)} 至 ${formatDate(group.period_end)}`
}

function groupSubtitle(group: UsageBriefReportGroup) {
  const tokenText = `Token ${formatCompactNumber(group.input_tokens)} / ${formatCompactNumber(group.output_tokens)}`
  if (group.group_by === 'user') {
    return `${group.report_count} 份报告 · ${tokenText}`
  }
  return `${group.user_count} 个用户 · ${group.report_count} 份报告 · ${tokenText}`
}

function statusLabel(value: string) {
  const labels: Record<string, string> = {
    queued: '排队中',
    running: '生成中',
    succeeded: '已完成',
    failed: '失败',
    canceled: '已取消',
    paused: '已暂停',
    partial: '部分完成'
  }
  return labels[value] || value
}

function emailStatusLabel(value?: string) {
  const labels: Record<string, string> = {
    pending: '未发送',
    sent: '已发送',
    failed: '发送失败',
    skipped: '已跳过'
  }
  return labels[value || 'pending'] || value || '未发送'
}

function chunkSummaryText(chunk: UsageBriefJobChunk) {
  const parts = [`分片 ${chunk.chunk_index}`, chunk.status === 'failed' && (chunk.retry_count || 0) >= 15 ? '失败已跳过' : statusLabel(chunk.status)]
  if (chunk.retry_count) parts.push(`重试 ${chunk.retry_count}/15`)
  parts.push(`估算 ${formatCompactNumber(chunk.token_estimated)} token`)
  if (chunk.error_message) parts.push('查看错误原因')
  return parts.join(' · ')
}

function batchPeriod(batch: UsageBriefBatch) {
  if (batch.batch_scope === 'test') return `${formatDate(batch.range_start)} 至 ${formatDate(batch.range_end)}`
  return `${periodLabel(batch.period_type || 'daily')} · ${formatDate(batch.period_start)} 至 ${formatDate(batch.period_end)}`
}

function tokenProgress(value: Pick<UsageBriefBatch, 'token_estimated_processed' | 'token_estimated_total' | 'input_tokens' | 'output_tokens'>) {
  const total = value.token_estimated_total || 0
  const processed = Math.min(value.token_estimated_processed || 0, total || value.token_estimated_processed || 0)
  const remain = Math.max(total - processed, 0)
  const estimated = total > 0 ? `${formatCompactNumber(processed)} / ${formatCompactNumber(total)}，剩余 ${formatCompactNumber(remain)}` : '未估算'
  const actual = value.input_tokens || value.output_tokens ? `实际 ${formatCompactNumber(value.input_tokens)}/${formatCompactNumber(value.output_tokens)}` : ''
  return actual ? `${estimated} · ${actual}` : estimated
}

function formatCompactNumber(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0'
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`
  return String(Math.round(value))
}

function jobStageLabel(job: UsageBriefJob) {
  const labels: Record<string, string> = {
    queued: '排队',
    running: '运行',
    chunking: '分片',
    summarizing_chunks: '分片总结',
    merged: '合并',
    waiting_retry: '等待重试',
    completed: '完成',
    failed: '失败',
    canceled: '取消'
  }
  return labels[job.stage || ''] || job.stage || '-'
}

function isBatchExpanded(id: number) {
  return expandedBatchIds.value.has(id)
}

function isBatchActionPending(id: number) {
  return batchActionIds.value.has(id)
}

function isJobActionPending(id: number) {
  return jobActionIds.value.has(id)
}

function isReportGroupExpanded(key: string) {
  return expandedReportGroupKeys.value.has(key)
}

function isReportGroupDeletePending(key: string) {
  return reportGroupDeleteKeys.value.has(key)
}

function toggleReportGroup(key: string) {
  const next = new Set(expandedReportGroupKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  expandedReportGroupKeys.value = next
}

async function setReportGroupBy(value: UsageBriefReportGroupBy) {
  if (reportGroupBy.value === value) return
  reportGroupBy.value = value
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(reportGroupStorageKey, value)
  }
  reportPage.value = 1
  clearSelection()
  await loadReports()
}

function buildReportGroupQuery(): UsageBriefReportGroupQuery {
  const params: UsageBriefReportGroupQuery = {
    page: reportPage.value,
    page_size: reportPageSize.value,
    period_type: reportFilters.period_type,
    group_by: reportGroupBy.value
  }
  if (typeof reportFilters.user_id === 'number' && reportFilters.user_id > 0) {
    params.user_id = reportFilters.user_id
  }
  if (reportFilters.start_date) {
    params.start_date = reportFilters.start_date
  }
  if (reportFilters.end_date) {
    params.end_date = reportFilters.end_date
  }
  return params
}

function canViewTestResult(job: UsageBriefJob) {
  return job.job_scope === 'test' && (job.status === 'succeeded' || job.status === 'partial') && Boolean(job.result_md)
}

function canSendJobEmail(job: UsageBriefJob) {
  return (job.status === 'succeeded' || job.status === 'partial') && job.email_status !== 'sent' && Boolean(job.user_email || job.user_id)
}

function clearSelection() {
  selectedReport.value = null
  selectedTestJob.value = null
  selectedChunkJob.value = null
  selectedConversationJob.value = null
  selectedChunks.value = []
  selectedChunkPage.value = 1
  selectedChunkTotal.value = 0
  selectedChunksLoading.value = false
  selectedConversations.value = []
  selectedConversationPage.value = 1
  selectedConversationTotal.value = 0
  selectedConversationsLoading.value = false
  editForm.title = ''
  editForm.content_md = ''
}

function selectTestResult(job: UsageBriefJob) {
  selectedReport.value = null
  selectedChunkJob.value = null
  selectedConversationJob.value = null
  selectedChunks.value = []
  selectedChunkTotal.value = 0
  selectedConversations.value = []
  selectedConversationTotal.value = 0
  selectedTestJob.value = job
  editForm.title = ''
  editForm.content_md = ''
}

async function selectJobChunks(job: UsageBriefJob) {
  selectedReport.value = null
  selectedTestJob.value = null
  selectedConversationJob.value = null
  selectedChunkJob.value = job
  selectedChunks.value = []
  selectedChunkPage.value = 1
  selectedChunkTotal.value = 0
  selectedConversations.value = []
  selectedConversationTotal.value = 0
  editForm.title = ''
  editForm.content_md = ''
  await loadSelectedJobChunks(job.id)
}

async function loadSelectedJobChunks(jobID = selectedChunkJob.value?.id) {
  if (!jobID) return
  selectedChunksLoading.value = true
  try {
    const res = await adminUsageBriefAPI.listJobChunks(jobID, {
      chunk_type: 'source',
      page: selectedChunkPage.value,
      page_size: selectedChunkPageSize.value
    })
    if (selectedChunkJob.value?.id === jobID) {
      selectedChunks.value = res.items || []
      selectedChunkTotal.value = res.total || 0
      if (selectedChunkTotal.value > 0 && selectedChunks.value.length === 0 && selectedChunkPage.value > 1) {
        selectedChunkPage.value = Math.max(1, Math.ceil(selectedChunkTotal.value / selectedChunkPageSize.value))
        await loadSelectedJobChunks(jobID)
      }
    }
  } catch (error: any) {
    appStore.showError(error?.message || '加载分片失败')
    if (selectedChunkJob.value?.id === jobID) {
      selectedChunks.value = []
      selectedChunkTotal.value = 0
    }
  } finally {
    if (selectedChunkJob.value?.id === jobID) {
      selectedChunksLoading.value = false
    }
  }
}

async function selectJobConversations(job: UsageBriefJob) {
  selectedReport.value = null
  selectedTestJob.value = null
  selectedChunkJob.value = null
  selectedConversationJob.value = job
  selectedChunks.value = []
  selectedChunkTotal.value = 0
  selectedConversations.value = []
  selectedConversationPage.value = 1
  selectedConversationTotal.value = 0
  editForm.title = ''
  editForm.content_md = ''
  await loadSelectedJobConversations(job.id)
}

async function loadSelectedJobConversations(jobID = selectedConversationJob.value?.id) {
  if (!jobID) return
  selectedConversationsLoading.value = true
  try {
    const res = await adminUsageBriefAPI.listJobConversations(jobID, {
      page: selectedConversationPage.value,
      page_size: selectedConversationPageSize.value
    })
    if (selectedConversationJob.value?.id === jobID) {
      selectedConversations.value = res.items || []
      selectedConversationTotal.value = res.total || 0
      if (selectedConversationTotal.value > 0 && selectedConversations.value.length === 0 && selectedConversationPage.value > 1) {
        selectedConversationPage.value = Math.max(1, Math.ceil(selectedConversationTotal.value / selectedConversationPageSize.value))
        await loadSelectedJobConversations(jobID)
      }
    }
  } catch (error: any) {
    appStore.showError(error?.message || '加载对话失败')
    if (selectedConversationJob.value?.id === jobID) {
      selectedConversations.value = []
      selectedConversationTotal.value = 0
    }
  } finally {
    if (selectedConversationJob.value?.id === jobID) {
      selectedConversationsLoading.value = false
    }
  }
}

async function handleSelectedChunkPageChange(page: number) {
  selectedChunkPage.value = page
  await loadSelectedJobChunks()
}

async function handleSelectedChunkPageSizeChange(pageSize: number) {
  selectedChunkPageSize.value = pageSize
  selectedChunkPage.value = 1
  await loadSelectedJobChunks()
}

async function handleSelectedConversationPageChange(page: number) {
  selectedConversationPage.value = page
  await loadSelectedJobConversations()
}

async function handleSelectedConversationPageSizeChange(pageSize: number) {
  selectedConversationPageSize.value = pageSize
  selectedConversationPage.value = 1
  await loadSelectedJobConversations()
}

function formatChunkJSON(raw?: string) {
  const text = raw || ''
  if (!text.trim()) return '{}'
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

async function withJobAction(id: number, action: () => Promise<void>) {
  const next = new Set(jobActionIds.value)
  next.add(id)
  jobActionIds.value = next
  try {
    await action()
  } finally {
    const after = new Set(jobActionIds.value)
    after.delete(id)
    jobActionIds.value = after
  }
}

async function withBatchAction(id: number, action: () => Promise<void>) {
  const next = new Set(batchActionIds.value)
  next.add(id)
  batchActionIds.value = next
  try {
    await action()
  } finally {
    const after = new Set(batchActionIds.value)
    after.delete(id)
    batchActionIds.value = after
  }
}

async function loadSettings() {
  settings.value = await adminUsageBriefAPI.getSettings()
  Object.assign(settingsForm, {
    base_url: settings.value.base_url,
    api_key: '',
    model: settings.value.model,
    context_tokens: settings.value.context_tokens,
    output_reserved_tokens: settings.value.output_reserved_tokens,
    concurrency: settings.value.concurrency
  })
}

async function loadSelectors() {
  const groups = await adminAPI.groups.getAll()
  groupOptions.value = groups || []
}

function clearSearchTimeout(timeout: ReturnType<typeof setTimeout> | null) {
  if (timeout) clearTimeout(timeout)
}

function cancelTestUserSearch() {
  clearSearchTimeout(testUserSearchTimeout)
  testUserSearchTimeout = null
  if (testUserAbortController) {
    testUserAbortController.abort()
    testUserAbortController = null
  }
  testUserLoading.value = false
}

function cancelReportUserSearch() {
  clearSearchTimeout(reportUserSearchTimeout)
  reportUserSearchTimeout = null
  if (reportUserAbortController) {
    reportUserAbortController.abort()
    reportUserAbortController = null
  }
  reportUserLoading.value = false
}

function openTestUserDropdown() {
  clearSearchTimeout(testUserCloseTimeout)
  if (testUserQuery.value.trim() || testUserOptions.value.length > 0) {
    showTestUserDropdown.value = true
  }
}

function scheduleCloseTestUserDropdown() {
  clearSearchTimeout(testUserCloseTimeout)
  testUserCloseTimeout = setTimeout(() => {
    showTestUserDropdown.value = false
  }, 120)
}

function openReportUserDropdown() {
  clearSearchTimeout(reportUserCloseTimeout)
  if (reportUserQuery.value.trim() || reportUserOptions.value.length > 0) {
    showReportUserDropdown.value = true
  }
}

function scheduleCloseReportUserDropdown() {
  clearSearchTimeout(reportUserCloseTimeout)
  reportUserCloseTimeout = setTimeout(() => {
    showReportUserDropdown.value = false
  }, 120)
}

function handleTestUserInput() {
  testForm.user_id = undefined
  const keyword = testUserQuery.value.trim()
  cancelTestUserSearch()
  if (!keyword) {
    testUserOptions.value = []
    showTestUserDropdown.value = false
    return
  }
  showTestUserDropdown.value = true
  testUserSearchTimeout = setTimeout(searchTestUsers, 300)
}

async function searchTestUsers() {
  const keyword = testUserQuery.value.trim()
  if (!keyword) return
  const controller = new AbortController()
  testUserAbortController = controller
  testUserLoading.value = true
  try {
    const res = await adminAPI.users.list(1, 10, { role: 'user', search: keyword }, { signal: controller.signal })
    if (controller.signal.aborted || keyword !== testUserQuery.value.trim()) return
    testUserOptions.value = res.items || []
    showTestUserDropdown.value = true
  } catch (error: any) {
    if (error?.name !== 'AbortError' && error?.code !== 'ERR_CANCELED') {
      testUserOptions.value = []
    }
  } finally {
    if (testUserAbortController === controller) {
      testUserAbortController = null
      testUserLoading.value = false
    }
  }
}

function selectTestUser(user: AdminUser) {
  testForm.user_id = user.id
  testUserQuery.value = user.email
  testUserOptions.value = []
  showTestUserDropdown.value = false
}

function clearTestUser() {
  cancelTestUserSearch()
  testForm.user_id = undefined
  testUserQuery.value = ''
  testUserOptions.value = []
  showTestUserDropdown.value = false
}

function handleReportUserInput() {
  const keyword = reportUserQuery.value.trim()
  if (selectedReportUser.value && keyword !== selectedReportUser.value.email) {
    selectedReportUser.value = null
  }
  if (reportFilters.user_id !== '') {
    reportFilters.user_id = ''
    void handleReportFilterChange()
  }
  cancelReportUserSearch()
  if (!keyword) {
    reportUserOptions.value = []
    showReportUserDropdown.value = false
    return
  }
  showReportUserDropdown.value = true
  reportUserSearchTimeout = setTimeout(searchReportUsers, 300)
}

async function searchReportUsers() {
  const keyword = reportUserQuery.value.trim()
  if (!keyword) return
  const controller = new AbortController()
  reportUserAbortController = controller
  reportUserLoading.value = true
  try {
    const res = await adminAPI.users.list(1, 10, { role: 'user', search: keyword }, { signal: controller.signal })
    if (controller.signal.aborted || keyword !== reportUserQuery.value.trim()) return
    reportUserOptions.value = res.items || []
    showReportUserDropdown.value = true
  } catch (error: any) {
    if (error?.name !== 'AbortError' && error?.code !== 'ERR_CANCELED') {
      reportUserOptions.value = []
    }
  } finally {
    if (reportUserAbortController === controller) {
      reportUserAbortController = null
      reportUserLoading.value = false
    }
  }
}

function selectReportUser(user: AdminUser) {
  selectedReportUser.value = user
  reportFilters.user_id = user.id
  reportUserQuery.value = user.email
  reportUserOptions.value = []
  showReportUserDropdown.value = false
  void handleReportFilterChange()
}

function clearReportUser() {
  cancelReportUserSearch()
  selectedReportUser.value = null
  reportUserQuery.value = ''
  reportUserOptions.value = []
  showReportUserDropdown.value = false
  if (reportFilters.user_id !== '') {
    reportFilters.user_id = ''
    void handleReportFilterChange()
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    settings.value = await adminUsageBriefAPI.updateSettings({
      base_url: settingsForm.base_url,
      api_key: settingsForm.api_key || undefined,
      model: settingsForm.model,
      context_tokens: settingsForm.context_tokens,
      output_reserved_tokens: settingsForm.output_reserved_tokens,
      concurrency: settingsForm.concurrency
    })
    settingsForm.api_key = ''
    appStore.showSuccess('已保存用量简报配置')
  } catch (error: any) {
    appStore.showError(error?.message || '保存配置失败')
  } finally {
    savingSettings.value = false
  }
}

async function clearAPIKey() {
  savingSettings.value = true
  try {
    settings.value = await adminUsageBriefAPI.updateSettings({ clear_api_key: true })
    settingsForm.api_key = ''
    appStore.showSuccess('已清除 API Key')
  } catch (error: any) {
    appStore.showError(error?.message || '清除失败')
  } finally {
    savingSettings.value = false
  }
}

async function triggerProduction() {
  triggering.value = true
  try {
    const payload: { period_type: UsageBriefPeriodType; period_date: string } = {
      period_type: productionForm.period_type,
      period_date: productionForm.period_date
    }
    const res = await adminUsageBriefAPI.triggerProduction(payload)
    appStore.showSuccess(`已加入 ${res.count} 个生产任务`)
    jobPage.value = 1
    await loadJobs()
  } catch (error: any) {
    appStore.showError(error?.message || '触发失败')
  } finally {
    triggering.value = false
  }
}

async function createTestJob() {
  if (!testForm.user_id) {
    appStore.showError('请选择普通用户')
    return
  }
  triggering.value = true
  try {
    const payload: { user_id: number; group_id?: number; range_start: string; range_end: string } = {
      user_id: testForm.user_id,
      range_start: testForm.range_start,
      range_end: testForm.range_end
    }
    if (testForm.group_id) payload.group_id = testForm.group_id
    await adminUsageBriefAPI.createTestJob(payload)
    appStore.showSuccess('已加入测试队列')
    jobPage.value = 1
    await loadJobs()
  } catch (error: any) {
    appStore.showError(error?.message || '创建测试任务失败')
  } finally {
    triggering.value = false
  }
}

async function toggleBatch(batch: UsageBriefBatch) {
  const next = new Set(expandedBatchIds.value)
  if (next.has(batch.id)) {
    next.delete(batch.id)
    expandedBatchIds.value = next
    return
  }
  next.add(batch.id)
  expandedBatchIds.value = next
  await loadBatchJobs(batch.id)
}

async function loadBatchJobs(batchID: number) {
  batchJobsLoading.value = { ...batchJobsLoading.value, [batchID]: true }
  try {
    const res = await adminUsageBriefAPI.listBatchJobs(batchID, { page: 1, page_size: 1000 })
    batchJobs.value = { ...batchJobs.value, [batchID]: res.items || [] }
  } catch (error: any) {
    appStore.showError(error?.message || '加载批次任务失败')
  } finally {
    batchJobsLoading.value = { ...batchJobsLoading.value, [batchID]: false }
  }
}

async function refreshExpandedBatchJobs() {
  const visible = new Set(batches.value.map((batch) => batch.id))
  const ids = Array.from(expandedBatchIds.value).filter((id) => visible.has(id))
  await Promise.all(ids.map((id) => loadBatchJobs(id)))
}

async function loadJobs(): Promise<boolean> {
  loadingJobs.value = true
  try {
    const res = await adminUsageBriefAPI.listBatches({ page: jobPage.value, page_size: jobPageSize.value, scope: jobFilters.scope, status: jobFilters.status })
    batches.value = res.items || []
    jobTotal.value = res.total || 0
    if (jobTotal.value > 0 && batches.value.length === 0 && jobPage.value > 1) {
      jobPage.value = Math.max(1, Math.ceil(jobTotal.value / jobPageSize.value))
      return loadJobs()
    }
    await refreshExpandedBatchJobs()
    return true
  } catch (error: any) {
    appStore.showError(error?.message || '加载队列失败')
    return false
  } finally {
    loadingJobs.value = false
  }
}

async function handleJobFilterChange() {
  jobPage.value = 1
  await loadJobs()
}

async function handleJobPageChange(page: number) {
  jobPage.value = page
  await loadJobs()
}

async function handleJobPageSizeChange(pageSize: number) {
  jobPageSize.value = pageSize
  jobPage.value = 1
  await loadJobs()
}

async function refreshJobs() {
  if (await loadJobs()) {
    appStore.showSuccess('队列已刷新')
  }
}

async function pauseBatch(id: number) {
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.pauseBatch(id)
      appStore.showSuccess('批次已暂停')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '暂停批次失败')
    }
  })
}

async function resumeBatch(id: number) {
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.resumeBatch(id)
      appStore.showSuccess('批次已恢复')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '恢复批次失败')
    }
  })
}

async function cancelBatch(id: number) {
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.cancelBatch(id)
      appStore.showSuccess('批次已取消')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '取消批次失败')
    }
  })
}

async function resetBatch(id: number) {
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.resetBatch(id)
      appStore.showSuccess('批次已恢复，将复用未变化的成功分片')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '恢复批次失败')
    }
  })
}

async function rerunBatch(id: number) {
  if (!window.confirm('重新生成会清空该批次已保存的分片进度，并从第一个分片重新开始，确定继续吗？')) {
    return
  }
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.rerunBatch(id)
      appStore.showSuccess('批次已加入重新生成队列')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '重新生成批次失败')
    }
  })
}

async function deleteBatch(id: number) {
  await withBatchAction(id, async () => {
    try {
      await adminUsageBriefAPI.deleteBatch(id)
      if ((batchJobs.value[id] || []).some((job) => selectedTestJob.value?.id === job.id || selectedChunkJob.value?.id === job.id)) {
        clearSelection()
      }
      const nextExpanded = new Set(expandedBatchIds.value)
      nextExpanded.delete(id)
      expandedBatchIds.value = nextExpanded
      const nextJobs = { ...batchJobs.value }
      delete nextJobs[id]
      batchJobs.value = nextJobs
      appStore.showSuccess('批次已删除')
      await loadJobs()
    } catch (error: any) {
      appStore.showError(error?.message || '删除批次失败')
    }
  })
}

function findJobBatchID(jobID: number) {
  for (const [batchID, jobs] of Object.entries(batchJobs.value)) {
    if (jobs.some((job) => job.id === jobID)) {
      return Number(batchID)
    }
  }
  return undefined
}

async function cancelJob(id: number) {
  await withJobAction(id, async () => {
    try {
      await adminUsageBriefAPI.cancelJob(id)
      appStore.showSuccess('任务已取消')
      await loadJobs()
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    } catch (error: any) {
      appStore.showError(error?.message || '取消任务失败')
    }
  })
}

async function resetJob(id: number) {
  await withJobAction(id, async () => {
    try {
      await adminUsageBriefAPI.resetJob(id)
      appStore.showSuccess('任务已恢复，将复用未变化的成功分片')
      await loadJobs()
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    } catch (error: any) {
      appStore.showError(error?.message || '恢复任务失败')
    }
  })
}

async function rerunJob(id: number) {
  if (!window.confirm('重新生成会清空该任务已保存的分片进度，并从第一个分片重新开始，确定继续吗？')) {
    return
  }
  await withJobAction(id, async () => {
    try {
      await adminUsageBriefAPI.rerunJob(id)
      appStore.showSuccess('任务已加入重新生成队列')
      await loadJobs()
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    } catch (error: any) {
      appStore.showError(error?.message || '重新生成任务失败')
    }
  })
}

async function sendJobEmail(id: number) {
  await withJobAction(id, async () => {
    try {
      await adminUsageBriefAPI.sendJobEmail(id)
      appStore.showSuccess('邮件已发送')
      await loadJobs()
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    } catch (error: any) {
      appStore.showError(error?.message || '发送邮件失败')
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    }
  })
}

async function deleteJob(id: number) {
  await withJobAction(id, async () => {
    try {
      await adminUsageBriefAPI.deleteJob(id)
      if (selectedTestJob.value?.id === id || selectedChunkJob.value?.id === id) {
        clearSelection()
      }
      appStore.showSuccess('任务已删除')
      await loadJobs()
      const batchID = findJobBatchID(id)
      if (batchID) await loadBatchJobs(batchID)
    } catch (error: any) {
      appStore.showError(error?.message || '删除任务失败')
    }
  })
}

async function loadReports(): Promise<boolean> {
  const requestID = ++reportLoadRequestID
  loadingReports.value = true
  const params = buildReportGroupQuery()
  try {
    const res = await adminUsageBriefAPI.listReportGroups(params)
    if (requestID !== reportLoadRequestID) return false
    reportGroups.value = res.items || []
    reportTotal.value = res.total || 0
    if (reportTotal.value > 0 && reportGroups.value.length === 0 && reportPage.value > 1) {
      reportPage.value = Math.max(1, Math.ceil(reportTotal.value / reportPageSize.value))
      return loadReports()
    }
    syncExpandedReportGroups()
    return true
  } catch (error: any) {
    if (requestID === reportLoadRequestID) {
      appStore.showError(error?.message || '加载报告失败')
    }
    return false
  } finally {
    if (requestID === reportLoadRequestID) {
      loadingReports.value = false
    }
  }
}

function syncExpandedReportGroups() {
  const visibleKeys = new Set(reportGroups.value.map((group) => group.key))
  const next = new Set(Array.from(expandedReportGroupKeys.value).filter((key) => visibleKeys.has(key)))
  if (next.size === 0 && reportGroups.value.length > 0) {
    next.add(reportGroups.value[0].key)
  }
  expandedReportGroupKeys.value = next
}

async function selectReport(id: number) {
  selectedTestJob.value = null
  selectedChunkJob.value = null
  selectedConversationJob.value = null
  selectedChunks.value = []
  selectedChunkPage.value = 1
  selectedChunkTotal.value = 0
  selectedConversations.value = []
  selectedConversationPage.value = 1
  selectedConversationTotal.value = 0
  selectedReport.value = await adminUsageBriefAPI.getReport(id)
  editForm.title = selectedReport.value.title
  editForm.content_md = selectedReport.value.content_md
}

async function handleReportFilterChange() {
  reportPage.value = 1
  clearSelection()
  await loadReports()
}

async function handleReportPageChange(page: number) {
  reportPage.value = page
  await loadReports()
}

async function handleReportPageSizeChange(pageSize: number) {
  reportPageSize.value = pageSize
  reportPage.value = 1
  await loadReports()
}

async function refreshReports() {
  if (await loadReports()) {
    appStore.showSuccess('报告已刷新')
  }
}

async function saveReport() {
  if (!selectedReport.value) return
  savingReport.value = true
  try {
    selectedReport.value = await adminUsageBriefAPI.updateReport(selectedReport.value.id, { title: editForm.title, content_md: editForm.content_md })
    appStore.showSuccess('报告已保存')
    await loadReports()
  } catch (error: any) {
    appStore.showError(error?.message || '保存报告失败')
  } finally {
    savingReport.value = false
  }
}

async function deleteReport() {
  if (!selectedReport.value) return
  savingReport.value = true
  try {
    await adminUsageBriefAPI.deleteReport(selectedReport.value.id)
    clearSelection()
    appStore.showSuccess('报告已删除')
    await loadReports()
  } catch (error: any) {
    appStore.showError(error?.message || '删除报告失败')
  } finally {
    savingReport.value = false
  }
}

async function deleteReportGroup(group: UsageBriefReportGroup) {
  const title = groupTitle(group)
  if (!window.confirm(`确定删除「${title}」分组下的 ${group.report_count} 份报告吗？此操作只会删除当前筛选范围内的正式报告。`)) {
    return
  }
  const next = new Set(reportGroupDeleteKeys.value)
  next.add(group.key)
  reportGroupDeleteKeys.value = next
  try {
    const reportIDs = new Set(group.reports.map((report) => report.id))
    const res = await adminUsageBriefAPI.deleteReportGroup({
      ...buildReportGroupQuery(),
      group_key: group.key
    })
    if (selectedReport.value && reportIDs.has(selectedReport.value.id)) {
      clearSelection()
    }
    const nextExpanded = new Set(expandedReportGroupKeys.value)
    nextExpanded.delete(group.key)
    expandedReportGroupKeys.value = nextExpanded
    appStore.showSuccess(`已删除 ${res.deleted_count || 0} 份报告`)
    await loadReports()
  } catch (error: any) {
    appStore.showError(error?.message || '删除报告分组失败')
  } finally {
    const after = new Set(reportGroupDeleteKeys.value)
    after.delete(group.key)
    reportGroupDeleteKeys.value = after
  }
}

onMounted(async () => {
  await Promise.all([loadSettings(), loadSelectors(), loadJobs(), loadReports()])
})

onBeforeUnmount(() => {
  cancelTestUserSearch()
  cancelReportUserSearch()
  clearSearchTimeout(testUserCloseTimeout)
  clearSearchTimeout(reportUserCloseTimeout)
})
</script>
