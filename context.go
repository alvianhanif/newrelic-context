package nrcontext

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/smacker/newrelic-context/nrgorm"
	"github.com/smacker/newrelic-context/nrredis"
	redis "gopkg.in/redis.v5"
)

type contextKey int

const txnKey contextKey = 0

// Set NewRelic transaction to context
func ContextWithTxn(c context.Context, txn newrelic.Transaction) context.Context {
	return context.WithValue(c, txnKey, txn)
}

// Get NewRelic transaction from context anywhere
func GetTnxFromContext(c context.Context) newrelic.Transaction {
	tnx := c.Value(txnKey)
	return tnx.(newrelic.Transaction)
}

// Sets transaction from Context to gorm settings, returns cloned DB
func SetTxnToGorm(ctx context.Context, db *gorm.DB) *gorm.DB {
	txn := GetTnxFromContext(ctx)
	return nrgorm.SetTxnToGorm(txn, db)
}

// Gets transaction from Context and applies RedisWrapper, returns cloned client
func WrapRedisClient(ctx context.Context, c *redis.Client) *redis.Client {
	txn := GetTnxFromContext(ctx)
	return nrredis.WrapRedisClient(txn, c)
}
