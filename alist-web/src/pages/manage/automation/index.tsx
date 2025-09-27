import {
  Badge,
  Box,
  Button,
  Checkbox,
  Divider,
  Flex,
  FormControl,
  FormLabel,
  Heading,
  HStack,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Switch as HopeSwitch,
  Text,
  Textarea,
  VStack,
  createDisclosure,
} from "@hope-ui/solid"
import { For, Show, createMemo, createSignal } from "solid-js"
import { createStore } from "solid-js/store"
import { MaybeLoading } from "~/components"
import { useFetch, useListFetch, useManageTitle, useT } from "~/hooks"
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
  notify,
} from "~/utils"
import {
  AutomationHistory,
  AutomationJob,
  AutomationJobPayload,
  AutomationSchedule,
  AutomationStep,
  AutomationStepOptions,
} from "~/types"

const unitOptions = [
  { value: "second" as const, label: "秒" },
  { value: "minute" as const, label: "分钟" },
  { value: "hour" as const, label: "小时" },
  { value: "day" as const, label: "天" },
]

const weekdayOptions = [
  { value: "monday", label: "周一" },
  { value: "tuesday", label: "周二" },
  { value: "wednesday", label: "周三" },
  { value: "thursday", label: "周四" },
  { value: "friday", label: "周五" },
  { value: "saturday", label: "周六" },
  { value: "sunday", label: "周日" },
]

const createDefaultOptions = (): AutomationStepOptions => ({
  new_name: "",
  inner_path: "/",
  password: "",
  put_into_new_dir: false,
  cache_full: true,
})

const createDefaultJob = (): AutomationJobPayload => ({
  name: "",
  enabled: true,
  schedule: {
    mode: "interval",
    interval_value: 1,
    interval_unit: "hour",
    weekly_day: "monday",
    time_of_day: "09:00",
    once_at: "",
  },
  steps: [],
})

const toDateTimeLocal = (value?: string | null): string => {
  if (!value) return ""
  const parsed = new Date(value)
  if (!Number.isNaN(parsed.getTime())) {
    const local = new Date(parsed.getTime() - parsed.getTimezoneOffset() * 60000)
    return local.toISOString().slice(0, 16)
  }
  return value.replace(" ", "T").slice(0, 16)
}

