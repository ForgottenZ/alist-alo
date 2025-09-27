import {
  Badge,
  Box,
  Button,
  Checkbox,
  Divider,
  FormControl,
  FormLabel,
  HStack,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Spacer,
  Switch as HopeSwitch,
  Text,
  VStack,
  createDisclosure,
} from "@hope-ui/solid"
import {
  For,
  Show,
  createMemo,
  createSignal,
  onMount,
} from "solid-js"
import { createStore } from "solid-js/store"
import {
  automationCreate,
  automationDelete,
  automationList,
  automationRun,
  automationToggle,
  automationUpdate,
  handleResp,
  notify,
} from "~/utils"
import { useFetch, useManageTitle } from "~/hooks"
import {
  AutomationOperation,
  AutomationSchedule,
  AutomationTask,
  AutomationTaskRequest,
} from "~/types"

const intervalUnits = [
  { value: "second", label: "秒" },
  { value: "minute", label: "分" },
  { value: "hour", label: "小时" },
  { value: "day", label: "天" },
]

const weekdayOptions = [
  { value: 1, label: "周一" },
  { value: 2, label: "周二" },
  { value: 3, label: "周三" },
  { value: 4, label: "周四" },
  { value: 5, label: "周五" },
  { value: 6, label: "周六" },
  { value: 0, label: "周日" },
]

const createOperationId = () =>
  `op_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`

const createEmptyOperation = (): AutomationOperation => ({
  id: createOperationId(),
  type: "copy",
  source: "",
  destination: "",
})

const toDateTimeInput = (value?: string) => {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ""
  }
  const iso = date.toISOString()
  return iso.slice(0, 16)
}

