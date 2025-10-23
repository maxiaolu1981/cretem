package server

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	v1 "github.com/maxiaolu1981/cretem/nexuscore/api/apiserver/v1"
	metav1 "github.com/maxiaolu1981/cretem/nexuscore/component-base/meta/v1"

	"github.com/maxiaolu1981/cretem/cdmp-mini/internal/pkg/usercache"
	"github.com/maxiaolu1981/cretem/cdmp-mini/pkg/log"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type userRowBuffer struct {
	id         uint64
	instanceID string
	name       string
	nickname   string
	password   string
	email      sql.RawBytes
	phone      sql.RawBytes
	status     int
	isAdmin    int
	extend     sql.RawBytes
	createdAt  time.Time
	updatedAt  time.Time
	dest       [12]any
}

func newUserRowBuffer() *userRowBuffer {
	buf := &userRowBuffer{}
	buf.dest = [12]any{
		&buf.id,
		&buf.instanceID,
		&buf.name,
		&buf.nickname,
		&buf.password,
		&buf.email,
		&buf.phone,
		&buf.status,
		&buf.isAdmin,
		&buf.extend,
		&buf.createdAt,
		&buf.updatedAt,
	}
	return buf
}

func (b *userRowBuffer) reset() {
	b.id = 0
	b.instanceID = ""
	b.name = ""
	b.nickname = ""
	b.password = ""
	b.email = nil
	b.phone = nil
	b.status = 0
	b.isAdmin = 0
	b.extend = nil
	b.createdAt = time.Time{}
	b.updatedAt = time.Time{}
}

var userRowBufferPool = sync.Pool{
	New: func() any {
		return newUserRowBuffer()
	},
}

func scanUserFromScanner(scanner rowScanner) (*v1.User, error) {
	buf := userRowBufferPool.Get().(*userRowBuffer)
	buf.reset()

	if err := scanner.Scan(buf.dest[:]...); err != nil {
		userRowBufferPool.Put(buf)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			ID:         buf.id,
			InstanceID: buf.instanceID,
			Name:       buf.name,
			CreatedAt:  buf.createdAt,
			UpdatedAt:  buf.updatedAt,
		},
		Nickname: buf.nickname,
		Password: buf.password,
		Status:   buf.status,
		IsAdmin:  buf.isAdmin,
	}

	if len(buf.email) > 0 {
		user.Email = usercache.NormalizeEmail(string(buf.email))
	}
	if len(buf.phone) > 0 {
		user.Phone = usercache.NormalizePhone(string(buf.phone))
	}

	if len(buf.extend) > 0 {
		user.ExtendShadow = string(buf.extend)
		ext := make(metav1.Extend)
		if err := jsonCodec.Unmarshal(buf.extend, &ext); err != nil {
			log.Debugf("extendShadow 解码失败: %v", err)
		} else {
			user.Extend = ext
		}
	}

	userRowBufferPool.Put(buf)
	return user, nil
}