const AutomationPage = () => {
  const t = useT()
  useManageTitle("manage.sidemenu.automation")
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [jobs, setJobs] = createSignal<AutomationJob[]>([])
  const [histories, setHistories] = createSignal<Record<number, AutomationHistory[]>>({})
  const [editingId, setEditingId] = createSignal<number | undefined>()
  const [form, setForm] = createStore<AutomationJobPayload>(createDefaultJob())

  const actionLabels = createMemo(() => ({
    copy: t("automation.actions.copy"),
    move: t("automation.actions.move"),
    delete: t("automation.actions.delete"),
    rename: t("automation.actions.rename"),
    decompress: t("automation.actions.decompress"),
  }))

  const [listLoading, fetchJobs] = useFetch(automationList)
  const [createLoading, createJob] = useFetch(automationCreate)
  const [updateLoading, updateJob] = useFetch(automationUpdate)
  const [deleteLoading, deleteJob] = useFetch(automationDelete)
  const [toggleLoading, toggleJob] = useFetch(automationToggle)
  const [runLoading, runJob] = useFetch(automationRun)
  const [historyLoading, requestHistory] = useListFetch(automationHistory)

  const refreshJobs = async () => {
    const resp = await fetchJobs()
    handleResp(resp, (data) => {
      setJobs(data ?? [])
      const map: Record<number, AutomationHistory[]> = {}
      data?.forEach((job) => {
        map[job.id] = job.histories ?? []
      })
      setHistories(map)
    })
  }

  refreshJobs()

  const closeModal = () => {
    onClose()
    setTimeout(() => {
      setEditingId(undefined)
      setForm(createDefaultJob())
    })
  }

  const openCreate = () => {
    setEditingId(undefined)
    setForm(createDefaultJob())
    onOpen()
  }

  const openEdit = (job: AutomationJob) => {
    setEditingId(job.id)
    const payload: AutomationJobPayload = {
      id: job.id,
      name: job.name,
      enabled: job.enabled,
      schedule: {
        mode: job.schedule.mode || "interval",
        interval_value: job.schedule.interval_value ?? 1,
        interval_unit: job.schedule.interval_unit ?? "hour",
        weekly_day: job.schedule.weekly_day || "monday",
        time_of_day: job.schedule.time_of_day || "09:00",
        once_at: toDateTimeLocal(job.schedule.once_at ?? ""),
      },
      steps: job.steps.map((step) => ({
        action: step.action,
        source: step.source,
        target: step.target,
        description: step.description,
        options: {
          new_name: step.options?.new_name ?? "",
          inner_path: step.options?.inner_path || "/",
          password: step.options?.password ?? "",
          put_into_new_dir: step.options?.put_into_new_dir ?? false,
          cache_full:
            step.options?.cache_full === undefined ? true : step.options.cache_full,
        },
      })),
    }
    setForm(payload)
    onOpen()
  }

  const moveStep = (from: number, to: number) => {
    if (to < 0 || to >= form.steps.length) return
    const steps = [...form.steps]
    const [item] = steps.splice(from, 1)
    steps.splice(to, 0, item)
    setForm("steps", steps)
  }

  const addStep = () => {
    setForm("steps", [
      ...form.steps,
      {
        action: "copy",
        source: "",
        target: "",
        description: "",
        options: createDefaultOptions(),
      },
    ])
  }

  const removeStep = (index: number) => {
    const steps = [...form.steps]
    steps.splice(index, 1)
    setForm("steps", steps)
  }

  const scheduleSummary = (schedule: AutomationSchedule) => {
    if (schedule.mode === "weekly") {
      const day = weekdayOptions.find((item) => item.value === schedule.weekly_day)
      const time = schedule.time_of_day || "00:00"
      return `每周${day?.label ?? ""} ${time}`
    }
    if (schedule.mode === "once") {
      return schedule.once_at ? `指定时间 ${schedule.once_at}` : "指定时间"
    }
    const unit = unitOptions.find((item) => item.value === schedule.interval_unit)
    return `每${schedule.interval_value ?? 1}${unit?.label ?? "小时"}`
  }

  const submit = async () => {
    const schedule: AutomationSchedule = { mode: form.schedule.mode }
    if (form.schedule.mode === "interval") {
      schedule.interval_value = form.schedule.interval_value ?? 1
      schedule.interval_unit = form.schedule.interval_unit ?? "hour"
    } else if (form.schedule.mode === "weekly") {
      schedule.weekly_day = form.schedule.weekly_day || "monday"
      schedule.time_of_day = form.schedule.time_of_day || "09:00"
    } else if (form.schedule.mode === "once") {
      schedule.once_at = form.schedule.once_at || ""
    }
    const payload: AutomationJobPayload = {
      id: editingId(),
      name: form.name,
      enabled: form.enabled,
      schedule,
      steps: form.steps.map((step) => ({
        action: step.action,
        source: step.source.trim(),
        target: step.target?.trim(),
        description: step.description?.trim(),
        options: {
          new_name: step.options?.new_name?.trim() ?? "",
          inner_path: step.options?.inner_path || "/",
          password: step.options?.password ?? "",
          put_into_new_dir: step.options?.put_into_new_dir ?? false,
          cache_full:
            step.options?.cache_full === undefined ? true : step.options.cache_full,
        },
      })),
    }
    const resp = editingId() ? await updateJob(payload) : await createJob(payload)
    handleResp(resp, (data) => {
      notify.success(t("automation.save_success"))
      if (editingId()) {
        setJobs((prev) => prev.map((job) => (job.id === data.id ? data : job)))
      } else {
        setJobs((prev) => [...prev, data])
      }
      setHistories((prev) => ({ ...prev, [data.id]: data.histories ?? [] }))
      closeModal()
    })
  }

  const handleToggle = async (job: AutomationJob) => {
    const resp = await toggleJob(job.id, !job.enabled)
    handleResp(resp, (data) => {
      notify.success(t("automation.toggle_success"))
      setJobs((prev) => prev.map((item) => (item.id === data.id ? data : item)))
    })
  }

  const handleRun = async (job: AutomationJob) => {
    const resp = await runJob(job.id)
    handleResp(resp, () => {
      notify.success(t("automation.run_success"))
    })
  }

  const handleDelete = async (job: AutomationJob) => {
    if (!confirm(t("automation.confirm_delete"))) return
    const resp = await deleteJob(job.id)
    handleResp(resp, () => {
      notify.success(t("automation.delete_success"))
      setJobs((prev) => prev.filter((item) => item.id !== job.id))
      setHistories((prev) => {
        const next = { ...prev }
        delete next[job.id]
        return next
      })
    })
  }

  const refreshHistory = async (job: AutomationJob) => {
    const resp = await requestHistory(job.id)
    handleResp(resp, (data) => {
      setHistories((prev) => ({ ...prev, [job.id]: data ?? [] }))
    })
  }

  const historyFor = (job: AutomationJob) => {
    const cache = histories()
    return cache[job.id] ?? job.histories ?? []
  }

  const formLoading = () => (editingId() ? updateLoading() : createLoading())

  return (
    <VStack w="$full" alignItems="start" spacing="$4">
      <Flex w="$full" justifyContent="space-between" alignItems="center">
        <Heading size="lg">{t("automation.title")}</Heading>
        <HStack spacing="$2">
          <Button onClick={refreshJobs}>{t("automation.refresh")}</Button>
          <Button colorScheme="primary" onClick={openCreate}>
            {t("automation.create")}
          </Button>
        </HStack>
      </Flex>
      <MaybeLoading loading={listLoading()}>
        <Show when={jobs().length > 0} fallback={<Text>{t("automation.empty")}</Text>}>
          <VStack w="$full" alignItems="stretch" spacing="$4">
            <For each={jobs()}>
              {(job) => (
                <Box border="$neutral6" borderWidth="1px" rounded="$lg" p="$4">
                  <Flex justifyContent="space-between" alignItems="center" mb="$2">
                    <HStack spacing="$2" alignItems="center">
                      <Heading size="md">{job.name}</Heading>
                      <Badge colorScheme={job.enabled ? "success" : "neutral"}>
                        {job.enabled ? t("automation.toggle_on") : t("automation.toggle_off")}
                      </Badge>
                    </HStack>
                    <HStack spacing="$2">
                      <Button size="sm" variant="subtle" onClick={() => openEdit(job)}>
                        {t("automation.edit")}
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        loading={runLoading()}
                        onClick={() => handleRun(job)}
                      >
                        {t("automation.run")}
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        loading={toggleLoading()}
                        onClick={() => handleToggle(job)}
                      >
                        {job.enabled ? t("automation.toggle_off") : t("automation.toggle_on")}
                      </Button>
                      <Button
                        size="sm"
                        colorScheme="danger"
                        loading={deleteLoading()}
                        onClick={() => handleDelete(job)}
                      >
                        {t("automation.delete")}
                      </Button>
                    </HStack>
                  </Flex>
                  <Text fontSize="sm" color="$neutral10">
                    {t("automation.next_run")}: {job.next_run_at ? formatDate(job.next_run_at) : "-"}
                  </Text>
                  <Text fontSize="sm" color="$neutral10" mb="$2">
                    {t("automation.last_run")}: {job.last_run_at ? formatDate(job.last_run_at) : "-"}
                  </Text>
                  <Text fontSize="sm" mb="$2">
                    {t("automation.form.schedule")}: {scheduleSummary(job.schedule)}
                  </Text>
                  <VStack alignItems="start" spacing="$1" mb="$3">
                    <For each={job.steps}>
                      {(step, i) => (
                        <Text fontSize="sm">
                          {i() + 1}. {actionLabels()[step.action]} {step.source}
                          <Show when={step.target && step.action !== "delete"}>
                            <Text as="span"> → {step.target}</Text>
                          </Show>
                          <Show when={step.action === "rename" && step.options?.new_name}>
                            <Text as="span">
                              {' '}（{t("automation.steps.new_name")}: {step.options?.new_name}）
                            </Text>
                          </Show>
                          <Show when={step.action === "decompress"}>
                            <Text as="span">
                              {' '}（{t("automation.steps.inner_path")}: {step.options?.inner_path || "/"}
                              {step.options?.password
                                ? `，${t("automation.steps.password")}: ${step.options?.password}`
                                : ""}
                              ）
                            </Text>
                          </Show>
                        </Text>
                      )}
                    </For>
                  </VStack>
                  <VStack alignItems="start" spacing="$2">
                    <HStack spacing="$2">
                      <Text fontWeight="$medium">{t("automation.history.title")}</Text>
                      <Button
                        size="xs"
                        variant="subtle"
                        loading={historyLoading() === job.id}
                        onClick={() => refreshHistory(job)}
                      >
                        {t("automation.history.refresh")}
                      </Button>
                    </HStack>
                    <Show
                      when={historyFor(job).length > 0}
                      fallback={<Text fontSize="sm">{t("automation.history.empty")}</Text>}
                    >
                      <VStack alignItems="start" spacing="$1">
                        <For each={historyFor(job)}>
                          {(history) => (
                            <Box
                              borderWidth="1px"
                              borderColor={history.success ? "$success6" : "$danger6"}
                              rounded="$md"
                              p="$2"
                              w="100%"
                            >
                              <Text fontSize="sm" color={history.success ? "$success11" : "$danger11"}>
                                {t("automation.history.status")}: {history.success ? "成功" : "失败"}
                              </Text>
                              <Text fontSize="sm">
                                {t("automation.history.started_at")}: {formatDate(history.started_at)}
                              </Text>
                              <Text fontSize="sm">
                                {t("automation.history.finished_at")}:
                                {history.finished_at ? formatDate(history.finished_at) : "-"}
                              </Text>
                              <Text fontSize="sm">{t("automation.history.message")}: {history.message}</Text>
                            </Box>
                          )}
                        </For>
                      </VStack>
                    </Show>
                  </VStack>
                </Box>
              )}
            </For>
          </VStack>
        </Show>
      </MaybeLoading>

      <Modal opened={isOpen()} onClose={closeModal} size="xl" scrollBehavior="inside">
        <ModalOverlay />
        <ModalContent>
          <ModalCloseButton />
          <ModalHeader>
            {editingId() ? t("automation.edit") : t("automation.create")}
          </ModalHeader>
          <ModalBody>
            <VStack w="$full" alignItems="start" spacing="$3">
              <FormControl w="$full" required>
                <FormLabel>{t("automation.form.name")}</FormLabel>
                <Input
                  value={form.name}
                  onInput={(e) => setForm("name", e.currentTarget.value)}
                />
              </FormControl>
              <FormControl display="flex" alignItems="center">
                <FormLabel mb="0">{t("automation.form.enabled")}</FormLabel>
                <HopeSwitch
                  checked={form.enabled}
                  onChange={(e) => setForm("enabled", e.currentTarget.checked)}
                />
              </FormControl>
              <Divider />
              <VStack w="$full" alignItems="start" spacing="$2">
                <Text fontWeight="$medium">{t("automation.form.schedule")}</Text>
                <FormControl w="$full">
                  <FormLabel>{t("automation.form.mode")}</FormLabel>
                  <select
                    value={form.schedule.mode}
                    onChange={(e) =>
                      setForm("schedule", {
                        ...form.schedule,
                        mode: e.currentTarget.value as AutomationSchedule["mode"],
                      })
                    }
                    style={{
                      padding: "6px 8px",
                      border: "1px solid var(--hope-colors-neutral6)",
                      "border-radius": "6px",
                      width: "100%",
                    }}
                  >
                    <option value="interval">{t("automation.form.interval")}</option>
                    <option value="weekly">{t("automation.form.weekly")}</option>
                    <option value="once">{t("automation.form.once")}</option>
                  </select>
                </FormControl>
                <Show when={form.schedule.mode === "interval"}>
                  <HStack spacing="$2" w="$full">
                    <FormControl>
                      <FormLabel>{t("automation.form.interval_value")}</FormLabel>
                      <Input
                        type="number"
                        min="1"
                        value={form.schedule.interval_value ?? 1}
                        onInput={(e) =>
                          setForm("schedule", {
                            ...form.schedule,
                            interval_value: Number(e.currentTarget.value) || 1,
                          })
                        }
                      />
                    </FormControl>
                    <FormControl>
                      <FormLabel>{t("automation.form.interval_unit")}</FormLabel>
                      <select
                        value={form.schedule.interval_unit ?? "hour"}
                        onChange={(e) =>
                          setForm("schedule", {
                            ...form.schedule,
                            interval_unit: e.currentTarget.value as AutomationSchedule["interval_unit"],
                          })
                        }
                        style={{
                          padding: "6px 8px",
                          border: "1px solid var(--hope-colors-neutral6)",
                          "border-radius": "6px",
                          width: "100%",
                        }}
                      >
                        {unitOptions.map((unit) => (
                          <option value={unit.value}>{unit.label}</option>
                        ))}
                      </select>
                    </FormControl>
                  </HStack>
                </Show>
                <Show when={form.schedule.mode === "weekly"}>
                  <HStack spacing="$2" w="$full">
                    <FormControl>
                      <FormLabel>{t("automation.form.weekly_day")}</FormLabel>
                      <select
                        value={form.schedule.weekly_day || "monday"}
                        onChange={(e) =>
                          setForm("schedule", {
                            ...form.schedule,
                            weekly_day: e.currentTarget.value,
                          })
                        }
                        style={{
                          padding: "6px 8px",
                          border: "1px solid var(--hope-colors-neutral6)",
                          "border-radius": "6px",
                          width: "100%",
                        }}
                      >
                        {weekdayOptions.map((day) => (
                          <option value={day.value}>{day.label}</option>
                        ))}
                      </select>
                    </FormControl>
                    <FormControl>
                      <FormLabel>{t("automation.form.time_of_day")}</FormLabel>
                      <Input
                        type="time"
                        value={form.schedule.time_of_day || "09:00"}
                        onInput={(e) =>
                          setForm("schedule", {
                            ...form.schedule,
                            time_of_day: e.currentTarget.value,
                          })
                        }
                      />
                    </FormControl>
                  </HStack>
                </Show>
                <Show when={form.schedule.mode === "once"}>
                  <FormControl w="$full">
                    <FormLabel>{t("automation.form.once_at")}</FormLabel>
                    <Input
                      type="datetime-local"
                      value={form.schedule.once_at || ""}
                      onInput={(e) =>
                        setForm("schedule", {
                          ...form.schedule,
                          once_at: e.currentTarget.value,
                        })
                      }
                    />
                  </FormControl>
                </Show>
              </VStack>
              <Divider />
              <VStack w="$full" alignItems="start" spacing="$2">
                <Flex w="$full" justifyContent="space-between" alignItems="center">
                  <Text fontWeight="$medium">{t("automation.steps.title")}</Text>
                  <Button size="sm" onClick={addStep}>
                    {t("automation.steps.add")}
                  </Button>
                </Flex>
                <Show when={form.steps.length > 0} fallback={<Text fontSize="sm">{t("automation.history.empty")}</Text>}>
                  <VStack w="$full" alignItems="stretch" spacing="$3">
                    <For each={form.steps}>
                      {(step, i) => (
                        <Box borderWidth="1px" borderColor="$neutral6" rounded="$md" p="$3">
                          <Flex justifyContent="space-between" alignItems="center" mb="$2">
                            <Text fontWeight="$medium">{`${t("automation.steps.title")}${i() + 1}`}</Text>
                            <HStack spacing="$1">
                              <Button
                                size="xs"
                                variant="subtle"
                                onClick={() => moveStep(i(), i() - 1)}
                                disabled={i() === 0}
                              >
                                {t("automation.steps.move_up")}
                              </Button>
                              <Button
                                size="xs"
                                variant="subtle"
                                onClick={() => moveStep(i(), i() + 1)}
                                disabled={i() === form.steps.length - 1}
                              >
                                {t("automation.steps.move_down")}
                              </Button>
                              <Button
                                size="xs"
                                colorScheme="danger"
                                onClick={() => removeStep(i())}
                              >
                                {t("automation.steps.remove")}
                              </Button>
                            </HStack>
                          </Flex>
                          <VStack alignItems="start" spacing="$2">
                            <FormControl w="$full">
                              <FormLabel>{t("automation.steps.action")}</FormLabel>
                              <select
                                value={step.action}
                                onChange={(e) =>
                                  setForm("steps", i(), {
                                    ...step,
                                    action: e.currentTarget.value as AutomationStep["action"],
                                  })
                                }
                                style={{
                                  padding: "6px 8px",
                                  border: "1px solid var(--hope-colors-neutral6)",
                                  "border-radius": "6px",
                                  width: "100%",
                                }}
                              >
                                {Object.entries(actionLabels()).map(([value, label]) => (
                                  <option value={value}>{label}</option>
                                ))}
                              </select>
                            </FormControl>
                            <FormControl w="$full">
                              <FormLabel>{t("automation.steps.source")}</FormLabel>
                              <Input
                                value={step.source}
                                onInput={(e) =>
                                  setForm("steps", i(), "source", e.currentTarget.value)
                                }
                              />
                            </FormControl>
                            <Show when={step.action !== "delete"}>
                              <FormControl w="$full">
                                <FormLabel>{t("automation.steps.target")}</FormLabel>
                                <Input
                                  value={step.target || ""}
                                  onInput={(e) =>
                                    setForm("steps", i(), "target", e.currentTarget.value)
                                  }
                                />
                              </FormControl>
                            </Show>
                            <FormControl w="$full">
                              <FormLabel>{t("automation.steps.description")}</FormLabel>
                              <Textarea
                                value={step.description || ""}
                                onInput={(e) =>
                                  setForm("steps", i(), "description", e.currentTarget.value)
                                }
                              />
                            </FormControl>
                            <Show when={step.action === "rename"}>
                              <FormControl w="$full">
                                <FormLabel>{t("automation.steps.new_name")}</FormLabel>
                                <Input
                                  value={step.options?.new_name || ""}
                                  onInput={(e) =>
                                    setForm("steps", i(), "options", {
                                      ...step.options,
                                      new_name: e.currentTarget.value,
                                    })
                                  }
                                />
                              </FormControl>
                            </Show>
                            <Show when={step.action === "decompress"}>
                              <VStack alignItems="start" spacing="$2" w="$full">
                                <FormControl w="$full">
                                  <FormLabel>{t("automation.steps.inner_path")}</FormLabel>
                                  <Input
                                    value={step.options?.inner_path || "/"}
                                    onInput={(e) =>
                                      setForm("steps", i(), "options", {
                                        ...step.options,
                                        inner_path: e.currentTarget.value,
                                      })
                                    }
                                  />
                                </FormControl>
                                <FormControl w="$full">
                                  <FormLabel>{t("automation.steps.password")}</FormLabel>
                                  <Input
                                    value={step.options?.password || ""}
                                    onInput={(e) =>
                                      setForm("steps", i(), "options", {
                                        ...step.options,
                                        password: e.currentTarget.value,
                                      })
                                    }
                                  />
                                </FormControl>
                                <Checkbox
                                  checked={step.options?.put_into_new_dir ?? false}
                                  onChange={(e) =>
                                    setForm("steps", i(), "options", {
                                      ...step.options,
                                      put_into_new_dir: e.currentTarget.checked,
                                    })
                                  }
                                >
                                  {t("automation.steps.put_into_new_dir")}
                                </Checkbox>
                                <Checkbox
                                  checked={step.options?.cache_full ?? true}
                                  onChange={(e) =>
                                    setForm("steps", i(), "options", {
                                      ...step.options,
                                      cache_full: e.currentTarget.checked,
                                    })
                                  }
                                >
                                  {t("automation.steps.cache_full")}
                                </Checkbox>
                              </VStack>
                            </Show>
                          </VStack>
                        </Box>
                      )}
                    </For>
                  </VStack>
                </Show>
              </VStack>
            </VStack>
          </ModalBody>
          <ModalFooter>
            <HStack spacing="$2">
              <Button variant="ghost" onClick={closeModal}>
                {t("automation.cancel")}
              </Button>
              <Button colorScheme="primary" loading={formLoading()} onClick={submit}>
                {t("automation.submit")}
              </Button>
            </HStack>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </VStack>
  )
}

export default AutomationPage
