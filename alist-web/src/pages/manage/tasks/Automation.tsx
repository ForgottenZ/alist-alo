import {
  Box,
  Button,
  Checkbox,
  Divider,
  FormControl,
  FormLabel,
  HStack,
  Input,
  Select,
  SimpleGrid,
  Stack,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  VStack,
  createDisclosure,
} from "@hope-ui/solid"
import { For, Show, createEffect, createSignal } from "solid-js"
import { createStore } from "solid-js/store"
import {
  automationCreate,
  automationDelete,
  automationDetail,
  automationList,
  automationRun,
  automationToggle,
  automationUpdate,
} from "~/utils"
import { handleResp, notify } from "~/utils"
import { useFetch, useManageTitle } from "~/hooks"
import { AutomationHistory, AutomationOperation, AutomationTask } from "~/types"
const pad = (value: number) => value.toString().padStart(2, "0")

const formatDateTime = (value?: string | null, withSeconds = false) => {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  const datePart = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
    date.getDate(),
  )}`
  const timePart = `${pad(date.getHours())}:${pad(date.getMinutes())}`
  const seconds = withSeconds ? `:${pad(date.getSeconds())}` : ""
  return `${datePart} ${timePart}${seconds}`
}

const defaultOperation = (): AutomationOperation => ({
  type: "copy",
  source: "",
  destination: "",
  new_name: "",
  inner_path: "/",
  password: "",
  cache_full: true,
  put_into_new_dir: false,
})

const defaultForm = () => ({
  id: 0,
  name: "",
  description: "",
  enabled: true,
  schedule_type: "once",
  interval_value: 1,
  interval_unit: "hour",
  time_of_day: "08:00",
  weekdays: [] as number[],
  once_at: "",
  operations: [] as AutomationOperation[],
})

const Automation = () => {
  useManageTitle("manage.sidemenu.automation")
  const [tasks, setTasks] = createSignal<AutomationTask[]>([])
  const [histories, setHistories] = createSignal<AutomationHistory[]>([])
  const [loading, loadTasks] = useFetch(automationList)
  const [saving, saveTask] = useFetch((payload: any) =>
    payload.id ? automationUpdate(payload) : automationCreate(payload),
  )
  const [toggleLoading, toggleReq] = useFetch(automationToggle)
  const [runLoading, runReq] = useFetch(automationRun)
  const [delLoading, deleteReq] = useFetch(automationDelete)
  const [, getDetail] = useFetch(automationDetail)
  const { isOpen: formOpen, onOpen: openForm, onClose: closeForm } =
    createDisclosure()
  const {
    isOpen: historyOpen,
    onOpen: openHistory,
    onClose: closeHistory,
  } = createDisclosure()
  const [form, setForm] = createStore(defaultForm())
  const [opDraft, setOpDraft] = createStore(defaultOperation())

  const refresh = async () => {
    const resp = await loadTasks()
    handleResp(resp, (data) => {
      setTasks(data as AutomationTask[])
    })
  }
  createEffect(refresh)

  const resetForm = () => {
    setForm(defaultForm())
    setOpDraft(defaultOperation())
    setHistories([])
  }

  const openCreate = () => {
    resetForm()
    openForm()
  }

  const fillForm = (task: AutomationTask) => {
    setForm({
      id: task.id,
      name: task.name,
      description: task.description ?? "",
      enabled: task.enabled,
      schedule_type: task.schedule_type,
      interval_value: task.interval_value ?? 1,
      interval_unit: task.interval_unit ?? "hour",
      time_of_day: task.time_of_day ?? "08:00",
      weekdays: task.weekdays ?? [],
      once_at: task.once_at ? task.once_at.slice(0, 16) : "",
      operations: task.operations ?? [],
    })
  }

  const submitForm = async () => {
    if (!form.name.trim()) {
      notify.warning("请输入任务名称")
      return
    }
    if (form.operations.length === 0) {
      notify.warning("请至少添加一个步骤")
      return
    }
    const payload = {
      id: form.id,
      name: form.name,
      description: form.description,
      enabled: form.enabled,
      schedule_type: form.schedule_type,
      interval_value: form.interval_value,
      interval_unit: form.interval_unit,
      time_of_day: form.time_of_day,
      weekdays: form.weekdays,
      once_at:
        form.schedule_type === "once" && form.once_at
          ? new Date(form.once_at).toISOString()
          : "",
      operations: form.operations,
    }
    const resp = await saveTask(payload)
    handleResp(resp, () => {
      notify.success("保存成功")
      closeForm()
      refresh()
    })
  }

  const editTask = async (task: AutomationTask) => {
    resetForm()
    const resp = await getDetail(task.id)
    handleResp(resp, (data) => {
      const detail = data as { task: AutomationTask; histories: AutomationHistory[] }
      fillForm(detail.task)
      setHistories(detail.histories ?? [])
      openForm()
    })
  }

  const addOperation = () => {
    if (!opDraft.source.trim() && opDraft.type !== "delete") {
      notify.warning("请填写源路径")
      return
    }
    if (opDraft.type === "rename" && !opDraft.new_name?.trim()) {
      notify.warning("请填写新的名称")
      return
    }
    if (["copy", "move", "decompress"].includes(opDraft.type) && !opDraft.destination?.trim()) {
      notify.warning("请填写目标路径")
      return
    }
    const next = { ...opDraft }
    setForm("operations", (ops) => [...ops, next])
    setOpDraft(defaultOperation())
  }

  const removeOperation = (idx: number) => {
    setForm("operations", (ops) => ops.filter((_, i) => i !== idx))
  }

  const moveOperation = (idx: number, offset: number) => {
    setForm("operations", (ops) => {
      const next = [...ops]
      const target = idx + offset
      if (target < 0 || target >= next.length) return ops
      const temp = next[idx]
      next[idx] = next[target]
      next[target] = temp
      return next
    })
  }

  const toggleTask = async (task: AutomationTask) => {
    const resp = await toggleReq(task.id, !task.enabled)
    handleResp(resp, () => {
      notify.success("状态已更新")
      refresh()
    })
  }

  const runTask = async (task: AutomationTask) => {
    const resp = await runReq(task.id)
    handleResp(resp, () => notify.success("任务已提交执行"))
  }

  const deleteTask = async (task: AutomationTask) => {
    const resp = await deleteReq(task.id)
    handleResp(resp, () => {
      notify.success("任务已删除")
      refresh()
    })
  }

  const showHistory = async (task: AutomationTask) => {
    const resp = await getDetail(task.id)
    handleResp(resp, (data) => {
      const detail = data as { histories: AutomationHistory[]; task: AutomationTask }
      setHistories(detail.histories ?? [])
      openHistory()
    })
  }

  const renderOperation = (op: AutomationOperation, idx: number) => {
    return (
      <Box
        border="1px solid var(--hope-colors-neutral6)"
        rounded="$md"
        p="$3"
        w="$full"
      >
        <HStack justifyContent="space-between" mb="$2">
          <Text fontWeight="bold">步骤 {idx + 1}: {renderOpLabel(op.type)}</Text>
          <HStack spacing="$2">
            <Button size="xs" onClick={() => moveOperation(idx, -1)}>
              上移
            </Button>
            <Button size="xs" onClick={() => moveOperation(idx, 1)}>
              下移
            </Button>
            <Button size="xs" colorScheme="danger" onClick={() => removeOperation(idx)}>
              删除
            </Button>
          </HStack>
        </HStack>
        <OperationSummary op={op} />
      </Box>
    )
  }

  return (
    <VStack align="stretch" spacing="$4">
      <HStack justifyContent="space-between">
        <Text fontSize="$2xl" fontWeight="bold">
          自动化任务
        </Text>
        <Button onClick={openCreate}>新建任务</Button>
      </HStack>
      <Table>
        <Thead>
          <Tr>
            <Th>名称</Th>
            <Th>计划</Th>
            <Th>下次执行</Th>
            <Th>状态</Th>
            <Th>操作</Th>
          </Tr>
        </Thead>
        <Tbody>
          <For each={tasks()}>
            {(task) => (
              <Tr>
                <Td>
                  <Text fontWeight="semibold">{task.name}</Text>
                  <Show when={task.description}>
                    <Text color="$neutral9" fontSize="sm">
                      {task.description}
                    </Text>
                  </Show>
                </Td>
                <Td>{renderSchedule(task)}</Td>
                <Td>{formatDateTime(task.next_run) || "-"}</Td>
                <Td>
                  <Text color={task.last_status === "error" ? "$danger9" : "$success9"}>
                    {task.enabled ? "启用" : "停用"}
                  </Text>
                  <Show when={task.last_error}>
                    <Text fontSize="sm" color="$danger9">
                      {task.last_error}
                    </Text>
                  </Show>
                </Td>
                <Td>
                  <HStack spacing="$2">
                    <Button size="sm" onClick={() => editTask(task)}>
                      编辑
                    </Button>
                    <Button size="sm" onClick={() => toggleTask(task)} loading={toggleLoading()}>
                      {task.enabled ? "停用" : "启用"}
                    </Button>
                    <Button size="sm" onClick={() => runTask(task)} loading={runLoading()}>
                      立即执行
                    </Button>
                    <Button size="sm" onClick={() => showHistory(task)}>
                      历史
                    </Button>
                    <Button
                      size="sm"
                      colorScheme="danger"
                      onClick={() => deleteTask(task)}
                      loading={delLoading()}
                    >
                      删除
                    </Button>
                  </HStack>
                </Td>
              </Tr>
            )}
          </For>
        </Tbody>
      </Table>

      <Show when={formOpen()}>
        <Box border="1px solid var(--hope-colors-neutral6)" rounded="$lg" p="$4">
          <VStack align="stretch" spacing="$3">
            <Text fontSize="$xl" fontWeight="bold">
              {form.id ? "编辑任务" : "新建任务"}
            </Text>
            <SimpleGrid columns={{ base: 1, md: 2 }} spacing="$3">
              <FormControl>
                <FormLabel>任务名称</FormLabel>
                <Input value={form.name} onInput={(e) => setForm("name", e.currentTarget.value)} />
              </FormControl>
              <FormControl>
                <FormLabel>启用</FormLabel>
                <Checkbox
                  checked={form.enabled}
                  onChange={(e) => setForm("enabled", e.currentTarget.checked)}
                >
                  当前任务启用
                </Checkbox>
              </FormControl>
            </SimpleGrid>
            <FormControl>
              <FormLabel>描述</FormLabel>
              <Input
                value={form.description}
                onInput={(e) => setForm("description", e.currentTarget.value)}
              />
            </FormControl>
            <SimpleGrid columns={{ base: 1, md: 2 }} spacing="$3">
              <FormControl>
                <FormLabel>计划类型</FormLabel>
                <Select
                  value={form.schedule_type}
                  onChange={(v) => setForm("schedule_type", v as string)}
                >
                  <option value="once">指定时间执行一次</option>
                  <option value="interval">按间隔循环</option>
                  <option value="weekly">每周定时</option>
                </Select>
              </FormControl>
              <Show when={form.schedule_type === "interval"}>
                <HStack spacing="$2">
                  <FormControl>
                    <FormLabel>间隔数值</FormLabel>
                    <Input
                      type="number"
                      value={form.interval_value}
                      onInput={(e) =>
                        setForm("interval_value", Number(e.currentTarget.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormControl>
                    <FormLabel>间隔单位</FormLabel>
                    <Select
                      value={form.interval_unit}
                      onChange={(v) => setForm("interval_unit", v as string)}
                    >
                      <option value="second">秒</option>
                      <option value="minute">分钟</option>
                      <option value="hour">小时</option>
                      <option value="day">天</option>
                    </Select>
                  </FormControl>
                </HStack>
              </Show>
              <Show when={form.schedule_type === "weekly"}>
                <FormControl>
                  <FormLabel>执行时间</FormLabel>
                  <Input
                    type="time"
                    value={form.time_of_day}
                    onInput={(e) => setForm("time_of_day", e.currentTarget.value)}
                  />
                </FormControl>
              </Show>
            </SimpleGrid>
            <Show when={form.schedule_type === "weekly"}>
              <FormControl>
                <FormLabel>执行星期</FormLabel>
                <HStack spacing="$2" wrap="wrap">
                  <For each={[
                    { value: 1, label: "周一" },
                    { value: 2, label: "周二" },
                    { value: 3, label: "周三" },
                    { value: 4, label: "周四" },
                    { value: 5, label: "周五" },
                    { value: 6, label: "周六" },
                    { value: 0, label: "周日" },
                  ]}>
                    {(item) => (
                      <Checkbox
                        checked={form.weekdays.includes(item.value)}
                        onChange={(e) => {
                          const checked = e.currentTarget.checked
                          setForm("weekdays", (prev) => {
                            if (checked) {
                              if (prev.includes(item.value)) return prev
                              return [...prev, item.value]
                            }
                            return prev.filter((v) => v !== item.value)
                          })
                        }}
                      >
                        {item.label}
                      </Checkbox>
                    )}
                  </For>
                </HStack>
              </FormControl>
            </Show>
            <Show when={form.schedule_type === "once"}>
              <FormControl>
                <FormLabel>执行时间</FormLabel>
                <Input
                  type="datetime-local"
                  value={form.once_at}
                  onInput={(e) => setForm("once_at", e.currentTarget.value)}
                />
              </FormControl>
            </Show>
            <Divider />
            <Text fontWeight="bold">任务步骤</Text>
            <VStack align="stretch" spacing="$3">
              <For each={form.operations}>{renderOperation}</For>
              <Box border="1px dashed var(--hope-colors-neutral6)" rounded="$md" p="$3">
                <Text mb="$2" fontWeight="semibold">
                  添加步骤
                </Text>
                <SimpleGrid columns={{ base: 1, md: 2 }} spacing="$2">
                  <FormControl>
                    <FormLabel>类型</FormLabel>
                    <Select
                      value={opDraft.type}
                      onChange={(v) => setOpDraft("type", v as string)}
                    >
                      <option value="copy">复制</option>
                      <option value="move">移动</option>
                      <option value="delete">删除</option>
                      <option value="rename">重命名</option>
                      <option value="decompress">解压</option>
                    </Select>
                  </FormControl>
                  <Show when={["copy", "move", "decompress"].includes(opDraft.type)}>
                    <FormControl>
                      <FormLabel>目标路径</FormLabel>
                      <Input
                        value={opDraft.destination ?? ""}
                        onInput={(e) => setOpDraft("destination", e.currentTarget.value)}
                      />
                    </FormControl>
                  </Show>
                </SimpleGrid>
                <Show when={opDraft.type !== "rename" && opDraft.type !== "delete"}>
                  <FormControl mt="$2">
                    <FormLabel>源路径</FormLabel>
                    <Input
                      value={opDraft.source}
                      onInput={(e) => setOpDraft("source", e.currentTarget.value)}
                    />
                  </FormControl>
                </Show>
                <Show when={opDraft.type === "delete"}>
                  <FormControl mt="$2">
                    <FormLabel>删除路径</FormLabel>
                    <Input
                      value={opDraft.source}
                      onInput={(e) => setOpDraft("source", e.currentTarget.value)}
                    />
                  </FormControl>
                </Show>
                <Show when={opDraft.type === "rename"}>
                  <HStack spacing="$2" mt="$2">
                    <FormControl>
                      <FormLabel>源路径</FormLabel>
                      <Input
                        value={opDraft.source}
                        onInput={(e) => setOpDraft("source", e.currentTarget.value)}
                      />
                    </FormControl>
                    <FormControl>
                      <FormLabel>新名称</FormLabel>
                      <Input
                        value={opDraft.new_name ?? ""}
                        onInput={(e) => setOpDraft("new_name", e.currentTarget.value)}
                      />
                    </FormControl>
                  </HStack>
                </Show>
                <Show when={opDraft.type === "decompress"}>
                  <SimpleGrid columns={{ base: 1, md: 2 }} spacing="$2" mt="$2">
                    <FormControl>
                      <FormLabel>内部路径</FormLabel>
                      <Input
                        value={opDraft.inner_path ?? "/"}
                        onInput={(e) => setOpDraft("inner_path", e.currentTarget.value)}
                      />
                    </FormControl>
                    <FormControl>
                      <FormLabel>解压密码</FormLabel>
                      <Input
                        value={opDraft.password ?? ""}
                        onInput={(e) => setOpDraft("password", e.currentTarget.value)}
                      />
                    </FormControl>
                  </SimpleGrid>
                  <Checkbox
                    checked={opDraft.cache_full ?? true}
                    onChange={(e) => setOpDraft("cache_full", e.currentTarget.checked)}
                  >
                    完全缓存后再解压
                  </Checkbox>
                  <Checkbox
                    checked={opDraft.put_into_new_dir ?? false}
                    onChange={(e) => setOpDraft("put_into_new_dir", e.currentTarget.checked)}
                  >
                    输出到新目录
                  </Checkbox>
                </Show>
                <Button mt="$3" onClick={addOperation}>
                  添加步骤
                </Button>
              </Box>
            </VStack>
            <HStack spacing="$3" justifyContent="flex-end" pt="$2">
              <Button onClick={closeForm} variant="ghost">
                取消
              </Button>
              <Button onClick={submitForm} loading={saving()}>
                保存
              </Button>
            </HStack>
          </VStack>
        </Box>
      </Show>

      <Show when={historyOpen()}>
        <Box border="1px solid var(--hope-colors-neutral6)" rounded="$lg" p="$4">
          <HStack justifyContent="space-between" mb="$2">
            <Text fontSize="$xl" fontWeight="bold">
              执行历史
            </Text>
            <Button size="sm" onClick={closeHistory}>
              关闭
            </Button>
          </HStack>
          <VStack align="stretch" spacing="$2">
            <For each={histories()}>
              {(item) => (
                <Box border="1px solid var(--hope-colors-neutral5)" rounded="$md" p="$3">
                  <Text fontWeight="semibold">
                    {formatDateTime(item.executed_at, true) || "-"}（
                    {renderHistoryStatus(item.status)}）
                  </Text>
                  <Show when={item.message}>
                    <Text color="$danger9">{item.message}</Text>
                  </Show>
                  <Text fontSize="sm">耗时：{item.duration} ms</Text>
                </Box>
              )}
            </For>
            <Show when={histories().length === 0}>
              <Text color="$neutral9">暂无历史记录</Text>
            </Show>
          </VStack>
        </Box>
      </Show>
    </VStack>
  )
}

const renderOpLabel = (type: string) => {
  switch (type) {
    case "copy":
      return "复制"
    case "move":
      return "移动"
    case "delete":
      return "删除"
    case "rename":
      return "重命名"
    case "decompress":
      return "解压"
    default:
      return type
  }
}

const OperationSummary = (props: { op: AutomationOperation }) => {
  const { op } = props
  return (
    <VStack align="stretch" spacing="$1">
      <Show when={op.source}>
        <Text>源：{op.source}</Text>
      </Show>
      <Show when={op.destination}>
        <Text>目标：{op.destination}</Text>
      </Show>
      <Show when={op.new_name}>
        <Text>新名称：{op.new_name}</Text>
      </Show>
      <Show when={op.inner_path}>
        <Text>内部路径：{op.inner_path}</Text>
      </Show>
      <Show when={op.password}>
        <Text>密码：{op.password}</Text>
      </Show>
      <Show when={op.type === "decompress"}>
        <Text>完全缓存：{op.cache_full === false ? "否" : "是"}</Text>
        <Text>输出到新目录：{op.put_into_new_dir ? "是" : "否"}</Text>
      </Show>
    </VStack>
  )
}

const renderSchedule = (task: AutomationTask) => {
  switch (task.schedule_type) {
    case "once":
      return task.once_at
        ? `在 ${formatDateTime(task.once_at)} 执行`
        : "等待时间"
    case "interval":
      return `每 ${task.interval_value}${renderUnit(task.interval_unit)} 执行一次`
    case "weekly":
      return `每周 ${task.weekdays
        .map((d) =>
          ["周日", "周一", "周二", "周三", "周四", "周五", "周六"][d] ?? d.toString(),
        )
        .join("、")} 的 ${task.time_of_day}`
    default:
      return task.schedule_type
  }
}

const renderHistoryStatus = (status: string) => {
  switch (status) {
    case "success":
      return "成功"
    case "error":
      return "失败"
    case "skipped":
      return "跳过"
    default:
      return status
  }
}

const renderUnit = (unit: string) => {
  switch (unit) {
    case "second":
      return "秒"
    case "minute":
      return "分钟"
    case "hour":
      return "小时"
    case "day":
      return "天"
    default:
      return unit
  }
}

export default Automation
