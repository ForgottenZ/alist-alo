import {
  HStack,
  Input,
  Text,
  VStack,
  createDisclosure,
} from "@hope-ui/solid"
import { batch, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose } from "~/components"
import { useFetch, usePath, useRouter, useT } from "~/hooks"
import {
  bus,
  fsArchiveCompress,
  handleRespWithNotifySuccess,
  notify,
} from "~/utils"
import { selectedObjs } from "~/store"

const buildDefaultArchiveName = () => {
  const list = selectedObjs()
  if (list.length === 0) {
    return "压缩包.zip"
  }
  if (list.length === 1) {
    const name = list[0].name.replace(/\/$/, "")
    const base = name.replace(/\.[^./]+$/, "")
    return `${base || "压缩包"}.zip`
  }
  return `批量压缩_${new Date().getTime()}.zip`
}

export const Compress = () => {
  const t = useT()
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [loading, ok] = useFetch(fsArchiveCompress)
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const [archiveName, setArchiveName] = createSignal("")
  const [password, setPassword] = createSignal("")
  const handler = (name: string) => {
    if (name === "compress") {
      if (selectedObjs().length === 0) {
        notify.warning("请先选择要压缩的文件")
        return
      }
      batch(() => {
        setArchiveName(buildDefaultArchiveName())
        setPassword("")
      })
      onOpen()
    }
  }
  bus.on("tool", handler)
  onCleanup(() => {
    bus.off("tool", handler)
  })
  return (
    <ModalFolderChoose
      header={t("home.toolbar.compress-dst")}
      opened={isOpen()}
      onClose={onClose}
      loading={loading()}
      onSubmit={async (dst) => {
        if (!archiveName().trim()) {
          notify.warning("请填写压缩包名称")
          return
        }
        const resp = await ok(
          pathname(),
          dst,
          selectedObjs().map((item) => item.name),
          archiveName().trim(),
          password(),
        )
        handleRespWithNotifySuccess(resp, () => {
          refresh()
          onClose()
        })
      }}
    >
      <VStack spacing="$2" alignItems="flex-start" w="$full">
        <HStack w="$full" spacing="$1">
          <Text size="sm" css={{ whiteSpace: "nowrap" }}>
            {t("home.toolbar.compress-name")}
          </Text>
          <Input
            value={archiveName()}
            onInput={(e: any) => setArchiveName(e.target.value as string)}
            size="sm"
          />
        </HStack>
        <HStack w="$full" spacing="$1">
          <Text size="sm" css={{ whiteSpace: "nowrap" }}>
            {t("home.toolbar.compress-password")}
          </Text>
          <Input
            value={password()}
            onInput={(e: any) => setPassword(e.target.value as string)}
            size="sm"
          />
        </HStack>
      </VStack>
    </ModalFolderChoose>
  )
}
