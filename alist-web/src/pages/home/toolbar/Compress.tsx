import {
  createDisclosure,
  FormControl,
  FormLabel,
  Input,
  VStack,
} from "@hope-ui/solid"
import { batch, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose } from "~/components"
import { useFetch, usePath, useRouter } from "~/hooks"
import { selectedObjs } from "~/store"
import { bus, fsArchiveCompress, handleRespWithNotifySuccess } from "~/utils"

export const Compress = () => {
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [archiveName, setArchiveName] = createSignal("")
  const [password, setPassword] = createSignal("")
  const [loading, compress] = useFetch(fsArchiveCompress)
  const { pathname } = useRouter()
  const { refresh } = usePath()

  const handler = (name: string) => {
    if (name === "compress") {
      const objs = selectedObjs()
      if (objs.length === 0) return
      batch(() => {
        if (objs.length === 1) {
          setArchiveName(`${objs[0].name}.zip`)
        } else {
          setArchiveName(`${objs.length}个文件.zip`)
        }
        setPassword("")
      })
      onOpen()
    }
  }
  bus.on("tool", handler)
  onCleanup(() => bus.off("tool", handler))

  return (
    <ModalFolderChoose
      header="压缩到目标文件夹"
      opened={isOpen()}
      onClose={onClose}
      loading={loading()}
      onSubmit={async (dst) => {
        const objs = selectedObjs()
        const names = objs.map((o) => o.name)
        const resp = await compress(
          pathname(),
          names,
          dst,
          archiveName() || `${objs[0].name}.zip`,
          password(),
        )
        handleRespWithNotifySuccess(resp, () => {
          refresh()
          onClose()
        })
      }}
    >
      <VStack align="stretch" spacing="$2">
        <FormControl>
          <FormLabel>压缩包名称</FormLabel>
          <Input
            value={archiveName()}
            onInput={(e) => setArchiveName(e.currentTarget.value)}
          />
        </FormControl>
        <FormControl>
          <FormLabel>压缩密码（可选）</FormLabel>
          <Input
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </FormControl>
      </VStack>
    </ModalFolderChoose>
  )
}
