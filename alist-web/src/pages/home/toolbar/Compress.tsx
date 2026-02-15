import { createDisclosure, HStack, Input, Select, Text, VStack } from "@hope-ui/solid"
import { useFetch, usePath, useRouter, useT } from "~/hooks"
import { bus, fsArchiveCompress, handleRespWithNotifySuccess } from "~/utils"
import { batch, createMemo, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose } from "~/components"
import { selectedObjs } from "~/store"

export const Compress = () => {
  const t = useT()
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [loading, ok] = useFetch(fsArchiveCompress)
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const [archiveName, setArchiveName] = createSignal("archive")
  const [format, setFormat] = createSignal<"zip" | "7z">("7z")
  const [password, setPassword] = createSignal("")

  const defaultName = createMemo(() => {
    const list = selectedObjs()
    if (list.length === 1) {
      return list[0].name
    }
    return "archive"
  })

  const handler = (name: string) => {
    if (name !== "compress") return
    batch(() => {
      setArchiveName(defaultName())
      setFormat("7z")
      setPassword("")
    })
    onOpen()
  }

  bus.on("tool", handler)
  onCleanup(() => {
    bus.off("tool", handler)
  })

  return (
    <ModalFolderChoose
      header={t("home.toolbar.choose_dst_folder")}
      opened={isOpen()}
      onClose={onClose}
      loading={loading()}
      onSubmit={async (dst) => {
        const ext = format()
        let name = archiveName().trim()
        if (!name.toLowerCase().endsWith(`.${ext}`)) {
          name = `${name}.${ext}`
        }
        const resp = await ok(
          pathname(),
          dst,
          selectedObjs().map((o) => o.name),
          format(),
          name,
          password(),
        )
        handleRespWithNotifySuccess(resp, () => {
          refresh()
          onClose()
        })
      }}
    >
      <VStack spacing="$1" alignItems="flex-start">
        <HStack width="100%" spacing="$1">
          <Text size="sm" css={{ whiteSpace: "nowrap" }}>
            {t("home.toolbar.input_new_name")}
          </Text>
          <Input
            value={archiveName()}
            onInput={(e: any) => setArchiveName(e.target.value as string)}
            size="sm"
            flexGrow="1"
          />
        </HStack>
        <HStack width="100%" spacing="$1">
          <Text size="sm">Format</Text>
          <Select
            size="sm"
            value={format()}
            onChange={(e: any) => setFormat(e.target.value as "zip" | "7z")}
            flexGrow="1"
          >
            <option value="7z">7z</option>
            <option value="zip">zip</option>
          </Select>
        </HStack>
        <HStack width="100%" spacing="$1">
          <Text size="sm" css={{ whiteSpace: "nowrap" }}>
            {t("home.toolbar.decompress-pass")}
          </Text>
          <Input
            value={password()}
            onInput={(e: any) => setPassword(e.target.value as string)}
            size="sm"
            flexGrow="1"
          />
        </HStack>
      </VStack>
    </ModalFolderChoose>
  )
}
