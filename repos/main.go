package repos

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"msig/repos/auth"
	"msig/repos/contract"
)

// Provider is the repository provider.
type Provider struct {
	db *gorm.DB
	tx *gorm.DB
}

// New creates a new instance of Provider with the underlying DB instance.
func New(db *gorm.DB) *Provider {
	return &Provider{
		db: db,
	}
}
func (u *Provider) getDB() *gorm.DB {
	if u.tx != nil {
		return u.tx
	}
	return u.db
}

//Heath returns a new health check of repository provider.
func (u *Provider) Health() (err error) {
	db, err := u.db.DB()
	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		return err
	}

	return nil
}

func (u *Provider) GetContract() contract.Repo {
	return contract.New(u.getDB())
}

func (u *Provider) GetAuth() auth.Repo {
	return auth.New(u.getDB())
}

func (u *Provider) Start(ctx context.Context) {
	u.tx = u.db.Begin()
}

func (u *Provider) RollbackUnlessCommitted() {
	if u.tx != nil {
		if err := u.tx.Rollback().Error; err != nil {
			logrus.Printf("error on rollback: %s", err.Error())
		}
		u.tx = nil
	}
}

func (u *Provider) Commit() error {
	if u.tx == nil {
		return fmt.Errorf("tx is empty")
	}
	if err := u.tx.Commit().Error; err != nil {
		u.RollbackUnlessCommitted()
		return err
	}
	u.tx = nil
	return nil
}
