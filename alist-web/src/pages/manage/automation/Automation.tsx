import {
  Box,
  Button,
  Checkbox,
  CheckboxGroup,
  createDisclosure,
  HStack,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Select,
  SelectContent,
  SelectIcon,
  SelectListbox,
  SelectOption,
  SelectOptionText,
  SelectPlaceholder,
  SelectTrigger,
  SelectValue,
  Switch as HopeSwitch,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  VStack,
  Text,
  Divider,
  IconButton,
} from "@hope-ui/solid"
import { createSignal, For, Show, onMount } from "solid-js"
import { BsPlus, BsX } from "solid-icons/bs"
import { FaSolidArrowDown, FaSolidArrowUp } from "solid-icons/fa"
import { useFetch, useManageTitle } from "~/hooks"
import {
  automationCreate,
  automationDelete,
  automationHistory,
  automationList,
  automationRun,
  automationToggle,
  automationUpdate,
  formatDate,
  handleResp,
  handleRespWithNotifySuccess,
  notify,
} from "~/utils"
import {
  AutomationHistoryItem,
  AutomationStep,
  AutomationTask,
  AutomationTaskPayload,
} from "~/types/automation"

const weekdayLabels = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]

const defaultStep = (): AutomationStep => ({
  sort_order: 1,
  action: "copy",
  source: "",
  target: "",
  options: {},
})

const buildDefaultTask = (): AutomationTaskPayload => ({
  name: "",
  enabled: true,
  schedule_type: "interval",
  interval_value: 1,
  interval_unit: "hour",
  weekdays: [1],
  time_of_day: "08:00",
  specific_time: "",
  steps: [defaultStep()],
})

const toPayload = (task: AutomationTask): AutomationTaskPayload => ({
  id: task.id,
  name: task.name,
  enabled: task.enabled,
  schedule_type: task.schedule_type,
  interval_value: task.interval_value,
  interval_unit: task.interval_unit,
  weekdays: task.weekdays || [],
  time_of_day: task.time_of_day || "",
  specific_time: task.specific_time || "",
  steps: task.steps.map((step, index) => ({
    id: step.id,
    sort_order: step.sort_order || index + 1,
    action: step.action,
    source: step.source,
    target: step.target,
    options: step.options || {},
  })),
})

const describeSchedule = (task: AutomationTask) => {
  switch (task.schedule_type) {
    case "interval": {
      const unitMap: Record<string, string> = {
        second: "秒",
        seconds: "秒",
        minute: "分钟",
        minutes: "分钟",
        hour: "小时",
        hours: "小时",
        day: "天",
        days: "天",
      }
      const unit = unitMap[task.interval_unit] || task.interval_unit
      return `每${task.interval_value}${unit}`
    }
    case "weekly": {
      const days = (task.weekdays || [])
        .map((d) => weekdayLabels[d] ?? d)
        .join("、")
      return `每周${days} ${task.time_of_day || ""}`
    }
    case "once":
      return `指定时间 ${task.specific_time}`
    default:
      return task.schedule_type
  }
}

const actionOptions = [
  { label: "复制", value: "copy" },
  { label: "移动", value: "move" },
  { label: "删除", value: "delete" },
  { label: "重命名", value: "rename" },
  { label: "解压", value: "decompress" },
]

