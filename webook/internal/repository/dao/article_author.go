package dao

import (
	"context"
	"gorm.io/gorm"
)

//go:generate mockgen -destination=./mocks/article_author.mock.go -package=daomocks -source=./article_author.go ArticleAuthorDAO
type ArticleAuthorDAO interface {
	Create(ctx context.Context, art Article) (int64, error)
	Update(ctx context.Context, art Article) error
}

type GORMArticleAuthorDAO struct {
	db *gorm.DB
}

func NewGORMArticleAuthorDAO(db *gorm.DB) ArticleAuthorDAO {
	return &GORMArticleAuthorDAO{
		db: db,
	}
}

func (G GORMArticleAuthorDAO) Create(ctx context.Context, art Article) (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (G GORMArticleAuthorDAO) Update(ctx context.Context, art Article) error {
	//TODO implement me
	panic("implement me")
}
