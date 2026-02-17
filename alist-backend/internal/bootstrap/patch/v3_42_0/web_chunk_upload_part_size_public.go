package v3_42_0

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func SetWebChunkUploadPartSizePublic() {
	item, err := op.GetSettingItemByKey("web_chunk_upload_part_size")
	if err != nil {
		utils.Log.Errorf("cannot load web_chunk_upload_part_size setting: %v", err)
		return
	}
	if item.Flag == model.PUBLIC {
		return
	}
	item.Flag = model.PUBLIC
	if err = op.SaveSettingItem(item); err != nil {
		utils.Log.Errorf("cannot update web_chunk_upload_part_size flag: %v", err)
	}
}
