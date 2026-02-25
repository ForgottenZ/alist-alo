package v3_42_0

import (
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func PatchWebUISettings() {
	setWebChunkUploadPartSizePublic()
	ensureFooterPoweredByHref()
}

func setWebChunkUploadPartSizePublic() {
	item, err := op.GetSettingItemByKey(conf.WebChunkUploadPartSize)
	if err != nil {
		utils.Log.Errorf("cannot load %s setting: %v", conf.WebChunkUploadPartSize, err)
		return
	}
	if item.Flag == model.PUBLIC {
		return
	}
	item.Flag = model.PUBLIC
	if err = op.SaveSettingItem(item); err != nil {
		utils.Log.Errorf("cannot update %s flag: %v", conf.WebChunkUploadPartSize, err)
	}
}

func ensureFooterPoweredByHref() {
	if _, err := op.GetSettingItemByKey(conf.WebFooterPoweredByHref); err == nil {
		return
	}
	item := model.SettingItem{
		Key:   conf.WebFooterPoweredByHref,
		Value: "https://github.com/alist-org/alist",
		Type:  conf.TypeString,
		Group: model.SINGLE,
		Flag:  model.PUBLIC,
	}
	if err := op.SaveSettingItem(&item); err != nil {
		utils.Log.Errorf("cannot create %s setting: %v", conf.WebFooterPoweredByHref, err)
	}
}
