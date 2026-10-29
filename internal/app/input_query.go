package app

import (
	"encoding/json"
	"fmt"
)

// cart_transform 函数在 shoplazza.extension.toml 中声明的 metafield 读取需求。
// 平台约定：resource 仅支持 product / shop，条数上限为 MaxMetafieldsQuery（5 条）。
// 声明随函数一起上传；store 安装 app 后，平台在函数执行前按声明预取这些 metafield，
// 注入到函数输入里。
const (
	MetafieldsQueryResourceProduct = "product"
	MetafieldsQueryResourceShop    = "shop"
	MaxMetafieldsQuery             = 5
)

// MetafieldsQueryItem 对应 toml 中一个 [[metafields_query]] 块，也是序列化进
// input_query 的单条查询。toml 与 json 标签分别服务「读配置」与「传给后端」。
type MetafieldsQueryItem struct {
	Resource  string `toml:"resource" json:"resource"`
	Namespace string `toml:"namespace" json:"namespace"`
	Key       string `toml:"key" json:"key"`
}

// functionInputQuery 是上传给平台的 input_query 顶层结构。
type functionInputQuery struct {
	MetafieldsQuery []MetafieldsQueryItem `json:"metafields_query"`
}

// BuildInputQuery 校验声明的 metafields_query 并序列化为上传用的 input_query JSON 字符串。
//
// 无条目时返回 ""（不报错，调用方据此不上传 input_query）。CLI 面向开发者，
// 遇到非法 resource、空 namespace/key 或超过 MaxMetafieldsQuery 时直接返回错误，
// 让开发者在发布时就修正 toml。
func BuildInputQuery(items []MetafieldsQueryItem) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	if len(items) > MaxMetafieldsQuery {
		return "", fmt.Errorf("metafields_query has %d entries, max %d", len(items), MaxMetafieldsQuery)
	}
	for i, it := range items {
		if it.Resource != MetafieldsQueryResourceProduct && it.Resource != MetafieldsQueryResourceShop {
			return "", fmt.Errorf("metafields_query[%d] resource %q invalid, must be %q or %q",
				i, it.Resource, MetafieldsQueryResourceProduct, MetafieldsQueryResourceShop)
		}
		if it.Namespace == "" {
			return "", fmt.Errorf("metafields_query[%d] namespace is required", i)
		}
		if it.Key == "" {
			return "", fmt.Errorf("metafields_query[%d] key is required", i)
		}
	}

	out, err := json.Marshal(functionInputQuery{MetafieldsQuery: items})
	if err != nil {
		return "", fmt.Errorf("serialize input_query: %w", err)
	}
	return string(out), nil
}