const toReadableTime = (value?: string) => {
  if (!value) return "未设置"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return "未设置"
  }
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`
}

const describeSchedule = (schedule: AutomationSchedule) => {
  switch (schedule.mode) {
    case "weekly": {
      const weekdays = (schedule.weekdays || [])
        .map((w) => {
          const found = weekdayOptions.find((item) => item.value === w)
          return found ? found.label : `周${w}`
        })
        .join("、")
      return `每周 ${weekdays || "未选择"} 的 ${schedule.time_of_day || "00:00"}`
    }
    case "interval": {
      const unit = intervalUnits.find((item) => item.value === schedule.interval_unit)
      return `每 ${schedule.interval_value || 0}${unit ? unit.label : ""}`
    }
    default:
      return schedule.once_at ? `一次性 ${toReadableTime(schedule.once_at)}` : "一次性"
  }
}

const createDefaultForm = (): AutomationTaskRequest => ({
  id: undefined,
  name: "",
  enabled: true,
  schedule: {
    mode: "once",
    once_at: "",
    interval_value: 1,
    interval_unit: "hour",
    start_at: "",
    weekdays: [],
    time_of_day: "09:00",
  },
  operations: [],
})

const Automation = () => {
  useManageTitle("manage.sidemenu.automation")
  const [tasks, setTasks] = createSignal<AutomationTask[]>([])
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [form, setForm] = createStore<AutomationTaskRequest>(createDefaultForm())
  const [formMode, setFormMode] = createSignal<"create" | "edit">("create")
  const [listLoading, list] = useFetch(automationList)
  const [createLoading, createTask] = useFetch(
    (payload: AutomationTaskRequest) => automationCreate(payload),
  )
  const [updateLoading, updateTask] = useFetch(
    (payload: AutomationTaskRequest) => automationUpdate(payload),
  )
  const [toggleLoading, toggleTask] = useFetch((id: string, enabled: boolean) =>
    automationToggle(id, enabled),
  )
  const [runLoading, runTask] = useFetch((id: string) => automationRun(id))
  const [deleteLoading, deleteTask] = useFetch((id: string) => automationDelete(id))

  const refresh = async () => {
    const resp = await list()
    handleResp(resp, (data) => {
      setTasks(data?.tasks ?? [])
    })
  }

  onMount(() => {
    refresh()
  })

  const resetForm = () => {
    setForm(() => ({ ...createDefaultForm() }))
  }

  const openCreateModal = () => {
    setFormMode("create")
    resetForm()
    onOpen()
  }

  const openEditModal = (task: AutomationTask) => {
    setFormMode("edit")
    setForm(() => ({
      id: task.id,
      name: task.name,
      enabled: task.enabled,
      schedule: {
        mode: task.schedule.mode,
        once_at: toDateTimeInput(task.schedule.once_at),
        interval_value: task.schedule.interval_value || 1,
        interval_unit: task.schedule.interval_unit || "hour",
        start_at: toDateTimeInput(task.schedule.start_at),
        weekdays: task.schedule.weekdays ? [...task.schedule.weekdays] : [],
        time_of_day: task.schedule.time_of_day || "09:00",
      },
      operations: task.operations.map((op) => ({
        ...op,
        id: op.id || createOperationId(),
        cache_full: op.cache_full ?? true,
        put_into_new_dir: op.put_into_new_dir ?? false,
      })),
    }))
    onOpen()
  }

  const submitLoading = createMemo(() =>
    formMode() === "create" ? createLoading() : updateLoading(),
  )

  const closeModal = () => {
    onClose()
  }

  const updateTaskInList = (updated: AutomationTask) => {
    setTasks((prev) => prev.map((item) => (item.id === updated.id ? updated : item)))
  }

  const submitForm = async () => {
    if (!form.name.trim()) {
      notify.warning("请填写任务名称")
      return
    }
    if (!form.operations.length) {
      notify.warning("请至少添加一个操作步骤")
      return
    }
    for (const op of form.operations) {
      if (!op.source.trim()) {
        notify.warning("操作的来源路径不能为空")
        return
      }
      if (op.type !== "delete" && op.type !== "rename" && op.type !== "decompress") {
        if (!op.destination && (op.type === "copy" || op.type === "move")) {
          notify.warning("复制或移动操作需要填写目标路径")
          return
        }
      }
      if (op.type === "rename" && !op.new_name?.trim()) {
        notify.warning("重命名操作需要新的名称")
        return
      }
      if (op.type === "decompress" && !op.destination?.trim()) {
        notify.warning("解压操作需要目标路径")
        return
      }
    }
    if (form.schedule.mode === "interval" && (!form.schedule.interval_value || form.schedule.interval_value <= 0)) {
      notify.warning("请填写正确的间隔数值")
      return
    }
    if (form.schedule.mode === "weekly" && (!form.schedule.weekdays || form.schedule.weekdays.length === 0)) {
      notify.warning("请至少选择一个执行的星期")
      return
    }
    const schedule: AutomationSchedule = {
      mode: form.schedule.mode,
    }
    if (form.schedule.mode === "once") {
      if (form.schedule.once_at) {
        schedule.once_at = new Date(form.schedule.once_at).toISOString()
      }
    } else if (form.schedule.mode === "interval") {
      schedule.interval_value = Number(form.schedule.interval_value || 0)
      schedule.interval_unit = form.schedule.interval_unit || "hour"
      if (form.schedule.start_at) {
        schedule.start_at = new Date(form.schedule.start_at).toISOString()
      }
    } else if (form.schedule.mode === "weekly") {
      schedule.weekdays = [...(form.schedule.weekdays || [])].sort()
      schedule.time_of_day = form.schedule.time_of_day || "00:00"
      if (form.schedule.start_at) {
        schedule.start_at = new Date(form.schedule.start_at).toISOString()
      }
    }
    const operations: AutomationOperation[] = form.operations.map((op) => {
      const next: AutomationOperation = {
        ...op,
        id: op.id || createOperationId(),
        type: op.type,
        source: op.source.trim(),
      }
      if (op.destination) next.destination = op.destination.trim()
      if (op.new_name) next.new_name = op.new_name.trim()
      if (op.password) next.password = op.password
      if (op.inner_path) next.inner_path = op.inner_path
      if (typeof op.cache_full !== "undefined") next.cache_full = op.cache_full
      if (typeof op.put_into_new_dir !== "undefined")
        next.put_into_new_dir = op.put_into_new_dir
      return next
    })
    const payload: AutomationTaskRequest = {
      id: formMode() === "edit" ? form.id : undefined,
      name: form.name,
      enabled: form.enabled,
      schedule,
      operations,
    }
    if (formMode() === "create") {
      const resp = await createTask(payload)
      handleResp(resp, (data) => {
        notify.success("任务创建成功")
        setTasks((prev) => [data, ...prev.filter((item) => item.id !== data.id)])
        closeModal()
      })
    } else {
      const resp = await updateTask(payload)
      handleResp(resp, (data) => {
        notify.success("任务更新成功")
        updateTaskInList(data)
        closeModal()
      })
    }
  }

  const toggleTaskEnabled = async (task: AutomationTask, enabled: boolean) => {
    const resp = await toggleTask(task.id, enabled)
    handleResp(resp, (data) => {
      notify.success(enabled ? "任务已开启" : "任务已关闭")
      updateTaskInList(data)
    })
  }

  const runNow = async (task: AutomationTask) => {
    const resp = await runTask(task.id)
    handleResp(resp, (data) => {
      notify.success("已触发立即执行")
      updateTaskInList(data)
    })
  }

  const removeTask = async (task: AutomationTask) => {
    const resp = await deleteTask(task.id)
    handleResp(resp, () => {
      notify.success("任务已删除")
      setTasks((prev) => prev.filter((item) => item.id !== task.id))
    })
  }

  const setOperationField = (
    index: number,
    key: keyof AutomationOperation,
    value: any,
  ) => {
    setForm("operations", (ops) => {
      const next = ops.slice()
      next[index] = { ...next[index], [key]: value }
      return next
    })
  }

  const moveOperation = (index: number, direction: -1 | 1) => {
    setForm("operations", (ops) => {
      const next = ops.slice()
      const target = index + direction
      if (target < 0 || target >= next.length) return next
      const temp = next[index]
      next[index] = next[target]
      next[target] = temp
      return next
    })
  }

  const removeOperation = (index: number) => {
    setForm("operations", (ops) => ops.filter((_, i) => i !== index))
  }

  return (
    <VStack alignItems="start" spacing="$4" w="$full">
      <HStack spacing="$2" wrap="wrap">
        <Button colorScheme="accent" loading={listLoading()} onClick={refresh}>
          刷新任务列表
        </Button>
        <Button colorScheme="primary" onClick={openCreateModal}>
          新建任务
        </Button>
      </HStack>
      <VStack spacing="$3" w="$full">
        <For each={tasks()} fallback={<Text>暂时没有任务</Text>}>
          {(task) => (
            <VStack
              w="$full"
              alignItems="start"
              spacing="$2"
              p="$4"
              border="1px solid var(--hope-colors-neutral6)"
              rounded="$lg"
            >
              <HStack w="$full" alignItems="center" spacing="$2">
                <Text fontSize="$xl" fontWeight="$bold">
                  {task.name}
                </Text>
                <Badge colorScheme={task.enabled ? "success" : "danger"}>
                  {task.enabled ? "已启用" : "已停用"}
                </Badge>
                <Spacer />
                <HopeSwitch
                  checked={task.enabled}
                  loading={toggleLoading()}
                  onChange={(e) => toggleTaskEnabled(task, e.currentTarget.checked)}
                >
                  启用
                </HopeSwitch>
              </HStack>
              <Text color="$neutral11">计划：{describeSchedule(task.schedule)}</Text>
              <Show when={task.next_run}>
                <Text color="$neutral10">下次执行：{toReadableTime(task.next_run)}</Text>
              </Show>
              <Show when={task.last_run}>
                <Text color="$neutral10">上次执行：{toReadableTime(task.last_run)}</Text>
              </Show>
              <Show when={task.last_result}>
                <Text color="$danger10">最近结果：{task.last_result}</Text>
              </Show>
              <details style="width: 100%">
                <summary>执行步骤</summary>
                <VStack spacing="$2" alignItems="start" w="$full" mt="$2">
                  <For each={task.operations}>
                    {(op, index) => (
                      <Box w="$full" p="$2" bgColor="$neutral2" rounded="$md">
                        <Text fontWeight="$semibold">
                          步骤 {index() + 1}：{op.type}
                        </Text>
                        <VStack alignItems="start" spacing="$1">
                          <Text>来源：{op.source}</Text>
                          <Show when={op.destination}>
                            <Text>目标：{op.destination}</Text>
                          </Show>
                          <Show when={op.new_name}>
                            <Text>新名称：{op.new_name}</Text>
                          </Show>
                          <Show when={op.inner_path}>
                            <Text>内路径：{op.inner_path}</Text>
                          </Show>
                        </VStack>
                      </Box>
                    )}
                  </For>
                </VStack>
              </details>
              <details style="width: 100%">
                <summary>历史记录</summary>
                <VStack spacing="$2" alignItems="start" w="$full" mt="$2">
                  <For each={task.history} fallback={<Text>暂无历史</Text>}>
                    {(history) => (
                      <Box w="$full" p="$2" bgColor="$neutral2" rounded="$md">
                        <HStack spacing="$2" alignItems="center">
                          <Badge colorScheme={history.success ? "success" : "danger"}>
                            {history.success ? "成功" : "失败"}
                          </Badge>
                          <Text>
                            {toReadableTime(history.started_at)} ~ {toReadableTime(history.finished_at)}
                          </Text>
                        </HStack>
                        <Show when={history.message}>
                          <Text color="$danger10">{history.message}</Text>
                        </Show>
                        <Divider />
                        <VStack spacing="$1" alignItems="start" mt="$1">
                          <For each={history.results}>
                            {(result) => (
                              <Box>
                                <Text fontWeight="$medium">操作 {result.operation.type}</Text>
                                <Text>目标：{result.targets.join("，")}</Text>
                                <Show when={!result.success}>
                                  <Text color="$danger10">错误：{result.error}</Text>
                                </Show>
                              </Box>
                            )}
                          </For>
                        </VStack>
                      </Box>
                    )}
                  </For>
                </VStack>
              </details>
              <HStack spacing="$2">
                <Button
                  size="sm"
                  colorScheme="primary"
                  loading={runLoading()}
                  onClick={() => runNow(task)}
                >
                  立即执行
                </Button>
                <Button size="sm" onClick={() => openEditModal(task)}>
                  编辑
                </Button>
                <Button
                  size="sm"
                  colorScheme="danger"
                  loading={deleteLoading()}
                  onClick={() => removeTask(task)}
                >
                  删除
                </Button>
              </HStack>
            </VStack>
          )}
        </For>
      </VStack>
      <Modal opened={isOpen()} onClose={closeModal} size="xl" scrollBehavior="inside">
        <ModalOverlay />
        <ModalContent>
          <ModalCloseButton />
          <ModalHeader>{formMode() === "create" ? "新建自动化任务" : "编辑自动化任务"}</ModalHeader>
          <ModalBody>
            <VStack spacing="$3" alignItems="start" w="$full">
              <FormControl required w="$full">
                <FormLabel>任务名称</FormLabel>
                <Input
                  value={form.name}
                  onInput={(e) => setForm("name", e.currentTarget.value)}
                  placeholder="请输入任务名称"
                />
              </FormControl>
              <FormControl display="flex" alignItems="center" gap="$2">
                <HopeSwitch
                  checked={form.enabled}
                  onChange={(e) => setForm("enabled", e.currentTarget.checked)}
                >
                  立即启用
                </HopeSwitch>
              </FormControl>
              <Divider />
              <VStack spacing="$2" alignItems="start" w="$full">
                <Text fontWeight="$bold">执行计划</Text>
                <FormControl w="$full">
                  <FormLabel>计划类型</FormLabel>
                  <select
                    value={form.schedule.mode}
                    onChange={(e) =>
                      setForm("schedule", (prev) => ({
                        ...prev,
                        mode: e.currentTarget.value as AutomationSchedule["mode"],
                      }))
                    }
                    style="width: 100%; padding: 8px; border-radius: 8px; border: 1px solid var(--hope-colors-neutral6);"
                  >
                    <option value="once">仅执行一次</option>
                    <option value="interval">按固定间隔</option>
                    <option value="weekly">每周固定时间</option>
                  </select>
                </FormControl>
                <Show when={form.schedule.mode === "once"}>
                  <FormControl w="$full">
                    <FormLabel>执行时间</FormLabel>
                    <Input
                      type="datetime-local"
                      value={form.schedule.once_at || ""}
                      onInput={(e) =>
                        setForm("schedule", (prev) => ({
                          ...prev,
                          once_at: e.currentTarget.value,
                        }))
                      }
                    />
                  </FormControl>
                </Show>
                <Show when={form.schedule.mode === "interval"}>
                  <HStack w="$full" spacing="$2">
                    <FormControl>
                      <FormLabel>间隔数值</FormLabel>
                      <Input
                        type="number"
                        min="1"
                        value={form.schedule.interval_value}
                        onInput={(e) =>
                          setForm("schedule", (prev) => ({
                            ...prev,
                            interval_value: Number(e.currentTarget.value),
                          }))
                        }
                      />
                    </FormControl>
                    <FormControl>
                      <FormLabel>间隔单位</FormLabel>
                      <select
                        value={form.schedule.interval_unit || "hour"}
                        onChange={(e) =>
                          setForm("schedule", (prev) => ({
                            ...prev,
                            interval_unit: e.currentTarget.value,
                          }))
                        }
                        style="width: 100%; padding: 8px; border-radius: 8px; border: 1px solid var(--hope-colors-neutral6);"
                      >
                        <For each={intervalUnits}>
                          {(item) => <option value={item.value}>{item.label}</option>}
                        </For>
                      </select>
                    </FormControl>
                  </HStack>
                  <FormControl w="$full">
                    <FormLabel>起始时间（可选）</FormLabel>
                    <Input
                      type="datetime-local"
                      value={form.schedule.start_at || ""}
                      onInput={(e) =>
                        setForm("schedule", (prev) => ({
                          ...prev,
                          start_at: e.currentTarget.value,
                        }))
                      }
                    />
                  </FormControl>
                </Show>
                <Show when={form.schedule.mode === "weekly"}>
                  <FormControl w="$full">
                    <FormLabel>选择星期</FormLabel>
                    <HStack wrap="wrap" spacing="$2">
                      <For each={weekdayOptions}>
                        {(item) => (
                          <Checkbox
                            checked={form.schedule.weekdays?.includes(item.value) || false}
                            onChange={(e) =>
                              setForm("schedule", (prev) => {
                                const list = new Set(prev.weekdays || [])
                                if (e.currentTarget.checked) {
                                  list.add(item.value)
                                } else {
                                  list.delete(item.value)
                                }
                                return {
                                  ...prev,
                                  weekdays: Array.from(list),
                                }
                              })
                            }
                          >
                            {item.label}
                          </Checkbox>
                        )}
                      </For>
                    </HStack>
                  </FormControl>
                  <FormControl w="$full">
                    <FormLabel>执行时间</FormLabel>
                    <Input
                      type="time"
                      value={form.schedule.time_of_day || "00:00"}
                      onInput={(e) =>
                        setForm("schedule", (prev) => ({
                          ...prev,
                          time_of_day: e.currentTarget.value,
                        }))
                      }
                    />
                  </FormControl>
                  <FormControl w="$full">
                    <FormLabel>起始日期（可选）</FormLabel>
                    <Input
                      type="datetime-local"
                      value={form.schedule.start_at || ""}
                      onInput={(e) =>
                        setForm("schedule", (prev) => ({
                          ...prev,
                          start_at: e.currentTarget.value,
                        }))
                      }
                    />
                  </FormControl>
                </Show>
              </VStack>
              <Divider />
              <VStack spacing="$2" alignItems="start" w="$full">
                <HStack w="$full" justifyContent="space-between">
                  <Text fontWeight="$bold">操作步骤</Text>
                  <Button
                    size="sm"
                    colorScheme="primary"
                    onClick={() =>
                      setForm("operations", (ops) => [...ops, createEmptyOperation()])
                    }
                  >
                    添加步骤
                  </Button>
                </HStack>
                <For each={form.operations} fallback={<Text>尚未添加任何操作</Text>}>
                  {(operation, index) => (
                    <VStack
                      w="$full"
                      alignItems="start"
                      spacing="$2"
                      p="$3"
                      border="1px solid var(--hope-colors-neutral6)"
                      rounded="$lg"
                    >
                      <HStack w="$full" spacing="$2">
                        <Text fontWeight="$semibold">步骤 {index() + 1}</Text>
                        <Spacer />
                        <Button
                          size="xs"
                          variant="subtle"
                          onClick={() => moveOperation(index(), -1)}
                        >
                          上移
                        </Button>
                        <Button
                          size="xs"
                          variant="subtle"
                          onClick={() => moveOperation(index(), 1)}
                        >
                          下移
                        </Button>
                        <Button
                          size="xs"
                          colorScheme="danger"
                          onClick={() => removeOperation(index())}
                        >
                          删除
                        </Button>
                      </HStack>
                      <FormControl w="$full">
                        <FormLabel>操作类型</FormLabel>
                        <select
                          value={operation.type}
                          onChange={(e) =>
                            setOperationField(index(), "type", e.currentTarget.value)
                          }
                          style="width: 100%; padding: 8px; border-radius: 8px; border: 1px solid var(--hope-colors-neutral6);"
                        >
                          <option value="copy">复制</option>
                          <option value="move">移动</option>
                          <option value="delete">删除</option>
                          <option value="rename">重命名</option>
                          <option value="decompress">解压</option>
                        </select>
                      </FormControl>
                      <FormControl w="$full">
                        <FormLabel>来源路径</FormLabel>
                        <Input
                          value={operation.source}
                          placeholder="例如 /root/*.zip"
                          onInput={(e) =>
                            setOperationField(index(), "source", e.currentTarget.value)
                          }
                        />
                      </FormControl>
                      <Show when={operation.type === "copy" || operation.type === "move" || operation.type === "decompress"}>
                        <FormControl w="$full">
                          <FormLabel>目标路径</FormLabel>
                          <Input
                            value={operation.destination || ""}
                            placeholder="例如 /backup"
                            onInput={(e) =>
                              setOperationField(index(), "destination", e.currentTarget.value)
                            }
                          />
                        </FormControl>
                      </Show>
                      <Show when={operation.type === "rename"}>
                        <FormControl w="$full">
                          <FormLabel>新名称</FormLabel>
                          <Input
                            value={operation.new_name || ""}
                            onInput={(e) =>
                              setOperationField(index(), "new_name", e.currentTarget.value)
                            }
                          />
                        </FormControl>
                      </Show>
                      <Show when={operation.type === "decompress"}>
                        <FormControl w="$full">
                          <FormLabel>压缩包密码（可选）</FormLabel>
                          <Input
                            value={operation.password || ""}
                            onInput={(e) =>
                              setOperationField(index(), "password", e.currentTarget.value)
                            }
                          />
                        </FormControl>
                        <FormControl w="$full">
                          <FormLabel>内层路径（可选）</FormLabel>
                          <Input
                            value={operation.inner_path || ""}
                            placeholder="例如 /"
                            onInput={(e) =>
                              setOperationField(index(), "inner_path", e.currentTarget.value)
                            }
                          />
                        </FormControl>
                        <Checkbox
                          checked={operation.cache_full ?? true}
                          onChange={(e) =>
                            setOperationField(index(), "cache_full", e.currentTarget.checked)
                          }
                        >
                          全量缓存解压
                        </Checkbox>
                        <Checkbox
                          checked={operation.put_into_new_dir ?? false}
                          onChange={(e) =>
                            setOperationField(
                              index(),
                              "put_into_new_dir",
                              e.currentTarget.checked,
                            )
                          }
                        >
                          解压到新文件夹
                        </Checkbox>
                      </Show>
                    </VStack>
                  )}
                </For>
              </VStack>
            </VStack>
          </ModalBody>
          <ModalFooter>
            <Button variant="subtle" mr="$2" onClick={closeModal}>
              取消
            </Button>
            <Button
              colorScheme="primary"
              loading={submitLoading()}
              onClick={submitForm}
            >
              保存
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </VStack>
  )
}

export default Automation
