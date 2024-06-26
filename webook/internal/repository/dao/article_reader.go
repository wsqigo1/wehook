package dao

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:generate mockgen -source=./article_reader.go -package=daomocks -destination=./mocks/article_reader.mock.go ArticleReaderDAO
type ArticleReaderDAO interface {
	// Upsert INSERT OR UPDATE 语义，一般简写为 Upsert
	// 将会更新标题和内容，但是不会更新别的内容
	// 这个要求 Reader 和 Author 是不同库
	Upsert(ctx context.Context, art Article) error
	// UpsertV2 版本用于同库不同表
	UpsertV2(ctx context.Context, art PublishedArticle) error
}

type GORMArticleReaderDAO struct {
	db *gorm.DB
}

func NewGORMArticleReaderDAO(db *gorm.DB) ArticleReaderDAO {
	return &GORMArticleReaderDAO{
		db: db,
	}
}

func (dao *GORMArticleReaderDAO) Upsert(ctx context.Context, art Article) error {
	return dao.db.Clauses(clause.OnConflict{
		// ID 冲突的时候。实际上，在 MYSQL 里面你写不写都可以
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"title":   art.Title,
			"content": art.Content,
		}),
	}).Create(&art).Error
}

// UpsertV2 同库不同表
func (dao *GORMArticleReaderDAO) UpsertV2(ctx context.Context, art PublishedArticle) error {
	return dao.db.Clauses(clause.OnConflict{
		// ID 冲突的时候。实际上，在 MYSQL 里面你写不写都可以
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"title":   art.Title,
			"content": art.Content,
		}),
	}).Create(&art).Error
}

// PublishedArticle 衍生类型，偷个懒
type PublishedArticle Article
