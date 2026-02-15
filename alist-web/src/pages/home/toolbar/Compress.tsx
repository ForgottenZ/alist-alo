import {
  Button,
  Checkbox,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  createDisclosure,
} from "@hope-ui/solid"
import { createEffect, createSignal, onCleanup } from "solid-js"
import { useFetch, usePath, useRouter, useT } from "~/hooks"
import { selectedObjs } from "~/store"
import { bus, fsCompress, handleRespWithNotifySuccess } from "~/utils"

const defaultZipName = () => {
  const names = selectedObjs().map((obj) => obj.name)
  if (names.length === 1) {
    return `${names[0]}.zip`
  }
  return `archive-${new Date().getTime()}.zip`
}

export const Compress = () => {
  const t = useT()
  const { pathname } = useRouter()
  const { refresh } = usePath()
  const { isOpen, onOpen, onClose } = createDisclosure()
  const [overwrite, setOverwrite] = createSignal(false)
  const [name, setName] = createSignal("")
  const [loading, compress] = useFetch(fsCompress)

  const handler = (tool: string) => {
    if (tool === "compress") {
      setName(defaultZipName())
      setOverwrite(false)
      onOpen()
    }
  }

  bus.on("tool", handler)
  onCleanup(() => {
    bus.off("tool", handler)
  })

  createEffect(() => {
    if (!isOpen()) {
      return
    }
    if (!name()) {
      setName(defaultZipName())
    }
  })

  return (
    <Modal
      opened={isOpen()}
      onClose={onClose}
      size={{
        "@initial": "xs",
        "@md": "md",
      }}
    >
      <ModalOverlay />
      <ModalContent>
        <ModalHeader>{t("home.toolbar.compress")}</ModalHeader>
        <ModalBody>
          <FormControl>
            <FormLabel>{t("home.toolbar.input_filename")}</FormLabel>
            <Input value={name()} onInput={(e) => setName(e.currentTarget.value)} />
          </FormControl>
        </ModalBody>
        <ModalFooter display="flex" gap="$2">
          <Checkbox
            mr="auto"
            checked={overwrite()}
            onChange={() => {
              setOverwrite(!overwrite())
            }}
          >
            {t("home.conflict_policy.overwrite_existing")}
          </Checkbox>
          <Button onClick={onClose}>{t("global.cancel")}</Button>
          <Button
            colorScheme="accent"
            loading={loading()}
            onClick={async () => {
              const resp = await compress(
                pathname(),
                selectedObjs().map((obj) => obj.name),
                name(),
                overwrite(),
              )
              handleRespWithNotifySuccess(resp, () => {
                refresh(undefined, true)
                onClose()
              })
            }}
          >
            {t("global.confirm")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
