import { createDisclosure, HStack, Input, Text, VStack } from "@hope-ui/solid"
import { useFetch, usePath, useRouter, useT } from "~/hooks"
import { bus, fsArchiveCompress, handleRespWithNotifySuccess } from "~/utils"
import { batch, createMemo, createSignal, onCleanup } from "solid-js"
import { ModalFolderChoose, SelectWrapper } from "~/components"
import { selectedObjs } from "~/store"

export const Compress = () => {
  const t = useT()
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [loading, ok] = useFetch(fsArchiveCompress)
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const [archiveName, setArchiveName] = createSignal("archive")
  const [format, setFormat] = createSignal<"zip" | "7z">("7z")
  const [copyMode, setCopyMode] = createSignal<"temp" | "src_temp" | "none">(
    "temp",
  )
  const [password, setPassword] = createSignal("")
  const [volumeSize, setVolumeSize] = createSignal("")
  const [volumeUnit, setVolumeUnit] = createSignal<"K" | "M" | "G">("G")

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
      setCopyMode("temp")
      setPassword("")
      setVolumeSize("")
      setVolumeUnit("G")
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
        const trimmedVolumeSize = volumeSize().trim()
        const resp = await ok(
          pathname(),
          dst,
          selectedObjs().map((o) => o.name),
          format(),
          name,
          copyMode(),
          password(),
          trimmedVolumeSize ? `${trimmedVolumeSize}${volumeUnit()}` : "",
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
          <SelectWrapper
            value={format()}
            onChange={(v) => setFormat(v as "zip" | "7z")}
            options={[
              { label: "7z", value: "7z" },
              { label: "zip", value: "zip" },
            ]}
            size="sm"
            w="$full"
          />
        </HStack>
        <HStack width="100%" spacing="$1">
          <Text size="sm">{t("home.toolbar.compress_copy_mode")}</Text>
          <SelectWrapper
            value={copyMode()}
            onChange={(v) => setCopyMode(v as "temp" | "src_temp" | "none")}
            options={[
              {
                label: t("home.toolbar.compress_copy_mode_temp"),
                value: "temp",
              },
              {
                label: t("home.toolbar.compress_copy_mode_src_temp"),
                value: "src_temp",
              },
              {
                label: t("home.toolbar.compress_copy_mode_none"),
                value: "none",
              },
            ]}
            size="sm"
            w="$full"
          />
        </HStack>
        <HStack width="100%" spacing="$1" alignItems="center">
          <Text size="sm" css={{ whiteSpace: "nowrap" }}>
            {t("home.toolbar.compress_volume_size")}
          </Text>
          <Input
            value={volumeSize()}
            onInput={(e: any) =>
              setVolumeSize((e.target.value as string).replace(/\D/g, ""))
            }
            inputMode="numeric"
            pattern="[0-9]*"
            placeholder={t("home.toolbar.compress_volume_size_placeholder")}
            size="sm"
            flexGrow="1"
          />
          <SelectWrapper
            value={volumeUnit()}
            onChange={(v) => setVolumeUnit(v as "K" | "M" | "G")}
            options={[
              { label: "K", value: "K" },
              { label: "M", value: "M" },
              { label: "G", value: "G" },
            ]}
            size="sm"
            w="$24"
          />
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
