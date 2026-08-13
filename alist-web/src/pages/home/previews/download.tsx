import {
  Button,
  HStack,
  IconButton,
  Image,
  Popover,
  PopoverArrow,
  PopoverBody,
  PopoverContent,
  PopoverTrigger,
} from "@hope-ui/solid"
import { useCopyLink, useRouter, useT } from "~/hooks"
import { me, objStore } from "~/store"
import { FileInfo } from "./info"
import { OpenWith } from "../file/open-with"
import { createSignal, Show } from "solid-js"
import { BsQrCode } from "solid-icons/bs"
import QRCode from "qrcode"
import { ObjType, UserMethods, UserPermissions } from "~/types"

export const Download = (props: { openWith?: boolean }) => {
  const t = useT()
  const { copyCurrentRawLink } = useCopyLink()
  const { setSearchParams } = useRouter()
  const [qrUrl, setQrUrl] = createSignal("")
  QRCode.toDataURL(objStore.raw_url, {
    type: "image/jpeg",
    scale: 2,
  }).then((url) => setQrUrl(url))
  const [pinned, setPinned] = createSignal(false)
  const [hover, setHover] = createSignal(false)
  return (
    <FileInfo>
      <HStack spacing="$2">
        <Button colorScheme="accent" onClick={() => copyCurrentRawLink(true)}>
          {t("home.toolbar.copy_link")}
        </Button>
        <Button as="a" href={objStore.raw_url} target="_blank">
          {t("home.preview.download")}
        </Button>
        <Show
          when={
            objStore.obj.type === ObjType.UNKNOWN &&
            UserMethods.can(
              me(),
              UserPermissions.indexOf("open_as_text"),
            )
          }
        >
          <Button
            colorScheme="success"
            onClick={() => setSearchParams({ open_as_text: "true" })}
          >
            {t("home.preview.open_as_text")}
          </Button>
        </Show>
        <Popover opened={pinned() || hover()} motionPreset="none">
          <PopoverTrigger
            as={IconButton}
            icon={<BsQrCode />}
            aria-label="QRCode"
            colorScheme="success"
            onClick={() => {
              setPinned(!pinned())
            }}
            onMouseOver={() => setHover(true)}
            onMouseOut={() => setHover(false)}
          />
          <PopoverContent width="fit-content">
            <PopoverArrow />
            <PopoverBody>
              <Image
                maxWidth="300px"
                src={qrUrl()}
                alt="QR Code of download link"
                objectFit="cover"
              />
            </PopoverBody>
          </PopoverContent>
        </Popover>
      </HStack>
      <Show when={props.openWith}>
        <OpenWith />
      </Show>
    </FileInfo>
  )
}

export default Download
