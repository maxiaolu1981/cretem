/*
包摘要
v1 包定义了通用的元数据结构（如对象元信息、列表元信息）、扩展字段类型（Extend）以及 API 操作选项（如创建、更新、删除选项）。这些结构是整个系统中资源（如用户、策略等）的基础框架，统一了数据的元信息格式、扩展字段处理方式和 API 交互的参数规范，确保各模块之间的数据交互一致性。
核心流程
该包的核心是提供通用的数据结构和处理逻辑，支撑上层资源的定义和 API 交互，主要流程如下：
扩展字段处理：通过 Extend 类型（基于 map[string]interface{}）支持资源的动态扩展字段，提供序列化、合并（从数据库影子字段恢复）等方法，解决固定字段无法满足灵活业务需求的问题。
元数据定义：
TypeMeta：标识资源的类型和 API 版本，用于序列化和反序列化时的类型识别；
ObjectMeta：所有持久化资源的基础元信息（如 ID、实例 ID、创建 / 更新时间等），并通过 GORM 钩子函数（BeforeCreate、BeforeUpdate 等）自动处理扩展字段的数据库存储（序列化到 ExtendShadow 字段）；
ListMeta：列表数据的元信息（如总条数），支撑分页查询场景。
API 操作选项：定义 CreateOptions、UpdateOptions 等结构，规范 API 操作的参数（如是否 dry run、强制更新等），统一各资源的操作行为。
中文注释代码（注释汉化）
*/

package v1

import (
	"time"

	"github.com/maxiaolu1981/cretem/nexuscore/component-base/json" // 自定义库：JSON 序列化工具
	"gorm.io/gorm"                                                 // 第三方库：GORM ORM 框架
)

// Extend 定义了一种新类型，用于存储扩展字段（动态字段）。
type Extend map[string]interface{}

// String 返回 Extend 的字符串格式（JSON 序列化结果）。
func (ext Extend) String() string {
	data, _ := json.Marshal(ext)
	return string(data)
}

// Merge 从 extendShadow（数据库中存储的扩展字段影子字符串）合并扩展字段。
// 优先保留数据库中的影子字段，若当前扩展字段中无对应键，则从影子字段中补充。
func (ext Extend) Merge(extendShadow string) Extend {
	var extend Extend

	// 始终信任数据库中的 extendShadow
	_ = json.Unmarshal([]byte(extendShadow), &extend)
	for k, v := range extend {
		if _, ok := ext[k]; !ok { // 若当前扩展字段中无此键，则从影子字段添加
			ext[k] = v
		}
	}

	return ext
}

// TypeMeta 描述 API 请求或响应中的单个对象，包含表示对象类型和 API 模式版本的字符串。
// 带版本或需要持久化的结构应嵌入 TypeMeta。
type TypeMeta struct {
	// Kind 是表示此对象所代表的 REST 资源的字符串值。
	// 服务器可从客户端提交请求的端点推断此值。
	// 不可更新。
	// 采用驼峰命名法。
	// required: false（非必填）
	Kind string `json:"kind,omitempty"`

	// APIVersion 定义此对象表示的版本化模式。
	// 服务器应将识别的模式转换为最新的内部值，并可能拒绝未识别的值。
	APIVersion string `json:"apiVersion,omitempty"`
}

// ListMeta 描述合成资源（包括列表和各种状态对象）必须具有的元数据。
// 一个资源只能包含 {ObjectMeta, ListMeta} 中的一个。
type ListMeta struct {
	TotalCount int64 `json:"totalCount,omitempty"` // 总记录数（用于分页）
}

// ObjectMeta 是所有持久化资源必须具有的元数据，包含所有对象的通用属性。
// 同时被 GORM 用作数据库模型的基础元信息。
type ObjectMeta struct {
	// ID 是此对象在时间和空间上的唯一值。通常由存储在资源创建成功时生成，
	// 且在 PUT 操作中不允许更改。
	//
	// 由系统生成。
	// 只读。
	ID uint64 `json:"id,omitempty" gorm:"primary_key;AUTO_INCREMENT;column:id"` // 数据库主键（自增）

	// InstanceID 定义字符串类型的资源标识符，使用前缀区分资源类型，便于记忆和 URL 友好。
	InstanceID string `json:"instanceID,omitempty" gorm:"unique;column:instanceID;type:varchar(32);not null"` // 唯一实例 ID

	// Name 在每个命名空间内必须唯一。
	// 并非所有对象都需要限定到用户名 - 这些对象的此字段值将为空。
	//
	// 必须符合 DNS_LABEL 格式。
	// 不可更新。
	// （注：原注释中的 Username 字段已注释，此处保留说明）

	// Required: true（必填）
	// Name 必须唯一。创建资源时必填。
	// Name 主要用于创建幂等性和配置定义。
	// 仅当未指定 Name 时，才会自动生成。
	// 不可更新。
	Name string `json:"name,omitempty" gorm:"column:name;type:varchar(64);not null" validate:"name"` // 资源名称（唯一）

	// Extend 存储需要添加但不想新增表列的字段，不会存储在数据库中。
	Extend Extend `json:"extend,omitempty" gorm:"-" validate:"omitempty"` // 扩展字段（内存中）

	// ExtendShadow 是 Extend 的影子字段。请勿直接修改。
	// 用于将 Extend 序列化后存储到数据库（GORM 映射字段）。
	ExtendShadow string `json:"-" gorm:"column:extendShadow" validate:"omitempty"` // 扩展字段的数据库存储（JSON 字符串）

	// CreatedAt 是表示此对象在服务器上创建时的时间戳。
	// 不保证在不同操作之间按 happens-before 顺序设置。
	// 客户端不得设置此值。以 RFC3339 格式表示，且位于 UTC 时区。
	//
	// 由系统生成。
	// 只读。
	// 列表中为 null。
	CreatedAt time.Time `json:"createdAt,omitempty" gorm:"column:createdAt"` // 创建时间

	// UpdatedAt 是表示此对象在服务器上更新时的时间戳。
	// 客户端不得设置此值。以 RFC3339 格式表示，且位于 UTC 时区。
	//
	// 由系统生成。
	// 只读。
	// 列表中为 null。
	UpdatedAt time.Time `json:"updatedAt,omitempty" gorm:"column:updatedAt"` // 更新时间

	// DeletedAt 是此资源将被删除的 RFC 3339 日期和时间。
	// 此字段由服务器在用户请求优雅删除时设置，客户端不能直接设置。
	//
	// 当请求优雅删除时由系统生成。
	// 只读。
	// （注：原字段已注释，此处保留说明）
	// DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deletedAt;index:idx_deletedAt"`
}

