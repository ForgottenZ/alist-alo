import {
  Box,
  Button,
  Checkbox,
  FormControl,
  FormLabel,
  HStack,
  Input,
  Table,
  Tbody,
  Td,
  Textarea,
  Th,
  Thead,
  Tr,
  VStack,
} from "@hope-ui/solid"
import { For } from "solid-js"
import { createStore } from "solid-js/store"
import { SelectWrapper } from "~/components"
import { useFetch, useListFetch, useManageTitle, useT } from "~/hooks"
import {
  NotificationItem,
  NotificationType,
  PEmptyResp,
  PPageResp,
} from "~/types"
import { handleResp, notify, r } from "~/utils"
import { DeletePopover } from "../common/DeletePopover"

const defaultConfig: Record<NotificationType, string> = {
  pushdeer: JSON.stringify(
    {
      push_key: "",
      endpoint: "https://api2.pushdeer.com/message/push",
    },
    null,
    2,
  ),
  azure_oauth: JSON.stringify(
    {
      tenant_id: "",
      client_id: "",
      client_secret: "",
      scope: "",
      notify_url: "",
      method: "POST",
      body_template: '{"title":"{{title}}","body":"{{body}}"}',
    },
    null,
    2,
  ),
}

const emptyItem = (): NotificationItem => ({
  id: 0,
  name: "",
  type: "pushdeer",
  enabled: true,
  config: defaultConfig.pushdeer,
  remark: "",
})

const Notifications = () => {
  const t = useT()
  useManageTitle("manage.sidemenu.notifications")
  const [editing, setEditing] = createStore<NotificationItem>(emptyItem())
  const [getLoading, getItems] = useFetch(
    (): PPageResp<NotificationItem> => r.get("/admin/notification/list"),
  )
  const [items, setItems] = createStore<NotificationItem[]>([])
  const refresh = async () => {
    const resp = await getItems()
    handleResp(resp, (data) => setItems(data.content))
  }
  refresh()

  const [saving, saveItem] = useFetch((): PEmptyResp => {
    const action = editing.id ? "update" : "create"
    return r.post(`/admin/notification/${action}`, editing)
  })
  const [testing, testItem] = useListFetch((id: number): PEmptyResp => {
    return r.post(`/admin/notification/test?id=${id}`, {
      title: t("notifications.test_title"),
      body: t("notifications.test_body"),
    })
  })
  const [deleting, deleteItem] = useListFetch((id: number): PEmptyResp =>
    r.post(`/admin/notification/delete?id=${id}`),
  )

  const reset = () => setEditing(emptyItem())
  const providerOptions = (): { value: NotificationType; label: string }[] => [
    { value: "pushdeer", label: t("notifications.providers.pushdeer") },
    { value: "azure_oauth", label: t("notifications.providers.azure_oauth") },
  ]

  return (
    <VStack spacing="$4" alignItems="start" w="$full">
      <HStack spacing="$2">
        <Button colorScheme="accent" loading={getLoading()} onClick={refresh}>
          {t("global.refresh")}
        </Button>
        <Button onClick={reset}>{t("global.add")}</Button>
      </HStack>

      <Box w="$full" overflowX="auto">
        <Table highlightOnHover dense>
          <Thead>
            <Tr>
              <For each={["name", "type", "enabled", "remark"]}>
                {(key) => <Th>{t(`notifications.${key}`)}</Th>}
              </For>
              <Th>{t("global.operations")}</Th>
            </Tr>
          </Thead>
          <Tbody>
            <For each={items}>
              {(item) => (
                <Tr>
                  <Td>{item.name}</Td>
                  <Td>{t(`notifications.providers.${item.type}`)}</Td>
                  <Td>{item.enabled ? t("global.yes") : t("global.no")}</Td>
                  <Td>{item.remark}</Td>
                  <Td>
                    <HStack spacing="$2">
                      <Button onClick={() => setEditing(item)}>
                        {t("global.edit")}
                      </Button>
                      <Button
                        loading={testing() === item.id}
                        onClick={async () => {
                          const resp = await testItem(item.id)
                          handleResp(resp, () =>
                            notify.success(t("notifications.test_success")),
                          )
                        }}
                      >
                        {t("notifications.test")}
                      </Button>
                      <DeletePopover
                        name={item.name}
                        loading={deleting() === item.id}
                        onClick={async () => {
                          const resp = await deleteItem(item.id)
                          handleResp(resp, () => {
                            notify.success(t("global.delete_success"))
                            refresh()
                            if (editing.id === item.id) reset()
                          })
                        }}
                      />
                    </HStack>
                  </Td>
                </Tr>
              )}
            </For>
          </Tbody>
        </Table>
      </Box>

      <VStack spacing="$2" alignItems="start" w="$full">
        <FormControl required>
          <FormLabel>{t("notifications.name")}</FormLabel>
          <Input
            value={editing.name}
            onInput={(e) => setEditing("name", e.currentTarget.value)}
          />
        </FormControl>
        <FormControl required>
          <FormLabel>{t("notifications.type")}</FormLabel>
          <Box w={{ "@initial": "$full", "@md": "$80" }}>
            <SelectWrapper
              value={editing.type}
              onChange={(value) => {
                setEditing("type", value)
                setEditing("config", defaultConfig[value])
              }}
              options={providerOptions()}
            />
          </Box>
        </FormControl>
        <Checkbox
          checked={editing.enabled}
          onChange={() => setEditing("enabled", !editing.enabled)}
        >
          {t("notifications.enabled")}
        </Checkbox>
        <FormControl required>
          <FormLabel>{t("notifications.config")}</FormLabel>
          <Textarea
            rows={9}
            value={editing.config}
            onInput={(e) => setEditing("config", e.currentTarget.value)}
          />
        </FormControl>
        <FormControl>
          <FormLabel>{t("notifications.remark")}</FormLabel>
          <Textarea
            rows={2}
            value={editing.remark}
            onInput={(e) => setEditing("remark", e.currentTarget.value)}
          />
        </FormControl>
        <HStack spacing="$2">
          <Button
            colorScheme="accent"
            loading={saving()}
            onClick={async () => {
              const resp = await saveItem()
              handleResp(resp, () => {
                notify.success(t("global.save_success"))
                reset()
                refresh()
              })
            }}
          >
            {t(`global.${editing.id ? "save" : "add"}`)}
          </Button>
          <Button onClick={reset}>{t("global.reset")}</Button>
        </HStack>
      </VStack>
    </VStack>
  )
}

export default Notifications
