import {
  Input,
  Text,
  VStack,
  createDisclosure,
} from "@hope-ui/solid"
import { batch, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose } from "~/components"
import { useFetch, usePath, useRouter, useT } from "~/hooks"
import { bus, fsArchiveCompress, handleRespWithNotifySuccess } from "~/utils"
import { selectedObjs } from "~/store"

export const Compress = () => {
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [loading, ok] = useFetch(fsArchiveCompress)
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const [archiveName, setArchiveName] = createSignal("压缩包.zip")
  const [password, setPassword] = createSignal("")
  const t = useT()

  const handler = (name: string) => {
    if (name === "compress") {
      const names = selectedObjs().map((o) => o.name)
      batch(() => {
        if (names.length === 1) {
          setArchiveName(`${names[0]}.zip`)
        } else {
          setArchiveName("压缩包.zip")
        }
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
      header={t("home.toolbar.compress_header")}
      opened={isOpen()}
      onClose={onClose}
      loading={loading()}
      onSubmit={async (dst) => {
        const resp = await ok(
          pathname(),
          dst,
          selectedObjs().map((o) => o.name),
          archiveName(),
          password(),
        )
        handleRespWithNotifySuccess(resp, () => {
          refresh()
          onClose()
        })
      }}
    >
      <VStack spacing="$2" alignItems="flex-start" w="$full">
        <Text size="sm">{t("home.toolbar.compress_name")}</Text>
        <Input
          value={archiveName()}
          onInput={(e: any) => setArchiveName(e.target.value as string)}
          size="sm"
        />
        <Text size="sm">{t("home.toolbar.compress_password")}</Text>
        <Input
          value={password()}
          onInput={(e: any) => setPassword(e.target.value as string)}
          size="sm"
        />
      </VStack>
    </ModalFolderChoose>
  )
}
