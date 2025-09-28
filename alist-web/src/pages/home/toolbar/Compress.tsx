import { createDisclosure, Input, Text, VStack } from "@hope-ui/solid"
import { batch, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose } from "~/components"
import { selectedObjs, userCan } from "~/store"
import { bus, fsArchiveCompress, handleRespWithNotifySuccess } from "~/utils"
import { useFetch, usePath, useRouter, useT } from "~/hooks"

const guessArchiveName = () => {
  const objs = selectedObjs()
  if (objs.length === 1) {
    const name = objs[0].name
    const dot = name.lastIndexOf(".")
    if (dot > 0) {
      return `${name.slice(0, dot)}.zip`
    }
    return `${name}.zip`
  }
  return "压缩包.zip"
}

export const Compress = () => {
  const t = useT()
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [archiveName, setArchiveName] = createSignal("压缩包.zip")
  const [password, setPassword] = createSignal("")
  const [loading, submit] = useFetch(fsArchiveCompress)
  if (!userCan("compress")) {
    return null
  }

  const handler = (name: string) => {
    if (name === "compress") {
      batch(() => {
        setArchiveName(guessArchiveName())
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
      header={t("home.toolbar.choose_dst_folder")}
      opened={isOpen()}
      onClose={onClose}
      loading={loading()}
      onSubmit={async (dst) => {
        const resp = await submit(
          pathname(),
          dst,
          selectedObjs().map((item) => item.name),
          archiveName(),
          password(),
        )
        handleRespWithNotifySuccess(resp, () => {
          refresh()
          onClose()
        })
      }}
    >
      <VStack spacing="$2" alignItems="flex-start">
        <Text size="sm">压缩包名称</Text>
        <Input
          size="sm"
          value={archiveName()}
          onInput={(e: any) =>
            setArchiveName((e.target as HTMLInputElement).value)
          }
        />
        <Text size="sm">压缩密码（可选）</Text>
        <Input
          size="sm"
          type="password"
          value={password()}
          onInput={(e: any) =>
            setPassword((e.target as HTMLInputElement).value)
          }
        />
      </VStack>
    </ModalFolderChoose>
  )
}
