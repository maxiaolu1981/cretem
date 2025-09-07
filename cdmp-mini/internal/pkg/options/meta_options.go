package options

import (
	"fmt"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"
	"github.com/maxiaolu1981/cretem/nexuscore/errors"
)

type MetaOptions struct {
	ListOptions   *v1.ListOptions
	UpdateOptions *v1.UpdateOptions
	GetOptions    *v1.GetOptions
	DeleteOptions *v1.DeleteOptions
	CreateOptions *v1.CreateOptions
}

func NewMetaOptions() *MetaOptions {
	timeout := int64(60)
	offset := int64(20)
	limit := int64(50)
	return &MetaOptions{
		ListOptions: &v1.ListOptions{
			TimeoutSeconds: &timeout,
			Offset:         &offset,
			Limit:          &limit,
		},
		UpdateOptions: &v1.UpdateOptions{},
		DeleteOptions: &v1.DeleteOptions{},
		CreateOptions: &v1.CreateOptions{},
	}
}

// Complete 填充默认值并验证选项，返回错误信息
func (o *MetaOptions) Complete() error {
	if o == nil {
		o = &MetaOptions{}
	}

	// 确保各选项对象不为nil
	if o.ListOptions == nil {
		o.ListOptions = &v1.ListOptions{}
	}
	if o.UpdateOptions == nil {
		o.UpdateOptions = &v1.UpdateOptions{}
	}
	if o.GetOptions == nil {
		o.GetOptions = &v1.GetOptions{}
	}
	if o.DeleteOptions == nil {
		o.DeleteOptions = &v1.DeleteOptions{}
	}
	if o.CreateOptions == nil {
		o.CreateOptions = &v1.CreateOptions{}
	}

	// 使用指定的默认值
	timeout := int64(60)
	offset := int64(20)
	limit := int64(50)

	// 完成 ListOptions 配置
	if o.ListOptions.TimeoutSeconds == nil {
		o.ListOptions.TimeoutSeconds = &timeout
	}
	if o.ListOptions.Offset == nil {
		o.ListOptions.Offset = &offset
	}
	if o.ListOptions.Limit == nil {
		o.ListOptions.Limit = &limit
	}

	// 确保 TypeMeta 有合理值
	if o.ListOptions.APIVersion == "" {
		o.ListOptions.APIVersion = "v1"
	}

	if o.UpdateOptions.APIVersion == "" {
		o.UpdateOptions.APIVersion = "v1"
	}

	// 确保 GetOptions TypeMeta 有合理值
	if o.GetOptions.APIVersion == "" {
		o.GetOptions.APIVersion = "v1"
	}

	// 确保 DeleteOptions TypeMeta 有合理值
	if o.DeleteOptions.APIVersion == "" {
		o.DeleteOptions.APIVersion = "v1"
	}

	if o.CreateOptions.APIVersion == "" {
		o.CreateOptions.APIVersion = "v1"
	}

	return nil
}

// Validate 验证 MetaOptions 的参数有效性
func (o *MetaOptions) Validate() error {
	if o == nil {
		return errors.New("MetaOptions 不能为空")
	}

	// 验证 TimeoutSeconds
	if o.ListOptions.TimeoutSeconds != nil {
		if *o.ListOptions.TimeoutSeconds < 0 {
			return errors.New("超时时间不能为负数")
		}
		if *o.ListOptions.TimeoutSeconds > 60 {
			return errors.New("超时时间不能超过60秒")
		}
		// 0 是允许的（表示不超时），但不建议
	}

	// 验证 Offset
	if o.ListOptions.Offset != nil {
		if *o.ListOptions.Offset < 0 {
			return errors.New("分页偏移量不能为负数")
		}
		// 通常不建议超过 TotalCount，但这里无法验证，由业务层处理
	}

	// 验证 Limit
	if o.ListOptions.Limit != nil {
		if *o.ListOptions.Limit < 0 {
			return errors.New("每页条数不能为负数")
		}
		if *o.ListOptions.Limit > 100 {
			return errors.New("每页条数不能超过100条")
		}
		// 0 是允许的（表示返回空列表）
	}

	// 验证 DryRun 参数（必须是 nil 或精确的 ["All"]）
	if o.UpdateOptions.DryRun != nil {
		if len(o.UpdateOptions.DryRun) != 1 {
			return errors.New("UpdateOptions.DryRun 必须为 nil 或精确的 [\"All\"]")
		}
		if o.UpdateOptions.DryRun[0] != "All" {
			return fmt.Errorf("UpdateOptions.DryRun 必须为 \"All\", 实际值: %s", o.UpdateOptions.DryRun[0])
		}
	}

	if o.CreateOptions.DryRun != nil {
		if len(o.CreateOptions.DryRun) != 1 {
			return errors.New("CreateOptions.DryRun 必须为 nil 或精确的 [\"All\"]")
		}
		if o.CreateOptions.DryRun[0] != "All" {
			return fmt.Errorf("CreateOptions.DryRun 必须为 \"All\", 实际值: %s", o.CreateOptions.DryRun[0])
		}
	}

	return nil
}