const Automation = () => {
  useManageTitle("manage.sidemenu.automation")
  const [tasks, setTasks] = createSignal<AutomationTask[]>([])
  const [editing, setEditing] = createSignal<AutomationTaskPayload | null>(null)
  const [historyTitle, setHistoryTitle] = createSignal("")
  const [historyRecords, setHistoryRecords] = createSignal<
    AutomationHistoryItem[]
  >([])

  const editDisclosure = createDisclosure()
  const historyDisclosure = createDisclosure()

  const [listLoading, getTasks] = useFetch(automationList)
  const [createLoading, createTask] = useFetch(automationCreate)
  const [updateLoading, updateTask] = useFetch(automationUpdate)
  const [deleteLoading, deleteTask] = useFetch(automationDelete)
  const [toggleLoading, toggleTask] = useFetch(automationToggle)
  const [runLoading, runTask] = useFetch(automationRun)
  const [historyLoading, loadHistory] = useFetch(automationHistory)

  const refresh = async () => {
    const resp = await getTasks()
    handleResp(resp, (data) => setTasks(data))
  }

  onMount(() => {
    refresh()
  })

  const openCreate = () => {
    setEditing(buildDefaultTask())
    editDisclosure.onOpen()
  }

  const openEdit = (task: AutomationTask) => {
    setEditing(toPayload(task))
    editDisclosure.onOpen()
  }

  const updateCurrent = (
    updater: (payload: AutomationTaskPayload) => AutomationTaskPayload,
  ) => {
    setEditing((prev) => {
      if (!prev) return prev
      return updater({
        ...prev,
        steps: prev.steps.map((step) => ({ ...step })),
      })
    })
  }

  const saveTask = async () => {
    const payload = editing()
    if (!payload) return
    if (!payload.name.trim()) {
      notify.warning("请填写任务名称")
      return
    }
    if (payload.steps.length === 0) {
      notify.warning("请至少添加一个步骤")
      return
    }
    const normalized = {
      ...payload,
      steps: payload.steps.map((step, index) => ({
        ...step,
        sort_order: index + 1,
        options: step.options || {},
      })),
    }
    const resp = payload.id
      ? await updateTask(normalized)
      : await createTask(normalized)
    handleRespWithNotifySuccess(resp, () => {
      editDisclosure.onClose()
      refresh()
    })
  }

  const removeTask = async (id: number) => {
    const resp = await deleteTask(id)
    handleRespWithNotifySuccess(resp, () => {
      refresh()
    })
  }

  const toggleTaskState = async (task: AutomationTask) => {
    const resp = await toggleTask({ id: task.id, enabled: !task.enabled })
    handleRespWithNotifySuccess(resp, () => {
      refresh()
    })
  }

  const runOnce = async (task: AutomationTask) => {
    const resp = await runTask(task.id)
    handleRespWithNotifySuccess(resp, () => {
      notify.success("已发送执行指令")
    })
  }

  const openHistory = async (task: AutomationTask) => {
    setHistoryTitle(task.name)
    historyDisclosure.onOpen()
    const resp = await loadHistory(task.id)
    handleResp(resp, (data) => setHistoryRecords(data))
  }

  const current = () => editing()

  return (
    <VStack alignItems="flex-start" spacing="$3" w="$full">
      <HStack spacing="$2">
        <Button colorScheme="accent" loading={listLoading()} onClick={refresh}>
          刷新
        </Button>
        <Button onClick={openCreate}>新增任务</Button>
      </HStack>
      <Table highlightOnHover>
        <Thead>
          <Tr>
            <Th>任务名称</Th>
            <Th>调度</Th>
            <Th>状态</Th>
            <Th>下次执行</Th>
            <Th>操作</Th>
          </Tr>
        </Thead>
        <Tbody>
          <For
            each={tasks()}
            fallback={
              <Tr>
                <Td colSpan={5}>暂无任务</Td>
              </Tr>
            }
          >
            {(task) => (
              <Tr>
                <Td>{task.name}</Td>
                <Td>{describeSchedule(task)}</Td>
                <Td>{task.status}</Td>
                <Td>
                  <Show
                    when={task.next_run_at}
                    fallback={<Text color="$neutral11">--</Text>}
                  >
                    {(next) => <Text>{formatDate(next())}</Text>}
                  </Show>
                </Td>
                <Td>
                  <HStack spacing="$2" wrap>
                    <Button size="sm" onClick={() => openEdit(task)}>
                      编辑
                    </Button>
                    <HopeSwitch
                      checked={task.enabled}
                      disabled={toggleLoading()}
                      onChange={() => toggleTaskState(task)}
                    >
                      {task.enabled ? "开启" : "关闭"}
                    </HopeSwitch>
                    <Button
                      size="sm"
                      loading={runLoading()}
                      onClick={() => runOnce(task)}
                    >
                      立即执行
                    </Button>
                    <Button
                      size="sm"
                      colorScheme="info"
                      loading={historyLoading()}
                      onClick={() => openHistory(task)}
                    >
                      历史
                    </Button>
                    <Button
                      size="sm"
                      colorScheme="danger"
                      loading={deleteLoading()}
                      onClick={() => removeTask(task.id)}
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

      <Modal
        opened={editDisclosure.isOpen()}
        onClose={editDisclosure.onClose}
        size="xl"
      >
        <ModalOverlay />
        <ModalContent>
          <ModalCloseButton />
          <ModalHeader>{current()?.id ? "编辑任务" : "新增任务"}</ModalHeader>
          <ModalBody>
            <VStack alignItems="stretch" spacing="$3">
              <Input
                placeholder="任务名称"
                value={current()?.name || ""}
                onInput={(e: any) =>
                  updateCurrent((payload) => ({
                    ...payload,
                    name: (e.target as HTMLInputElement).value,
                  }))
                }
              />
              <HopeSwitch
                checked={current()?.enabled ?? true}
                onChange={(e: { currentTarget: HTMLInputElement }) =>
                  updateCurrent((payload) => ({
                    ...payload,
                    enabled: e.currentTarget.checked,
                  }))
                }
              >
                {current()?.enabled ? "任务开启" : "任务关闭"}
              </HopeSwitch>
              <Select
                value={current()?.schedule_type || "interval"}
                onChange={(value) =>
                  updateCurrent(
                    (payload) =>
                      ({
                        ...payload,
                        schedule_type: value as string,
                      }) as AutomationTaskPayload,
                  )
                }
              >
                <SelectTrigger>
                  <SelectPlaceholder>选择调度方式</SelectPlaceholder>
                  <SelectValue />
                  <SelectIcon />
                </SelectTrigger>
                <SelectContent>
                  <SelectListbox>
                    <SelectOption value="interval">
                      <SelectOptionText>按间隔</SelectOptionText>
                    </SelectOption>
                    <SelectOption value="weekly">
                      <SelectOptionText>按星期</SelectOptionText>
                    </SelectOption>
                    <SelectOption value="once">
                      <SelectOptionText>指定时间</SelectOptionText>
                    </SelectOption>
                  </SelectListbox>
                </SelectContent>
              </Select>
              <Show when={current()?.schedule_type === "interval"}>
                <HStack spacing="$2" alignItems="center">
                  <Input
                    type="number"
                    min="1"
                    value={current()?.interval_value ?? 1}
                    onInput={(e: any) =>
                      updateCurrent((payload) => ({
                        ...payload,
                        interval_value: Number(
                          (e.target as HTMLInputElement).value || 1,
                        ),
                      }))
                    }
                  />
                  <Select
                    value={current()?.interval_unit || "hour"}
                    onChange={(value) =>
                      updateCurrent((payload) => ({
                        ...payload,
                        interval_unit: value as string,
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                      <SelectIcon />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectListbox>
                        <SelectOption value="second">
                          <SelectOptionText>秒</SelectOptionText>
                        </SelectOption>
                        <SelectOption value="minute">
                          <SelectOptionText>分钟</SelectOptionText>
                        </SelectOption>
                        <SelectOption value="hour">
                          <SelectOptionText>小时</SelectOptionText>
                        </SelectOption>
                        <SelectOption value="day">
                          <SelectOptionText>天</SelectOptionText>
                        </SelectOption>
                      </SelectListbox>
                    </SelectContent>
                  </Select>
                </HStack>
              </Show>
              <Show when={current()?.schedule_type === "weekly"}>
                <VStack alignItems="flex-start" spacing="$2">
                  <Text>选择星期</Text>
                  <CheckboxGroup
                    value={current()?.weekdays || []}
                    onChange={(values) =>
                      updateCurrent((payload) => ({
                        ...payload,
                        weekdays: (values as number[]).map(Number),
                      }))
                    }
                  >
                    <HStack spacing="$2" wrap>
                      <For each={weekdayLabels}>
                        {(label, index) => (
                          <Checkbox value={index()}>{label}</Checkbox>
                        )}
                      </For>
                    </HStack>
                  </CheckboxGroup>
                  <Input
                    placeholder="如 08:00"
                    value={current()?.time_of_day || ""}
                    onInput={(e: any) =>
                      updateCurrent((payload) => ({
                        ...payload,
                        time_of_day: (e.target as HTMLInputElement).value,
                      }))
                    }
                  />
                </VStack>
              </Show>
              <Show when={current()?.schedule_type === "once"}>
                <Input
                  placeholder="格式 2000/01/02 19:00"
                  value={current()?.specific_time || ""}
                  onInput={(e: any) =>
                    updateCurrent((payload) => ({
                      ...payload,
                      specific_time: (e.target as HTMLInputElement).value,
                    }))
                  }
                />
              </Show>
              <Divider />
              <HStack
                justifyContent="space-between"
                alignItems="center"
                w="$full"
              >
                <Text>任务步骤</Text>
                <Button
                  leftIcon={<BsPlus />}
                  size="sm"
                  onClick={() =>
                    updateCurrent((payload) => ({
                      ...payload,
                      steps: [
                        ...payload.steps,
                        {
                          ...defaultStep(),
                          sort_order: payload.steps.length + 1,
                        },
                      ],
                    }))
                  }
                >
                  添加步骤
                </Button>
              </HStack>
              <For each={current()?.steps || []}>
                {(step, index) => (
                  <Box
                    border="1px solid"
                    borderColor="$neutral6"
                    rounded="$lg"
                    p="$3"
                  >
                    <HStack
                      justifyContent="space-between"
                      alignItems="center"
                      mb="$2"
                    >
                      <Text>步骤 {index() + 1}</Text>
                      <HStack spacing="$1">
                        <IconButton
                          aria-label="up"
                          size="sm"
                          disabled={index() === 0}
                          icon={<FaSolidArrowUp />}
                          onClick={() =>
                            updateCurrent((payload) => {
                              const steps = [...payload.steps]
                              const i = index()
                              ;[steps[i - 1], steps[i]] = [
                                steps[i],
                                steps[i - 1],
                              ]
                              return { ...payload, steps }
                            })
                          }
                        />
                        <IconButton
                          aria-label="down"
                          size="sm"
                          disabled={
                            index() === (current()?.steps.length || 1) - 1
                          }
                          icon={<FaSolidArrowDown />}
                          onClick={() =>
                            updateCurrent((payload) => {
                              const steps = [...payload.steps]
                              const i = index()
                              ;[steps[i + 1], steps[i]] = [
                                steps[i],
                                steps[i + 1],
                              ]
                              return { ...payload, steps }
                            })
                          }
                        />
                        <IconButton
                          aria-label="remove"
                          colorScheme="danger"
                          size="sm"
                          icon={<BsX />}
                          onClick={() =>
                            updateCurrent((payload) => ({
                              ...payload,
                              steps: payload.steps
                                .filter((_, i) => i !== index())
                                .map((item, idx) => ({
                                  ...item,
                                  sort_order: idx + 1,
                                })),
                            }))
                          }
                        />
                      </HStack>
                    </HStack>
                    <VStack alignItems="stretch" spacing="$2">
                      <Select
                        value={step.action}
                        onChange={(value) =>
                          updateCurrent((payload) => {
                            const steps = [...payload.steps]
                            steps[index()] = {
                              ...steps[index()],
                              action: value as string,
                            }
                            return { ...payload, steps }
                          })
                        }
                      >
                        <SelectTrigger>
                          <SelectValue />
                          <SelectIcon />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectListbox>
                            <For each={actionOptions}>
                              {(item) => (
                                <SelectOption value={item.value}>
                                  <SelectOptionText>
                                    {item.label}
                                  </SelectOptionText>
                                </SelectOption>
                              )}
                            </For>
                          </SelectListbox>
                        </SelectContent>
                      </Select>
                      <Input
                        placeholder="源路径或通配符"
                        value={step.source}
                        onInput={(e: any) =>
                          updateCurrent((payload) => {
                            const steps = [...payload.steps]
                            steps[index()] = {
                              ...steps[index()],
                              source: (e.target as HTMLInputElement).value,
                            }
                            return { ...payload, steps }
                          })
                        }
                      />
                      <Show
                        when={["copy", "move", "decompress"].includes(
                          step.action,
                        )}
                      >
                        <Input
                          placeholder="目标路径"
                          value={step.target || ""}
                          onInput={(e: any) =>
                            updateCurrent((payload) => {
                              const steps = [...payload.steps]
                              steps[index()] = {
                                ...steps[index()],
                                target: (e.target as HTMLInputElement).value,
                              }
                              return { ...payload, steps }
                            })
                          }
                        />
                      </Show>
                      <Show when={step.action === "rename"}>
                        <Input
                          placeholder="新名称"
                          value={step.target || ""}
                          onInput={(e: any) =>
                            updateCurrent((payload) => {
                              const steps = [...payload.steps]
                              steps[index()] = {
                                ...steps[index()],
                                target: (e.target as HTMLInputElement).value,
                              }
                              return { ...payload, steps }
                            })
                          }
                        />
                      </Show>
                      <Show when={step.action === "decompress"}>
                        <VStack alignItems="stretch" spacing="$1">
                          <Input
                            placeholder="解压密码（可选）"
                            value={step.options?.password || ""}
                            onInput={(e: any) =>
                              updateCurrent((payload) => {
                                const steps = [...payload.steps]
                                steps[index()] = {
                                  ...steps[index()],
                                  options: {
                                    ...steps[index()].options,
                                    password: (e.target as HTMLInputElement)
                                      .value,
                                  },
                                }
                                return { ...payload, steps }
                              })
                            }
                          />
                          <Input
                            placeholder="内部路径（默认为 /）"
                            value={step.options?.inner_path || ""}
                            onInput={(e: any) =>
                              updateCurrent((payload) => {
                                const steps = [...payload.steps]
                                steps[index()] = {
                                  ...steps[index()],
                                  options: {
                                    ...steps[index()].options,
                                    inner_path: (e.target as HTMLInputElement)
                                      .value,
                                  },
                                }
                                return { ...payload, steps }
                              })
                            }
                          />
                        </VStack>
                      </Show>
                    </VStack>
                  </Box>
                )}
              </For>
            </VStack>
          </ModalBody>
          <ModalFooter>
            <HStack spacing="$2">
              <Button onClick={editDisclosure.onClose}>取消</Button>
              <Button
                colorScheme="accent"
                loading={createLoading() || updateLoading()}
                onClick={saveTask}
              >
                保存
              </Button>
            </HStack>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal
        opened={historyDisclosure.isOpen()}
        onClose={historyDisclosure.onClose}
        size="lg"
      >
        <ModalOverlay />
        <ModalContent>
          <ModalCloseButton />
          <ModalHeader>历史记录 · {historyTitle()}</ModalHeader>
          <ModalBody>
            <Table>
              <Thead>
                <Tr>
                  <Th>状态</Th>
                  <Th>说明</Th>
                  <Th>开始时间</Th>
                  <Th>结束时间</Th>
                </Tr>
              </Thead>
              <Tbody>
                <For
                  each={historyRecords()}
                  fallback={
                    <Tr>
                      <Td colSpan={4}>暂无记录</Td>
                    </Tr>
                  }
                >
                  {(item) => (
                    <Tr>
                      <Td>{item.status}</Td>
                      <Td>{item.message}</Td>
                      <Td>
                        {item.started_at ? formatDate(item.started_at) : "--"}
                      </Td>
                      <Td>
                        {item.finished_at ? formatDate(item.finished_at) : "--"}
                      </Td>
                    </Tr>
                  )}
                </For>
              </Tbody>
            </Table>
          </ModalBody>
          <ModalFooter>
            <Button onClick={historyDisclosure.onClose}>关闭</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </VStack>
  )
}

export default Automation