// BeforeCreate 是 GORM 的钩子函数，在数据库记录创建前执行。
// 作用：将 Extend 序列化为字符串，存储到 ExtendShadow 字段（用于数据库存储）。
func (obj *ObjectMeta) BeforeCreate(tx *gorm.DB) error {
	obj.ExtendShadow = obj.Extend.String()
	return nil
}

// BeforeUpdate 是 GORM 的钩子函数，在数据库记录更新前执行。
// 作用：同 BeforeCreate，更新 ExtendShadow 以同步 Extend 的最新状态。
func (obj *ObjectMeta) BeforeUpdate(tx *gorm.DB) error {
	obj.ExtendShadow = obj.Extend.String()
	return nil
}

// AfterFind 是 GORM 的钩子函数，在查询数据库记录后执行。
// 作用：将数据库中的 ExtendShadow 反序列化为 Extend（内存中可用的扩展字段）。
func (obj *ObjectMeta) AfterFind(tx *gorm.DB) error {
	if err := json.Unmarshal([]byte(obj.ExtendShadow), &obj.Extend); err != nil {
		return err
	}
	return nil
}

// ListOptions 是标准 REST 列表查询的选项参数。
type ListOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// LabelSelector 用于查找匹配的 REST 资源。
	LabelSelector string `json:"labelSelector,omitempty" form:"labelSelector"`

	// FieldSelector 按字段限制返回的对象列表。默认为返回所有。
	FieldSelector string `json:"fieldSelector,omitempty" form:"fieldSelector"`

	// TimeoutSeconds 指定 ClientIP 类型会话的粘性时间（秒）。
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// Offset 指定开始返回记录前要跳过的记录数（分页偏移量）。
	Offset *int64 `json:"offset,omitempty" form:"offset"`

	// Limit 指定要检索的记录数（每页条数）。
	Limit *int64 `json:"limit,omitempty" form:"limit"`
}

// ExportOptions 是标准 REST 获取调用的查询选项。
// 已废弃。计划在 1.18 版本中移除。
type ExportOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// 此值是否应导出。导出会剥离用户无法指定的字段。
	// 已废弃。计划在 1.18 版本中移除。
	Export bool `json:"export"`
	// 导出是否应精确。精确导出保留集群特定字段（如 'Namespace'）。
	// 已废弃。计划在 1.18 版本中移除。
	Exact bool `json:"exact"`
}

// GetOptions 是标准 REST 获取调用的查询选项。
type GetOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息
}

// DeleteOptions 可在删除 API 对象时提供。
type DeleteOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// +optional（可选）
	Unscoped bool `json:"unscoped"` // 是否不使用默认作用域（如不过滤软删除记录）
}

// CreateOptions 可在创建 API 对象时提供。
type CreateOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// 当存在时，表示修改不应被持久化。无效或未识别的 dryRun 指令将
	// 导致错误响应，且请求不再进一步处理。有效值为：
	// - All：处理所有 dry run 阶段
	// +optional（可选）
	DryRun []string `json:"dryRun,omitempty"` // 干跑模式（不实际执行操作）
}

// PatchOptions 可在补丁更新 API 对象时提供。
// PatchOptions 旨在作为 UpdateOptions 的超集。
type PatchOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// 当存在时，表示修改不应被持久化。无效或未识别的 dryRun 指令将
	// 导致错误响应，且请求不再进一步处理。有效值为：
	// - All：处理所有 dry run 阶段
	// +optional（可选）
	DryRun []string `json:"dryRun,omitempty"` // 干跑模式

	// Force 用于 "force" Apply 请求。表示用户将重新获取
	// 其他人拥有的冲突字段。非 apply 补丁请求必须 unset 此标志。
	// +optional（可选）
	Force bool `json:"force,omitempty"` // 强制更新（忽略冲突）
}

// UpdateOptions 可在更新 API 对象时提供。
// UpdateOptions 中的所有字段也应存在于 PatchOptions 中。
type UpdateOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// 当存在时，表示修改不应被持久化。无效或未识别的 dryRun 指令将
	// 导致错误响应，且请求不再进一步处理。有效值为：
	// - All：处理所有 dry run 阶段
	// +optional（可选）
	DryRun []string `json:"dryRun,omitempty"` // 干跑模式
}

// AuthorizeOptions 可在授权 API 对象时提供。
type AuthorizeOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息
}

// TableOptions 用于调用者请求 Table 时。
type TableOptions struct {
	TypeMeta `json:",inline"` // 嵌入类型元信息

	// NoHeaders 仅对内部调用者开放。未包含在 OpenAPI 定义中，
	// 可能在未来版本中作为字段移除。
	NoHeaders bool `json:"-"` // 是否不返回表头
}
