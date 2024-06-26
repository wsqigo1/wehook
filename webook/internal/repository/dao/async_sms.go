package dao

import (
	"context"
	"github.com/ecodeclub/ekit/sqlx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrWaitingSMSNotFound = gorm.ErrRecordNotFound

//go:generate mockgen -source=./async_sms.go -package=daomocks -destination=mocks/async_sms.mock.go AsyncSmsDAO
type AsyncSmsDAO interface {
	Insert(ctx context.Context, s AsyncSms) error
	GetWaitingSMS(ctx context.Context) (AsyncSms, error)
	MarkSuccess(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64) error
}

const (
	// 因为本身状态没有暴露出去，所以不需要在 domain 里面定义
	asyncStatusWaiting = iota
	// 失败了，并且超过了重试次数
	asyncStatusFailed
	asyncStatusSuccess
)

type GORMAsyncSmsDAO struct {
	db *gorm.DB
}

func (g *GORMAsyncSmsDAO) Insert(ctx context.Context, s AsyncSms) error {
	return g.db.Create(&s).Error
}

func (g *GORMAsyncSmsDAO) GetWaitingSMS(ctx context.Context) (AsyncSms, error) {
	// 如果在高并发情况下， SELECT for UPDATE 对数据库的压力很大
	// 但是我们不是高并发，因为你部署 N 台机器，才有 N 个 goroutine 来查询
	// 并发不过百，随便写
	var s AsyncSms
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 避开一些偶发性的失败，我们只找 1 分钟前的异步短信发送
		now := time.Now().UnixMilli()
		endTime := now - time.Minute.Milliseconds()
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("utime < ? and status = ?", endTime, asyncStatusWaiting).
			First(&s).Error
		// SELECT xx FROM xxx WHERE xx FOR UPDATE，锁住了
		if err != nil {
			return err
		}

		// 只要更新了更新时间，根据我们的前端的规则，就不可能被别的节点抢占了
		err = tx.Model(&AsyncSms{}).
			Where("id = ?", s.Id).
			Updates(map[string]any{
				"retry_cnt": gorm.Expr("retry_cnt + 1"),
				// 更新成了当前时间戳，确保我在发送过程中，没人会再次抢到它
				// 也相当于，重试间隔一分钟
				"utime": now,
			}).Error
		return err
	})
	return s, err
}

func (g *GORMAsyncSmsDAO) MarkSuccess(ctx context.Context, id int64) error {
	//TODO implement me
	panic("implement me")
}

func (g *GORMAsyncSmsDAO) MarkFailed(ctx context.Context, id int64) error {
	//TODO implement me
	panic("implement me")
}

func NewGORMAsyncSmsDAO(db *gorm.DB) *GORMAsyncSmsDAO {
	return &GORMAsyncSmsDAO{
		db: db,
	}
}

type AsyncSms struct {
	Id int64
	// 使用我在 ekit 里面支持的 JSON 字段
	Config sqlx.JsonColumn[SmsConfig]
	// 重试次数
	RetryCnt int
	// 重试的最大次数
	RetryMax int
	Status   int
	Ctime    int64
	Utime    int64 `gorm:"index"`
}

type SmsConfig struct {
	TplId   string
	Args    []string
	Numbers []string
}
